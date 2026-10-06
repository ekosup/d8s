package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
)

func imageHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, fastWatch,
		docker.Container{ID: "c1", Name: "web", Image: "nginx:alpine", ImageID: "sha256:aaaaaaaaaaaaaaaa", State: "running", Created: created},
		docker.Container{ID: "c2", Name: "db", Image: "postgres", ImageID: "sha256:cccccccccccccccc", State: "running", Created: created},
	)
	h.fake.SetImages(
		docker.Image{ID: "sha256:aaaaaaaaaaaaaaaa", Tags: []string{"nginx:alpine"}, Size: 45 << 20, Created: created},
		docker.Image{ID: "sha256:bbbbbbbbbbbbbbbb", Size: 1 << 20, Created: created},
	)
	h.register(resource.Images(func() time.Time { return created.Add(time.Hour) }))
	h.show("images")
	h.step()
	return h
}

func TestImagesView(t *testing.T) {
	h := imageHarness(t)
	s := h.screen()
	for _, want := range []string{"Images[2]", "REPOSITORY", "TAG", "SIZE", "USED", "nginx", "alpine", "45.0MiB", "<none>", "<images>"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
}

func TestEnterDrillsDownAndEscReturns(t *testing.T) {
	h := imageHarness(t)
	h.app.key(tcell.KeyRune, 'j') // nginx (sorted after <none>)
	h.app.key(tcell.KeyEnter, 0)
	h.until("child view", func() bool { return strings.Contains(h.screen(), "web") })
	s := h.screen()
	if !strings.Contains(s, "<images> <containers>") || !strings.Contains(s, "nginx:alpine") || strings.Contains(s, "postgres") {
		t.Fatalf("drill-down wrong:\n%s", s)
	}
	// The child is a full container view: its actions work.
	h.app.key(tcell.KeyRune, 'r')
	h.until("restart", func() bool { return len(h.fake.Calls()) == 1 })
	if got := h.fake.Calls()[0]; got.Op != docker.OpRestart || got.ID != "c1" {
		t.Fatalf("call: %+v", got)
	}

	h.app.key(tcell.KeyEscape, 0)
	h.until("parent view", func() bool { return strings.Contains(h.screen(), "Images[2]") })
	if len(h.app.stack) != 1 {
		t.Fatalf("stack depth %d", len(h.app.stack))
	}

	// The parent is live again: a new image appears without a key press.
	h.fake.SetImages(docker.Image{ID: "sha256:dddddddddddddddd", Tags: []string{"redis:7"}, Size: 1, Created: created})
	h.fake.Emit(docker.Event{Type: "image", Action: "pull"})
	h.until("parent refreshed", func() bool { return strings.Contains(h.screen(), "redis") })
}

func TestOnlyTheVisibleViewIsRefreshed(t *testing.T) {
	h := imageHarness(t)
	h.app.key(tcell.KeyRune, 'j')
	h.app.key(tcell.KeyEnter, 0)
	h.until("child view", func() bool {
		return strings.Contains(h.screen(), "<images> <containers>") && strings.Contains(h.screen(), "web")
	})

	// An image event must not reach the covered image view.
	h.fake.SetImages()
	h.fake.Emit(docker.Event{Type: "image", Action: "delete"})
	h.app.key(tcell.KeyEscape, 0)
	// Back on the parent it refreshes on its own and shows the new state.
	h.until("parent caught up", func() bool { return strings.Contains(h.screen(), "Images[0]") })
}

func TestGlobalActionAndWarning(t *testing.T) {
	h := imageHarness(t)
	h.fake.SetPruneReport(docker.PruneReport{Count: 2, Reclaimed: 3 << 20})

	h.app.key(tcell.KeyCtrlP, 0)
	if s := h.screen(); !strings.Contains(s, "Prune dangling images?") {
		t.Fatalf("no prune confirmation:\n%s", s)
	}
	h.app.key(tcell.KeyRune, 'y')
	h.until("prune", func() bool { return slices.Contains(h.fake.Log(), "prune image") })

	h.app.key(tcell.KeyRune, 'j') // nginx, used by one container
	h.app.key(tcell.KeyCtrlD, 0)
	if s := h.screen(); !strings.Contains(s, "Delete nginx:alpine?") || !strings.Contains(s, "used by 1 container") {
		t.Fatalf("no in-use warning in the dialog:\n%s", s)
	}
	h.app.key(tcell.KeyRune, 'n')
}

func TestTextPage(t *testing.T) {
	h := imageHarness(t)
	h.fake.SetImageHistory("sha256:aaaaaaaaaaaaaaaa", docker.ImageLayer{ID: "x", CreatedBy: "RUN apk add nginx", Size: 1 << 20, Created: created})
	h.app.key(tcell.KeyRune, 'j')
	h.app.key(tcell.KeyRune, 'h')
	h.until("history page", func() bool { return len(h.app.stack) == 2 })
	if s := h.screen(); !strings.Contains(s, "History: nginx:alpine") || !strings.Contains(s, "RUN apk add nginx") || !strings.Contains(s, "<images> <history>") {
		t.Fatalf("history page wrong:\n%s", s)
	}
}
