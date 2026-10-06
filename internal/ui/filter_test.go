package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
)

func filterHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, fastWatch,
		docker.Container{ID: "1", Name: "web", Image: "nginx:alpine", State: "running", Created: created},
		docker.Container{ID: "2", Name: "db", Image: "postgres:15", State: "exited", Status: "Exited (0)", Created: created},
		docker.Container{ID: "3", Name: "api", Image: "nginx:1.27", State: "running", Created: created},
	)
	h.show("containers")
	h.step()
	return h
}

func TestFilterAppliesWhileTyping(t *testing.T) {
	h := filterHarness(t)
	h.app.key(tcell.KeyRune, '/')
	h.app.typeText("ngi")
	if got := ids(h.app.view.shown); len(got) != 2 {
		t.Fatalf("filter not live, rows: %v", got)
	}
	if line := h.lastLine(); !strings.HasPrefix(strings.TrimSpace(line), "/ngi") {
		t.Fatalf("prompt not shown: %q", line)
	}
	h.app.key(tcell.KeyEnter, 0)
	s := h.screen()
	if !strings.Contains(s, "Containers[2/3]") || !strings.Contains(s, "</ngi>") || strings.Contains(s, "postgres") {
		t.Fatalf("filter not kept after enter:\n%s", s)
	}
	if h.app.prompting != promptNone {
		t.Fatal("prompt still open")
	}
}

func TestFilterVariants(t *testing.T) {
	tests := []struct {
		filter string
		want   int
	}{
		{"nginx", 2},
		{"!nginx", 1},
		{"^(web|db)$", 2},
		{"exited", 1},
		{"nomatch", 0},
	}
	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			h := filterHarness(t)
			h.app.key(tcell.KeyRune, '/')
			h.app.typeText(tt.filter)
			h.app.key(tcell.KeyEnter, 0)
			if got := len(h.app.view.shown); got != tt.want {
				t.Fatalf("got %d rows, want %d", got, tt.want)
			}
		})
	}
}

func TestFilterSurvivesRefresh(t *testing.T) {
	h := filterHarness(t)
	h.app.key(tcell.KeyRune, '/')
	h.app.typeText("nginx")
	h.app.key(tcell.KeyEnter, 0)

	h.fake.SetContainers(
		docker.Container{ID: "1", Name: "web", Image: "nginx:alpine", State: "running", Created: created},
		docker.Container{ID: "2", Name: "db", Image: "postgres:15", State: "running", Created: created},
		docker.Container{ID: "4", Name: "cache", Image: "nginx:alpine", State: "running", Created: created},
		docker.Container{ID: "5", Name: "queue", Image: "redis:7", State: "running", Created: created},
	)
	h.fake.Emit(docker.Event{Type: "container", Action: "start"})
	h.step()
	s := h.screen()
	if !strings.Contains(s, "Containers[2/4]") || !strings.Contains(s, "cache") || strings.Contains(s, "redis") {
		t.Fatalf("filter lost on refresh:\n%s", s)
	}
}

func TestEscClearsFilter(t *testing.T) {
	t.Run("on the table", func(t *testing.T) {
		h := filterHarness(t)
		h.app.key(tcell.KeyRune, '/')
		h.app.typeText("nginx")
		h.app.key(tcell.KeyEnter, 0)
		h.app.key(tcell.KeyEscape, 0)
		if s := h.screen(); !strings.Contains(s, "Containers[3]") || len(h.app.stack) != 1 {
			t.Fatalf("esc did not clear the filter:\n%s", s)
		}
	})
	t.Run("while typing", func(t *testing.T) {
		h := filterHarness(t)
		h.app.key(tcell.KeyRune, '/')
		h.app.typeText("nginx")
		h.app.key(tcell.KeyEscape, 0)
		if s := h.screen(); !strings.Contains(s, "Containers[3]") || h.app.prompting != promptNone {
			t.Fatalf("esc did not clear the filter:\n%s", s)
		}
	})
}

func TestFilterPromptStartsWithCurrentFilter(t *testing.T) {
	h := filterHarness(t)
	h.app.key(tcell.KeyRune, '/')
	h.app.typeText("web")
	h.app.key(tcell.KeyEnter, 0)
	h.app.key(tcell.KeyRune, '/')
	if got := h.app.prompt.GetText(); got != "web" {
		t.Fatalf("prompt starts with %q", got)
	}
}

func TestFilterKeyIgnoredWithoutATable(t *testing.T) {
	a := NewApp(testInfo)
	a.Push(textPage("home", "body"))
	a.key(tcell.KeyRune, '/')
	if a.prompting != promptNone {
		t.Fatal("filter prompt opened on a page without a table")
	}
}
