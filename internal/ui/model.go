package ui

import (
	"regexp"
	"sort"
	"strings"

	"github.com/ekosup/d8s/internal/resource"
)

// tableModel holds the rows of a table together with the filter and sort
// applied to them. It knows nothing about the terminal.
type tableModel struct {
	cols     []resource.Column
	rows     []resource.Row
	filter   rowFilter
	sortCol  int
	sortDesc bool
	visible  []resource.Row
	dirty    bool
}

func newTableModel(cols []resource.Column) *tableModel {
	return &tableModel{cols: cols, dirty: true}
}

func (m *tableModel) SetRows(rows []resource.Row) {
	m.rows = rows
	m.dirty = true
}

func (m *tableModel) SetFilter(text string) {
	m.filter = parseFilter(text)
	m.dirty = true
}

func (m *tableModel) FilterText() string { return m.filter.text }

// SortBy orders by column col; choosing the current column flips the direction.
func (m *tableModel) SortBy(col int) {
	if col < 0 || col >= len(m.cols) {
		return
	}
	if col == m.sortCol {
		m.sortDesc = !m.sortDesc
	} else {
		m.sortCol, m.sortDesc = col, false
	}
	m.dirty = true
}

func (m *tableModel) Sort() (col int, desc bool) { return m.sortCol, m.sortDesc }

func (m *tableModel) Total() int { return len(m.rows) }

// Visible returns the rows that pass the filter, in sort order.
func (m *tableModel) Visible() []resource.Row {
	if !m.dirty {
		return m.visible
	}
	var out []resource.Row
	for _, r := range m.rows {
		if m.filter.matches(r) {
			out = append(out, r)
		}
	}
	col, desc := m.sortCol, m.sortDesc
	sort.SliceStable(out, func(i, j int) bool {
		a, b := sortKey(out[i], col), sortKey(out[j], col)
		if a == b {
			return out[i].ID < out[j].ID // keeps ties in place between refreshes
		}
		if desc {
			return a > b
		}
		return a < b
	})
	m.visible, m.dirty = out, false
	return out
}

func sortKey(r resource.Row, col int) string {
	if col < len(r.SortKeys) && r.SortKeys[col] != "" {
		return r.SortKeys[col]
	}
	if col < len(r.Cells) {
		return strings.ToLower(r.Cells[col])
	}
	return ""
}

// rowFilter is a parsed `/` filter: a case-insensitive regular expression,
// or a plain substring when the text is not a valid expression. A leading
// `!` inverts the match.
type rowFilter struct {
	text    string
	inverse bool
	re      *regexp.Regexp
	literal string
}

func parseFilter(text string) rowFilter {
	f := rowFilter{text: text}
	pattern, inverse := strings.CutPrefix(text, "!")
	f.inverse = inverse
	if pattern == "" {
		return f
	}
	if re, err := regexp.Compile("(?i)" + pattern); err == nil {
		f.re = re
	} else {
		f.literal = strings.ToLower(pattern)
	}
	return f
}

func (f rowFilter) matches(r resource.Row) bool {
	if f.re == nil && f.literal == "" {
		return true
	}
	hit := false
	for _, c := range r.Cells {
		if f.re != nil {
			hit = f.re.MatchString(c)
		} else {
			hit = strings.Contains(strings.ToLower(c), f.literal)
		}
		if hit {
			break
		}
	}
	return hit != f.inverse
}
