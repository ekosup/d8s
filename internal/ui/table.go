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
	if f := v.model.FilterText(); f != "" {
		return fmt.Sprintf(" %s[%d/%d] </%s> ", v.title, len(v.shown), total, tview.Escape(f))
	}
	return fmt.Sprintf(" %s[%d] ", v.title, total)
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
