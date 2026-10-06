package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
	"github.com/ekosup/d8s/internal/resource"
	"github.com/ekosup/d8s/internal/store"
)

var fastWatch = store.Options{Debounce: time.Millisecond, Poll: time.Hour, Retry: time.Millisecond}

// harness runs an App without the tview event loop: UI updates queued by
// the watcher are executed on the test goroutine.
type harness struct {
	t     *testing.T
	app   *App
	fake  *dockertest.Fake
	queue chan func()
}

func newHarness(t *testing.T, opts store.Options, cs ...docker.Container) *harness {
	t.Helper()
	reg, err := resource.Default(func() time.Time { return time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, fake: dockertest.NewFake(dockertest.WithContainers(cs...)), queue: make(chan func(), 256)}
	h.app = NewApp(testInfo, WithClient(h.fake), WithRegistry(reg), WithWatchOptions(opts))
	h.app.queue = func(f func()) { h.queue <- f }
	t.Cleanup(h.app.stopWatch)
	return h
}

// step runs the next queued UI update, failing if none arrives.
func (h *harness) step() {
	h.t.Helper()
	select {
	case f := <-h.queue:
		f()
	case <-time.After(2 * time.Second):
		h.t.Fatal("no UI update within 2s")
	}
}

func (h *harness) screen() string {
	h.t.Helper()
	return screenOf(h.t, h.app, 120, 24)
}

func (h *harness) show(cmd string) {
	h.t.Helper()
	res, err := h.app.registry.Lookup(cmd)
	if err != nil {
		h.t.Fatal(err)
	}
	h.app.ShowResource(res)
}

var created = time.Date(2026, 10, 6, 11, 0, 0, 0, time.UTC)

func TestContainerViewShowsContainers(t *testing.T) {
	h := newHarness(t, fastWatch,
		docker.Container{ID: "1", Name: "web", Image: "nginx:alpine", State: "running", Status: "Up 1 hour", Created: created},
		docker.Container{ID: "2", Name: "job", Image: "busybox", State: "exited", Status: "Exited (0) 1 hour ago", Created: created},
	)
	h.show("containers")
	h.step()
	s := h.screen()
	for _, want := range []string{"Containers[2]", "NAME", "STATUS", "PORTS", "AGE", "web", "nginx:alpine", "Up 1 hour", "job", "1h", "<containers>"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
}

func TestContainerViewUpdatesLive(t *testing.T) {
	h := newHarness(t, fastWatch)
	h.show("containers")
	h.step()
	if !strings.Contains(h.screen(), "Containers[0]") {
		t.Fatalf("expected an empty table:\n%s", h.screen())
	}

	// A container appears; no key is pressed.
	h.fake.SetContainers(docker.Container{ID: "1", Name: "d8s-demo-web", Image: "nginx:alpine", State: "running", Status: "Up 1 second", Created: created})
	h.fake.Emit(docker.Event{Type: "container", Action: "start", ID: "1"})
	h.step()
	if s := h.screen(); !strings.Contains(s, "d8s-demo-web") || !strings.Contains(s, "running") {
		t.Fatalf("new container not shown:\n%s", s)
	}

	// Its state changes.
	h.fake.SetContainers(docker.Container{ID: "1", Name: "d8s-demo-web", Image: "nginx:alpine", State: "exited", Status: "Exited (0) 1 second ago", Created: created})
	h.fake.Emit(docker.Event{Type: "container", Action: "die", ID: "1"})
	h.step()
	if s := h.screen(); !strings.Contains(s, "exited") {
		t.Fatalf("state change not shown:\n%s", s)
	}

	// It is removed.
	h.fake.SetContainers()
	h.fake.Emit(docker.Event{Type: "container", Action: "destroy", ID: "1"})
	h.step()
	if s := h.screen(); strings.Contains(s, "d8s-demo-web") {
		t.Fatalf("removed container still shown:\n%s", s)
	}
}

func TestContainerViewKeepsRowsOnRefreshError(t *testing.T) {
	opts := fastWatch
	opts.Poll = 10 * time.Millisecond
	h := newHarness(t, opts, docker.Container{ID: "1", Name: "web", State: "running", Created: created})
	h.show("containers")
	h.step()

	h.fake.SetListError(errors.New("connection refused"))
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(h.screen(), "connection refused") {
		if time.Now().After(deadline) {
			t.Fatalf("error never shown:\n%s", h.screen())
		}
		h.step()
	}
	if s := h.screen(); !strings.Contains(s, "web") {
		t.Fatalf("rows dropped on error:\n%s", s)
	}

	h.fake.SetListError(nil)
	for strings.Contains(h.screen(), "connection refused") {
		if time.Now().After(deadline) {
			t.Fatalf("error never cleared:\n%s", h.screen())
		}
		h.step()
	}
}

func TestSwitchingViewStopsTheOldWatcher(t *testing.T) {
	h := newHarness(t, fastWatch, docker.Container{ID: "1", Name: "web", State: "running", Created: created})
	other := resource.Resource{
		Name:    "things",
		Title:   "Things",
		Columns: []resource.Column{{Name: "THING"}, {Name: "SIZE", Right: true}},
		List: func(context.Context, docker.Client) ([]resource.Row, error) {
			return []resource.Row{{ID: "t1", Cells: []string{"alpha", "42"}}}, nil
		},
	}

	h.show("containers")
	h.step()
	oldDone := h.app.watchDone

	// A resource the view code has never heard of renders through the same path.
	h.app.ShowResource(other)
	select {
	case <-oldDone:
	case <-time.After(2 * time.Second):
		t.Fatal("old watcher still running after switching views")
	}
	h.step()
	s := h.screen()
	for _, want := range []string{"Things[1]", "THING", "SIZE", "alpha", "42", "<things>"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "web") || len(h.app.stack) != 1 {
		t.Fatalf("old view still present:\n%s", s)
	}
}

func TestStaleSnapshotIsIgnored(t *testing.T) {
	h := newHarness(t, fastWatch)
	h.show("containers")
	h.step()
	oldGen, oldView := h.app.gen, h.app.view
	h.show("containers")
	h.step()
	h.app.applySnapshot(oldGen, oldView, store.Snapshot{Rows: []resource.Row{{ID: "x", Cells: []string{"ghost"}}}})
	if strings.Contains(h.screen(), "ghost") {
		t.Fatal("snapshot from a closed view was drawn")
	}
}
