package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

func markHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, fastWatch,
		docker.Container{ID: "1", Name: "api", Image: "nginx", State: "running", Status: "Up", Created: created},
		docker.Container{ID: "2", Name: "db", Image: "postgres", State: "running", Status: "Up", Created: created},
		docker.Container{ID: "3", Name: "web", Image: "nginx", State: "running", Status: "Up", Created: created},
		docker.Container{ID: "4", Name: "worker", Image: "app", State: "running", Status: "Up", Created: created},
	)
	h.show("containers")
	h.step()
	return h
}

func TestSpaceMarksAndMovesDown(t *testing.T) {
	h := markHarness(t)
	h.app.key(tcell.KeyRune, ' ') // api, selection -> db
	h.app.key(tcell.KeyRune, 'j') // -> web
	h.app.key(tcell.KeyRune, ' ') // web, selection -> worker
	if got := ids(h.app.view.MarkedRows()); !slices.Equal(got, []string{"1", "3"}) {
		t.Fatalf("marked: %v", got)
	}
	if got := h.app.view.SelectedID(); got != "4" {
		t.Fatalf("selection on %q", got)
	}
	if s := h.screen(); !strings.Contains(s, "2 marked") {
		t.Fatalf("mark count not shown:\n%s", s)
	}
	// Space on a marked row unmarks it.
	h.app.key(tcell.KeyRune, 'g')
	h.app.key(tcell.KeyRune, ' ')
	if got := ids(h.app.view.MarkedRows()); !slices.Equal(got, []string{"3"}) {
		t.Fatalf("after unmark: %v", got)
	}
}

func TestBulkActionNamesEveryTarget(t *testing.T) {
	h := markHarness(t)
	h.app.key(tcell.KeyRune, ' ')
	h.app.key(tcell.KeyRune, ' ')
	h.app.key(tcell.KeyRune, ' ') // api, db, web marked; selection on worker
	h.app.key(tcell.KeyCtrlD, 0)
	s := h.screen()
	if !strings.Contains(s, "Delete 3 items: api, db, web?") {
		t.Fatalf("dialog does not name the targets:\n%s", s)
	}
	h.app.key(tcell.KeyRune, 'y')
	h.until("rows removed", func() bool { return strings.Contains(h.screen(), "Containers[1]") })

	want := []dockertest.Call{{Op: docker.OpRemove, ID: "1"}, {Op: docker.OpRemove, ID: "2"}, {Op: docker.OpRemove, ID: "3"}}
	if got := h.fake.Calls(); !slices.Equal(got, want) {
		t.Fatalf("calls: %v", got)
	}
	if !strings.Contains(h.screen(), "worker") || len(h.app.view.MarkedRows()) != 0 {
		t.Fatalf("the unmarked row was touched or marks were kept:\n%s", h.screen())
	}
}

func TestBulkActionWithoutConfirm(t *testing.T) {
	h := markHarness(t)
	h.app.key(tcell.KeyRune, ' ')
	h.app.key(tcell.KeyRune, ' ')
	h.app.key(tcell.KeyRune, 'r')
	h.until("both restarted", func() bool { return len(h.fake.Calls()) == 2 })
	h.until("result", func() bool { return strings.Contains(h.screen(), "Restart api, db: done") })
}

func TestMarksSurviveRefreshAndFilterButNotEsc(t *testing.T) {
	h := markHarness(t)
	h.app.key(tcell.KeyRune, ' ')
	h.fake.Emit(docker.Event{Type: "container", Action: "start"})
	h.step()
	if got := ids(h.app.view.MarkedRows()); !slices.Equal(got, []string{"1"}) {
		t.Fatalf("mark lost on refresh: %v", got)
	}

	// A marked row hidden by the filter is not acted on.
	h.app.key(tcell.KeyRune, '/')
	h.app.typeText("postgres")
	h.app.key(tcell.KeyEnter, 0)
	if got := ids(h.app.view.MarkedRows()); len(got) != 0 {
		t.Fatalf("hidden row still a target: %v", got)
	}

	h.app.key(tcell.KeyEscape, 0) // clears the filter
	if got := ids(h.app.view.MarkedRows()); !slices.Equal(got, []string{"1"}) {
		t.Fatalf("mark lost when the filter was cleared: %v", got)
	}
	h.app.key(tcell.KeyEscape, 0) // clears the marks
	if got := ids(h.app.view.MarkedRows()); len(got) != 0 || len(h.app.stack) != 1 {
		t.Fatalf("esc did not clear marks: %v", got)
	}
}

func TestMarkOfRemovedRowIsDropped(t *testing.T) {
	h := markHarness(t)
	h.app.key(tcell.KeyRune, ' ')
	h.fake.SetContainers(docker.Container{ID: "2", Name: "db", State: "running", Created: created})
	h.fake.Emit(docker.Event{Type: "container", Action: "destroy"})
	h.step()
	if s := h.screen(); strings.Contains(s, "marked") {
		t.Fatalf("mark of a vanished row still counted:\n%s", s)
	}
}
