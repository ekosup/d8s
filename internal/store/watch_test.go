package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
	"github.com/ekosup/d8s/internal/resource"
)

var containers = resource.Resource{
	Name:       "containers",
	Columns:    []resource.Column{{Name: "NAME"}},
	EventTypes: []string{"container"},
	List: func(ctx context.Context, c docker.Client) ([]resource.Row, error) {
		cs, err := c.Containers(ctx)
		if err != nil {
			return nil, err
		}
		rows := make([]resource.Row, len(cs))
		for i, x := range cs {
			rows[i] = resource.Row{ID: x.ID, Cells: []string{x.Name}}
		}
		return rows, nil
	},
}

// recorder collects snapshots delivered by a watcher.
type recorder struct {
	mu    sync.Mutex
	snaps []Snapshot
	ch    chan Snapshot
}

func newRecorder() *recorder { return &recorder{ch: make(chan Snapshot, 256)} }

func (r *recorder) add(s Snapshot) {
	r.mu.Lock()
	r.snaps = append(r.snaps, s)
	r.mu.Unlock()
	r.ch <- s
}

func (r *recorder) next(t *testing.T) Snapshot {
	t.Helper()
	select {
	case s := <-r.ch:
		return s
	case <-time.After(2 * time.Second):
		t.Fatal("no snapshot within 2s")
		return Snapshot{}
	}
}

func (r *recorder) quiet(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case s := <-r.ch:
		t.Fatalf("unexpected snapshot from %s", s.Source)
	case <-time.After(d):
	}
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not stop")
	}
}

const never = time.Hour

func TestWatchDeliversInitialSnapshot(t *testing.T) {
	fake := dockertest.NewFake(dockertest.WithContainers(docker.Container{ID: "1", Name: "web"}))
	rec := newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	done := Watch(ctx, fake, containers, Options{Debounce: time.Millisecond, Poll: never}, rec.add)

	s := rec.next(t)
	if s.Err != nil || len(s.Rows) != 1 || s.Rows[0].ID != "1" || s.Source != SourceInitial {
		t.Fatalf("got %+v", s)
	}
	cancel()
	waitDone(t, done)
}

func TestWatchRefreshesOnRelevantEvent(t *testing.T) {
	fake := dockertest.NewFake()
	rec := newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Watch(ctx, fake, containers, Options{Debounce: 5 * time.Millisecond, Poll: never}, rec.add)
	rec.next(t)

	fake.SetContainers(docker.Container{ID: "1", Name: "web"})
	fake.Emit(docker.Event{Type: "container", Action: "start", ID: "1"})
	s := rec.next(t)
	if len(s.Rows) != 1 || s.Source != SourceEvent {
		t.Fatalf("got %+v", s)
	}

	// Events of other types do not trigger a refresh.
	fake.Emit(docker.Event{Type: "image", Action: "pull"})
	rec.quiet(t, 60*time.Millisecond)
}

func TestWatchDebouncesBursts(t *testing.T) {
	fake := dockertest.NewFake()
	rec := newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Watch(ctx, fake, containers, Options{Debounce: 40 * time.Millisecond, Poll: never}, rec.add)
	rec.next(t)
	before := fake.ListCalls()

	for range 20 {
		fake.Emit(docker.Event{Type: "container", Action: "start"})
	}
	rec.next(t)
	rec.quiet(t, 100*time.Millisecond)
	if got := fake.ListCalls() - before; got != 1 {
		t.Fatalf("20 events caused %d list calls, want 1", got)
	}
}

func TestWatchPollsAsFallback(t *testing.T) {
	polled := resource.Resource{Name: "polled", Columns: containers.Columns, List: containers.List} // no event types
	fake := dockertest.NewFake()
	rec := newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Watch(ctx, fake, polled, Options{Debounce: time.Millisecond, Poll: 20 * time.Millisecond}, rec.add)
	rec.next(t)
	if s := rec.next(t); s.Source != SourcePoll {
		t.Fatalf("got source %s", s.Source)
	}
	if fake.EventCalls() != 0 {
		t.Fatal("subscribed to events for a resource that has no event types")
	}
}

func TestWatchResubscribesAfterEventStreamError(t *testing.T) {
	fake := dockertest.NewFake()
	rec := newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Watch(ctx, fake, containers, Options{Debounce: time.Millisecond, Poll: never, Retry: 5 * time.Millisecond}, rec.add)
	rec.next(t)

	fake.FailEvents(errors.New("connection reset"))
	deadline := time.Now().Add(2 * time.Second)
	for fake.EventCalls() < 2 {
		if time.Now().After(deadline) {
			t.Fatal("watcher did not subscribe again")
		}
		time.Sleep(2 * time.Millisecond)
	}
	fake.Emit(docker.Event{Type: "container", Action: "die"})
	// One refresh for the reconnect, one for the event; either way events flow again.
	if s := rec.next(t); s.Err != nil {
		t.Fatalf("got %+v", s)
	}
}

func TestWatchReportsListError(t *testing.T) {
	fake := dockertest.NewFake()
	boom := errors.New("boom")
	fake.SetListError(boom)
	rec := newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Watch(ctx, fake, containers, Options{Debounce: time.Millisecond, Poll: 20 * time.Millisecond}, rec.add)
	if s := rec.next(t); !errors.Is(s.Err, boom) {
		t.Fatalf("got %+v", s)
	}
	fake.SetListError(nil)
	for {
		if s := rec.next(t); s.Err == nil {
			return // recovered on a later poll
		}
	}
}

func TestWatchStopsAndStaysSilentAfterCancel(t *testing.T) {
	fake := dockertest.NewFake()
	rec := newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	done := Watch(ctx, fake, containers, Options{Debounce: time.Millisecond, Poll: 5 * time.Millisecond}, rec.add)
	rec.next(t)
	cancel()
	waitDone(t, done)

	for len(rec.ch) > 0 {
		<-rec.ch
	}
	calls := fake.ListCalls()
	fake.Emit(docker.Event{Type: "container", Action: "start"})
	rec.quiet(t, 50*time.Millisecond)
	if fake.ListCalls() != calls {
		t.Fatal("watcher kept listing after it was stopped")
	}
}
