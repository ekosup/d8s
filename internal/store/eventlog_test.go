package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestEventLogKeepsNewestEvents(t *testing.T) {
	fake := dockertest.NewFake()
	log := NewEventLog(3)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); log.Run(ctx, fake, time.Millisecond) }()
	waitFor(t, "subscription", func() bool { return fake.EventCalls() == 1 })

	for _, a := range []string{"create", "start", "die", "destroy"} {
		fake.Emit(docker.Event{Type: "container", Action: a, ID: "1"})
	}
	waitFor(t, "events", func() bool { return len(log.Recent()) == 3 && log.Recent()[2].Action == "destroy" })
	got := log.Recent()
	if got[0].Action != "start" || got[1].Action != "die" {
		t.Fatalf("kept %+v", got)
	}

	// Recent returns a copy: callers cannot corrupt the log.
	got[0].Action = "tampered"
	if log.Recent()[0].Action != "start" {
		t.Fatal("Recent exposed the internal slice")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestEventLogResubscribes(t *testing.T) {
	fake := dockertest.NewFake()
	log := NewEventLog(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go log.Run(ctx, fake, time.Millisecond)
	waitFor(t, "subscription", func() bool { return fake.EventCalls() == 1 })

	fake.FailEvents(errors.New("connection reset"))
	waitFor(t, "second subscription", func() bool { return fake.EventCalls() >= 2 })
	fake.Emit(docker.Event{Type: "image", Action: "pull"})
	waitFor(t, "event after reconnect", func() bool { return len(log.Recent()) == 1 })
}

func TestWatchWildcardRefreshesOnAnyEvent(t *testing.T) {
	all := containers
	all.EventTypes = []string{"*"}
	fake := dockertest.NewFake()
	rec := newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Watch(ctx, fake, all, Options{Debounce: time.Millisecond, Poll: never}, rec.add)
	rec.next(t)
	fake.Emit(docker.Event{Type: "volume", Action: "create"})
	if s := rec.next(t); s.Source != SourceEvent {
		t.Fatalf("source %s", s.Source)
	}
}
