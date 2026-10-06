package ui

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ekosup/d8s/internal/resource"
)

var testCols = []resource.Column{{Name: "NAME"}, {Name: "IMAGE"}, {Name: "STATE"}, {Name: "AGE"}}

func testRows() []resource.Row {
	return []resource.Row{
		{ID: "1", Cells: []string{"web", "nginx:alpine", "running", "2h"}, SortKeys: []string{"", "", "", "0200"}},
		{ID: "2", Cells: []string{"db", "postgres:15", "exited", "3d"}, SortKeys: []string{"", "", "", "7200"}},
		{ID: "3", Cells: []string{"api", "nginx:1.27", "running", "5m"}, SortKeys: []string{"", "", "", "0005"}},
	}
}

func ids(rows []resource.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

func TestModelFilter(t *testing.T) {
	tests := []struct {
		filter string
		want   []string
	}{
		{"", []string{"3", "2", "1"}},
		{"nginx", []string{"3", "1"}},
		{"NGINX", []string{"3", "1"}},      // case-insensitive
		{"!nginx", []string{"2"}},          // inverse
		{"^(web|db)$", []string{"2", "1"}}, // regex
		{"nginx:1\\.", []string{"3"}},      // regex with escape
		{"[", nil},                         // invalid regex falls back to a literal, which matches nothing
		{"!", []string{"3", "2", "1"}},     // inverse of nothing keeps everything
		{"exited", []string{"2"}},          // any column
		{"nomatch", nil},
	}
	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			m := newTableModel(testCols)
			m.SetRows(testRows())
			m.SetFilter(tt.filter)
			if got := ids(m.Visible()); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModelSort(t *testing.T) {
	m := newTableModel(testCols)
	m.SetRows(testRows())

	// Default: first column ascending.
	if got := ids(m.Visible()); !slices.Equal(got, []string{"3", "2", "1"}) {
		t.Fatalf("default order: %v", got)
	}
	m.SortBy(0) // same column again flips the direction
	if got := ids(m.Visible()); !slices.Equal(got, []string{"1", "2", "3"}) {
		t.Fatalf("name desc: %v", got)
	}
	m.SortBy(3) // AGE uses sort keys, not the display text
	if got := ids(m.Visible()); !slices.Equal(got, []string{"3", "1", "2"}) {
		t.Fatalf("age asc: %v", got)
	}
	if col, desc := m.Sort(); col != 3 || desc {
		t.Fatalf("sort state: col %d desc %v", col, desc)
	}
	m.SortBy(99) // out of range is ignored
	if col, _ := m.Sort(); col != 3 {
		t.Fatalf("out-of-range column changed the sort to %d", col)
	}
}

func TestModelSortIsStableAcrossRefresh(t *testing.T) {
	m := newTableModel(testCols)
	m.SortBy(2) // STATE: two rows tie on "running"
	m.SetRows(testRows())
	first := ids(m.Visible())
	rows := testRows()
	slices.Reverse(rows)
	m.SetRows(rows)
	if got := ids(m.Visible()); !slices.Equal(got, first) {
		t.Fatalf("order changed between refreshes: %v then %v", first, got)
	}
}

func TestModelFilterSurvivesRefresh(t *testing.T) {
	m := newTableModel(testCols)
	m.SetRows(testRows())
	m.SetFilter("nginx")
	m.SetRows(append(testRows(), resource.Row{ID: "4", Cells: []string{"cache", "nginx:alpine", "running", "1s"}}))
	if got := ids(m.Visible()); !slices.Equal(got, []string{"3", "4", "1"}) {
		t.Fatalf("got %v", got)
	}
	if m.Total() != 4 {
		t.Fatalf("total %d", m.Total())
	}
}

func manyRows(n int) []resource.Row {
	rows := make([]resource.Row, n)
	for i := range rows {
		state := "running"
		if i%7 == 0 {
			state = "exited"
		}
		rows[i] = resource.Row{
			ID:    fmt.Sprintf("id-%05d", i),
			Cells: []string{fmt.Sprintf("svc-%05d", (i*7919)%n), "registry.local/app:1." + fmt.Sprint(i%40), state, fmt.Sprintf("%dm", i)},
		}
	}
	return rows
}

func BenchmarkModel2000Rows(b *testing.B) {
	rows := manyRows(2000)
	m := newTableModel(testCols)
	m.SetFilter("app:1\\.(1|2)")
	b.ResetTimer()
	for b.Loop() {
		m.SetRows(rows)
		m.SortBy(0)
		_ = m.Visible()
	}
}
