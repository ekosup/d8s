package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/resource"
)

// tableView is the one table component every resource view uses.
type tableView struct {
	*tview.Table
	title    string
	model    *tableModel
	shown    []resource.Row // rows currently on screen, in order
	sortKeys []columnKey

	marks map[string]bool // IDs the user marked for a bulk action
	// note, when set, supplies a remark for the title about how the list
	// is narrowed at the source, such as "active only".
	note func() string

	natural  []int // width each column wants, from the last refresh
	fitWidth int   // inner width the cells are currently fitted to; -1 = not fitted
}

type columnKey struct {
	r   rune
	col int
}

func newTableView(title string, cols []resource.Column) *tableView {
	v := &tableView{
		Table: tview.NewTable(),
		title: title,
		model: newTableModel(cols),
		marks: map[string]bool{},
	}
	v.SetBorder(true)
	v.SetFixed(1, 0)
	v.SetSelectable(true, false)
	v.SetSelectedStyle(tcell.StyleDefault.Foreground(colorSelectFg).Background(colorSelectBg))
	v.sortKeys = assignSortKeys(cols)
	v.refresh()
	return v
}

// assignSortKeys gives each column the first letter of its name that is still
// free. G is skipped: it already means "jump to the last row".
func assignSortKeys(cols []resource.Column) []columnKey {
	used := map[rune]bool{'G': true}
	var keys []columnKey
	for i, c := range cols {
		for _, r := range strings.ToUpper(c.Name) {
			if unicode.IsLetter(r) && !used[r] {
				used[r] = true
				keys = append(keys, columnKey{r: r, col: i})
				break
			}
		}
	}
	return keys
}

func (v *tableView) SetRows(rows []resource.Row) {
	v.model.SetRows(rows)
	v.refresh()
}

func (v *tableView) SetFilter(text string) {
	v.model.SetFilter(text)
	v.refresh()
}

func (v *tableView) Filter() string { return v.model.FilterText() }

// SelectedID returns the ID of the highlighted row, or "" when there is none.
func (v *tableView) SelectedID() string {
	row, _ := v.GetSelection()
	if i := row - 1; i >= 0 && i < len(v.shown) {
		return v.shown[i].ID
	}
	return ""
}

func (v *tableView) bindings() []binding {
	out := make([]binding, 0, len(v.sortKeys))
	for _, k := range v.sortKeys {
		b := runeBinding(k.r,
			"shift-"+strings.ToLower(string(k.r)),
			"Sort by "+v.model.cols[k.col].Name,
			func() {
				v.model.SortBy(k.col)
				v.refresh()
			})
		b.group = groupSort
		out = append(out, b)
	}
	return out
}

// refresh redraws the table from the model, keeping the highlight on the
// same object when it is still there.
func (v *tableView) refresh() {
	selected := v.SelectedID()
	prevRow, _ := v.GetSelection()
	rows := v.model.Visible()
	sortCol, sortDesc := v.model.Sort()

	v.Clear()
	for c, col := range v.model.cols {
		name := col.Name
		if c == sortCol {
			if sortDesc {
				name += "↓"
			} else {
				name += "↑"
			}
		}
		cell := tview.NewTableCell(name).
			SetSelectable(false).
			SetExpansion(1).
			SetTextColor(colorHeader).
			SetAttributes(tcell.AttrBold)
		if col.Right {
			cell.SetAlign(tview.AlignRight)
		}
		v.SetCell(0, c, cell)
	}
	target := 0
	for i, r := range rows {
		color := toneColors[r.Tone]
		if v.marks[r.ID] {
			color = colorMark
		}
		for c, col := range v.model.cols {
			text := ""
			if c < len(r.Cells) {
				text = r.Cells[c]
			}
			cell := tview.NewTableCell(text).SetExpansion(1).SetTextColor(color)
			if col.Right {
				cell.SetAlign(tview.AlignRight)
			}
			v.SetCell(i+1, c, cell)
		}
		if r.ID == selected {
			target = i + 1
		}
	}
	v.shown = rows
	v.dropStaleMarks()
	v.measure()

	switch {
	case len(rows) == 0:
		v.Select(0, 0)
	case target > 0:
		v.Select(target, 0)
	default:
		// The selected object is gone (or nothing was selected yet): stay
		// near the old position.
		v.Select(min(max(prevRow, 1), len(rows)), 0)
	}
	v.SetTitle(v.titleText())
}

func (v *tableView) titleText() string {
	total := v.model.Total()
	name := v.title
	if v.note != nil {
		if n := v.note(); n != "" {
			name += "(" + n + ")"
		}
	}
	t := fmt.Sprintf(" %s[%d] ", name, total)
	if f := v.model.FilterText(); f != "" {
		t = fmt.Sprintf(" %s[%d/%d] </%s> ", name, len(v.shown), total, tview.Escape(f))
	}
	if n := len(v.marks); n > 0 {
		t += fmt.Sprintf("(%d marked) ", n)
	}
	return t
}

// minColumnWidth is how far a column may be squeezed; below this a value is
// no longer recognisable.
const minColumnWidth = 8

// measure records the width every column needs to show its text in full.
func (v *tableView) measure() {
	v.natural = make([]int, len(v.model.cols))
	for r := range v.GetRowCount() {
		for c := range v.natural {
			v.natural[c] = max(v.natural[c], tview.TaggedStringWidth(v.GetCell(r, c).Text))
		}
	}
	v.fitWidth = -1
}

// Draw fits the columns to the available width first, so that a long value
// in one column never pushes the columns after it off the screen.
func (v *tableView) Draw(screen tcell.Screen) {
	if _, _, width, _ := v.GetInnerRect(); width != v.fitWidth {
		v.fit(width)
	}
	v.Table.Draw(screen)
}

// fit shortens the widest columns, one character at a time, until the table
// is no wider than width. Columns that already fit are left alone.
func (v *tableView) fit(width int) {
	v.fitWidth = width
	widths := append([]int(nil), v.natural...)
	total := max(len(widths)-1, 0) // one separator between columns
	for _, w := range widths {
		total += w
	}
	for total > width {
		// The first column identifies the row, so it is squeezed last.
		widest := -1
		for c, w := range widths {
			if c > 0 && w > minColumnWidth && (widest < 0 || w > widths[widest]) {
				widest = c
			}
		}
		if widest < 0 && len(widths) > 0 && widths[0] > minColumnWidth {
			widest = 0
		}
		if widest < 0 {
			break // nothing left to squeeze; the terminal is simply too small
		}
		widths[widest]--
		total--
	}
	for c, w := range widths {
		limit := 0 // no limit
		if w < v.natural[c] {
			limit = w
		}
		for r := range v.GetRowCount() {
			v.GetCell(r, c).SetMaxWidth(limit)
		}
	}
}

// SelectedRow returns the highlighted row.
func (v *tableView) SelectedRow() (resource.Row, bool) {
	row, _ := v.GetSelection()
	if i := row - 1; i >= 0 && i < len(v.shown) {
		return v.shown[i], true
	}
	return resource.Row{}, false
}

// toggleMark marks or unmarks the highlighted row and moves down, so that
// holding space marks a run of rows.
func (v *tableView) toggleMark() {
	row, ok := v.SelectedRow()
	if !ok {
		return
	}
	if v.marks[row.ID] {
		delete(v.marks, row.ID)
	} else {
		v.marks[row.ID] = true
	}
	if r, _ := v.GetSelection(); r < len(v.shown) {
		v.Select(r+1, 0)
	}
	v.refresh()
}

// MarkedRows returns the marked rows that are currently visible, in table
// order. Rows hidden by a filter are never acted on.
func (v *tableView) MarkedRows() []resource.Row {
	var out []resource.Row
	for _, r := range v.shown {
		if v.marks[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

// ClearMarks unmarks everything and reports whether anything was marked.
func (v *tableView) ClearMarks() bool {
	if len(v.marks) == 0 {
		return false
	}
	clear(v.marks)
	v.refresh()
	return true
}

// dropStaleMarks forgets marks on objects that no longer exist.
func (v *tableView) dropStaleMarks() {
	if len(v.marks) == 0 {
		return
	}
	alive := make(map[string]bool, v.model.Total())
	for _, r := range v.model.rows {
		alive[r.ID] = true
	}
	for id := range v.marks {
		if !alive[id] {
			delete(v.marks, id)
		}
	}
}

// SetSort sets the order the table starts with.
func (v *tableView) SetSort(col int, desc bool) {
	if col < 0 || col >= len(v.model.cols) {
		return
	}
	v.model.sortCol, v.model.sortDesc, v.model.dirty = col, desc, true
	v.refresh()
}
