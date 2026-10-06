package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/resource"
)

// render draws p on an in-memory screen and returns its text, one string per line.
func render(t *testing.T, p tview.Primitive, w, h int) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	p.SetRect(0, 0, w, h)
	p.Draw(screen)
	screen.Show()
	cells, cw, ch := screen.GetContents()
	lines := make([]string, ch)
	for y := range ch {
		var sb strings.Builder
		for x := range cw {
			if r := cells[y*cw+x].Runes; len(r) > 0 {
				sb.WriteRune(r[0])
			} else {
				sb.WriteRune(' ')
			}
		}
		lines[y] = strings.TrimRight(sb.String(), " ")
	}
	return lines
}

func lineContaining(lines []string, s string) int {
	for i, l := range lines {
		if strings.Contains(l, s) {
			return i
		}
	}
	return -1
}

func press(v *tableView, key tcell.Key, r rune) {
	ev := tcell.NewEventKey(key, r, tcell.ModNone)
	for _, b := range v.bindings() {
		if b.matches(ev) {
			b.do()
			return
		}
	}
	v.InputHandler()(ev, func(tview.Primitive) {})
}

func TestTableRendersRowsAndTitle(t *testing.T) {
	v := newTableView("Containers", testCols)
	v.SetRows(testRows())
	lines := render(t, v, 80, 10)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Containers[3]", "NAME", "IMAGE", "STATE", "AGE", "web", "postgres:15", "running"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
	// Sorted by name: api, db, web.
	if a, d, w := lineContaining(lines, "api"), lineContaining(lines, "db "), lineContaining(lines, "web"); a >= d || d >= w {
		t.Fatalf("rows not sorted by name:\n%s", joined)
	}
}

func TestTableFilterTitle(t *testing.T) {
	v := newTableView("Containers", testCols)
	v.SetRows(testRows())
	v.SetFilter("nginx")
	joined := strings.Join(render(t, v, 80, 10), "\n")
	if !strings.Contains(joined, "Containers[2/3]") || !strings.Contains(joined, "</nginx>") {
		t.Fatalf("title does not show the filter:\n%s", joined)
	}
	if strings.Contains(joined, "postgres") {
		t.Fatalf("filtered row still visible:\n%s", joined)
	}
}

func TestTableSelectionFollowsRowAcrossRefresh(t *testing.T) {
	v := newTableView("Containers", testCols)
	v.SetRows(testRows())
	press(v, tcell.KeyRune, 'j') // api -> db
	if got := v.SelectedID(); got != "2" {
		t.Fatalf("selected %q, want 2", got)
	}
	// A new row sorts before the selected one; selection must stay on "db".
	v.SetRows(append(testRows(), resource.Row{ID: "9", Cells: []string{"aaa", "busybox", "running", "1s"}}))
	if got := v.SelectedID(); got != "2" {
		t.Fatalf("selection moved to %q after refresh", got)
	}
	// The selected row disappears; selection stays in range.
	v.SetRows(testRows()[:1])
	if got := v.SelectedID(); got != "1" {
		t.Fatalf("selected %q, want the only row", got)
	}
	v.SetRows(nil)
	if got := v.SelectedID(); got != "" {
		t.Fatalf("selected %q on an empty table", got)
	}
}

func TestTableNavigationKeys(t *testing.T) {
	v := newTableView("Containers", testCols)
	v.SetRows(testRows())
	press(v, tcell.KeyRune, 'G')
	if got := v.SelectedID(); got != "1" { // web is last
		t.Fatalf("G selected %q", got)
	}
	press(v, tcell.KeyRune, 'g')
	if got := v.SelectedID(); got != "3" { // api is first
		t.Fatalf("g selected %q", got)
	}
	press(v, tcell.KeyDown, 0)
	press(v, tcell.KeyUp, 0)
	if got := v.SelectedID(); got != "3" {
		t.Fatalf("down/up selected %q", got)
	}
}

func TestTableSortBindings(t *testing.T) {
	v := newTableView("Containers", testCols)
	v.SetRows(testRows())

	labels := map[string]string{}
	for _, b := range v.bindings() {
		labels[b.label] = b.desc
	}
	for _, want := range []string{"shift-n", "shift-i", "shift-s", "shift-a"} {
		if _, ok := labels[want]; !ok {
			t.Fatalf("no binding %s in %v", want, labels)
		}
	}

	press(v, tcell.KeyRune, 'A') // sort by AGE
	lines := render(t, v, 80, 10)
	if a, w, d := lineContaining(lines, "api"), lineContaining(lines, "web"), lineContaining(lines, "db "); a >= w || w >= d {
		t.Fatalf("not sorted by age:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.Contains(strings.Join(lines, "\n"), "AGE↑") {
		t.Fatalf("no sort indicator:\n%s", strings.Join(lines, "\n"))
	}
	press(v, tcell.KeyRune, 'A')
	if !strings.Contains(strings.Join(render(t, v, 80, 10), "\n"), "AGE↓") {
		t.Fatal("second press did not flip the direction")
	}
}

func TestSortKeysAreUniqueAndAvoidNavigation(t *testing.T) {
	cols := []resource.Column{{Name: "STATE"}, {Name: "STATUS"}, {Name: "GROUP"}, {Name: "SS"}}
	v := newTableView("X", cols)
	seen := map[rune]bool{}
	for _, b := range v.bindings() {
		if b.r == 'G' {
			t.Fatal("G is reserved for jump-to-end")
		}
		if seen[b.r] {
			t.Fatalf("key %c bound twice", b.r)
		}
		seen[b.r] = true
	}
	if len(seen) != 3 { // SS has no free letter left
		t.Fatalf("got %d sort keys, want 3", len(seen))
	}
}

func TestTable2000RowsStaysResponsive(t *testing.T) {
	v := newTableView("Many", testCols)
	rows := manyRows(2000)
	start := time.Now()
	v.SetRows(rows)
	for range 50 {
		press(v, tcell.KeyRune, 'j')
	}
	press(v, tcell.KeyRune, 'N')
	v.SetFilter("svc-00")
	render(t, v, 120, 40)
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("2000 rows took %s", d)
	}
}
