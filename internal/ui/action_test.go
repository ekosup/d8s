package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

// until runs queued UI updates until cond holds.
func (h *harness) until(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		select {
		case f := <-h.queue:
			f()
		case <-time.After(10 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s:\n%s", what, h.screen())
		}
	}
}

func actionHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, fastWatch,
		docker.Container{ID: "1", Name: "api", Image: "nginx", State: "running", Status: "Up", Created: created},
		docker.Container{ID: "2", Name: "web", Image: "nginx", State: "running", Status: "Up", Created: created},
	)
	h.show("containers")
	h.step()
	return h
}

func TestActionRunsOnSelectedRow(t *testing.T) {
	h := actionHarness(t)
	h.app.key(tcell.KeyRune, 'j') // web
	h.app.key(tcell.KeyRune, 'r')
	h.until("restart result", func() bool { return strings.Contains(h.screen(), "Restart web: done") })
	if got := h.fake.Calls(); !slices.Equal(got, []dockertest.Call{{Op: docker.OpRestart, ID: "2"}}) {
		t.Fatalf("calls: %v", got)
	}
}

func TestDestructiveActionAsksFirst(t *testing.T) {
	h := actionHarness(t)
	h.app.key(tcell.KeyCtrlD, 0)
	s := h.screen()
	if !strings.Contains(s, "Delete api?") || !strings.Contains(s, "<y>") {
		t.Fatalf("no confirmation dialog:\n%s", s)
	}
	if len(h.fake.Calls()) != 0 {
		t.Fatal("deleted before confirmation")
	}

	h.app.key(tcell.KeyRune, 'n')
	if strings.Contains(h.screen(), "Delete api?") || len(h.fake.Calls()) != 0 || len(h.app.stack) != 1 {
		t.Fatal("n did not cancel")
	}

	h.app.key(tcell.KeyCtrlD, 0)
	h.app.key(tcell.KeyEscape, 0)
	if len(h.fake.Calls()) != 0 || len(h.app.stack) != 1 {
		t.Fatal("esc did not cancel")
	}

	h.app.key(tcell.KeyCtrlD, 0)
	h.app.key(tcell.KeyRune, 'y')
	h.until("row removed", func() bool { return strings.Contains(h.screen(), "Containers[1]") })
	if got := h.fake.Calls(); !slices.Equal(got, []dockertest.Call{{Op: docker.OpRemove, ID: "1"}}) {
		t.Fatalf("calls: %v", got)
	}
	if !strings.Contains(h.screen(), "Delete api: done") {
		t.Fatalf("no result message:\n%s", h.screen())
	}
}

func TestOtherKeysDoNotLeakThroughDialog(t *testing.T) {
	h := actionHarness(t)
	h.app.key(tcell.KeyCtrlK, 0)
	h.app.key(tcell.KeyRune, 'r') // would restart if it reached the table
	h.app.key(tcell.KeyRune, ':')
	if len(h.fake.Calls()) != 0 || h.app.prompting != promptNone {
		t.Fatal("keys reached the page under the dialog")
	}
}

func TestActionErrorIsShown(t *testing.T) {
	h := actionHarness(t)
	h.fake.SetActionError(errors.New("container is not running"))
	h.app.key(tcell.KeyRune, 'x')
	h.until("error message", func() bool { return strings.Contains(h.screen(), "Stop api: container is not running") })
}

func TestActionWithNoRows(t *testing.T) {
	h := newHarness(t, fastWatch)
	h.show("containers")
	h.step()
	h.app.key(tcell.KeyRune, 'r')
	h.app.key(tcell.KeyCtrlD, 0)
	if len(h.fake.Calls()) != 0 || len(h.app.stack) != 1 {
		t.Fatal("action ran with nothing selected")
	}
}

func TestHeaderShowsViewActions(t *testing.T) {
	h := actionHarness(t)
	lines := headerLines(t, h.app, 140)
	for _, want := range [][2]string{{"r", "Restart"}, {"x", "Stop"}, {"ctrl-d", "Delete"}, {"ctrl-k", "Kill"}} {
		if row, _ := hintAt(lines, want[0], want[1]); row < 0 {
			t.Fatalf("missing <%s> %s in:\n%s", want[0], want[1], strings.Join(lines, "\n"))
		}
	}
	// Help lists them too.
	h.app.key(tcell.KeyRune, '?')
	if s := screenOf(t, h.app, 140, 30); !strings.Contains(s, "<ctrl-d>") || !strings.Contains(s, "Delete") {
		t.Fatalf("actions missing from help:\n%s", s)
	}
}
