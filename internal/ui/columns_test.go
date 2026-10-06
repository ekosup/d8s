package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
)

// memorySorts is an in-memory SortStore.
type memorySorts struct {
	saved map[string][2]string
	saves int
}

func (m *memorySorts) Sort(view string) (string, bool, bool) {
	v, ok := m.saved[view]
	return v[0], v[1] == "desc", ok
}

func (m *memorySorts) SetSort(view, column string, desc bool) error {
	if m.saved == nil {
		m.saved = map[string][2]string{}
	}
	dir := "asc"
	if desc {
		dir = "desc"
	}
	m.saved[view] = [2]string{column, dir}
	m.saves++
	return nil
}

func columnHarness(t *testing.T, sorts SortStore, views map[string][]string) (*harness, []string) {
	t.Helper()
	h := newHarness(t, fastWatch,
		docker.Container{ID: "1", Name: "api", Image: "nginx", State: "running", Status: "Up", Created: created},
		docker.Container{ID: "2", Name: "web", Image: "apache", State: "exited", Status: "Exited (0)", Created: created},
	)
	h.app.sorts = sorts
	warnings := h.app.SetViews(views)
	h.show("containers")
	h.step()
	return h, warnings
}

func TestConfiguredColumnsAreShown(t *testing.T) {
	h, warnings := columnHarness(t, nil, map[string][]string{"containers": {"NAME", "STATE"}})
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	s := h.screen()
	if !strings.Contains(s, "NAME") || !strings.Contains(s, "STATE") || strings.Contains(s, "IMAGE") || strings.Contains(s, "nginx") {
		t.Fatalf("columns not applied:\n%s", s)
	}
	// Actions still act on the right object and call it by its name.
	h.app.key(tcell.KeyCtrlD, 0)
	if !strings.Contains(h.screen(), "Delete api?") {
		t.Fatalf("dialog lost the row's name:\n%s", h.screen())
	}
}

func TestBadColumnConfigIsReportedAndIgnored(t *testing.T) {
	h, warnings := columnHarness(t, nil, map[string][]string{"containers": {"NAME", "COLOUR"}, "nosuchview": {"NAME"}})
	joined := strings.Join(warnings, "\n")
	if len(warnings) != 2 || !strings.Contains(joined, "COLOUR") || !strings.Contains(joined, "nosuchview") {
		t.Fatalf("warnings: %v", warnings)
	}
	if !strings.Contains(h.screen(), "IMAGE") {
		t.Fatalf("a bad column list should leave the view as it was:\n%s", h.screen())
	}
}

func TestSortIsRememberedPerView(t *testing.T) {
	sorts := &memorySorts{}
	h, _ := columnHarness(t, sorts, nil)
	h.app.key(tcell.KeyRune, 'I') // sort by IMAGE
	h.app.key(tcell.KeyRune, 'I') // descending
	if got := sorts.saved["containers"]; got != [2]string{"IMAGE", "desc"} {
		t.Fatalf("saved: %v", sorts.saved)
	}

	// A later session opens the view already sorted that way.
	h2, _ := columnHarness(t, sorts, nil)
	if s := h2.screen(); !strings.Contains(s, "IMAGE↓") {
		t.Fatalf("remembered sort not applied:\n%s", s)
	}
	lines := strings.Split(h2.screen(), "\n")
	if lineContaining(lines, "nginx") > lineContaining(lines, "apache") {
		t.Fatalf("rows not in the remembered order:\n%s", h2.screen())
	}
}

func TestRememberedSortForAHiddenColumnFallsBack(t *testing.T) {
	sorts := &memorySorts{saved: map[string][2]string{"containers": {"PORTS", "desc"}}}
	h, _ := columnHarness(t, sorts, map[string][]string{"containers": {"NAME", "STATE"}})
	if s := h.screen(); !strings.Contains(s, "NAME↑") {
		t.Fatalf("expected the default sort when the remembered column is gone:\n%s", s)
	}
}

func TestOpeningAViewDoesNotRewriteItsSort(t *testing.T) {
	sorts := &memorySorts{saved: map[string][2]string{"containers": {"IMAGE", "desc"}}}
	columnHarness(t, sorts, nil)
	if sorts.saves != 0 {
		t.Fatalf("%d writes just from opening a view", sorts.saves)
	}
}
