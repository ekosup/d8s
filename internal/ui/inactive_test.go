package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
)

func TestHideAndShowInactiveContainers(t *testing.T) {
	h := newHarness(t, fastWatch,
		docker.Container{ID: "1", Name: "web", State: "running", Status: "Up", Created: created},
		docker.Container{ID: "2", Name: "old-job", State: "exited", Status: "Exited (0)", Created: created},
		docker.Container{ID: "3", Name: "crashed", State: "exited", Status: "Exited (1)", Created: created},
	)
	h.show("containers")
	h.step()
	if s := h.screen(); !strings.Contains(s, "Containers[3]") || strings.Contains(s, "active only") {
		t.Fatalf("expected everything, unlabelled:\n%s", s)
	}
	if row, _ := hintAt(headerLines(t, h.app, 120), "h", "Inactive"); row < 0 {
		t.Fatal("the toggle is not advertised in the header")
	}

	h.app.key(tcell.KeyRune, 'h')
	// No event and a one-hour poll: the table must refresh because of the key.
	h.until("inactive hidden", func() bool { return strings.Contains(h.screen(), "Containers(active only)[1]") })
	if s := h.screen(); strings.Contains(s, "old-job") || strings.Contains(s, "crashed") || !strings.Contains(s, "web") {
		t.Fatalf("wrong rows:\n%s", s)
	}
	if strings.Contains(h.screen(), ": done") {
		t.Fatalf("the toggle left a status message:\n%s", h.screen())
	}

	// A container that stops while hidden disappears on its own.
	h.fake.SetContainers(docker.Container{ID: "1", Name: "web", State: "exited", Status: "Exited (0)", Created: created})
	h.fake.Emit(docker.Event{Type: "container", Action: "die", ID: "1"})
	h.until("stopped container hidden", func() bool { return strings.Contains(h.screen(), "Containers(active only)[0]") })

	h.app.key(tcell.KeyRune, 'h')
	h.until("everything back", func() bool { return strings.Contains(h.screen(), "Containers[1]") })
	if !strings.Contains(h.screen(), "web") {
		t.Fatalf("row not back:\n%s", h.screen())
	}
}

func TestFilterKeepsTheNote(t *testing.T) {
	h := newHarness(t, fastWatch,
		docker.Container{ID: "1", Name: "web", State: "running", Created: created},
		docker.Container{ID: "2", Name: "api", State: "running", Created: created},
		docker.Container{ID: "3", Name: "old", State: "exited", Status: "Exited (0)", Created: created},
	)
	h.show("containers")
	h.step()
	h.app.key(tcell.KeyRune, 'h')
	h.until("hidden", func() bool { return strings.Contains(h.screen(), "(active only)[2]") })
	h.app.key(tcell.KeyRune, '/')
	h.app.typeText("web")
	h.app.key(tcell.KeyEnter, 0)
	if s := h.screen(); !strings.Contains(s, "Containers(active only)[1/2] </web>") {
		t.Fatalf("title should carry both the note and the filter:\n%s", s)
	}
}
