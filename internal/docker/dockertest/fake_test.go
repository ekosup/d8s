package dockertest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

func TestFakeContainers(t *testing.T) {
	f := NewFake(WithContainers(docker.Container{ID: "1", Name: "a"}))
	got, err := f.Containers(context.Background())
	if err != nil || len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("got %v, %v", got, err)
	}
	f.SetContainers(docker.Container{ID: "2", Name: "b"}, docker.Container{ID: "3", Name: "c"})
	got, _ = f.Containers(context.Background())
	if len(got) != 2 || f.ListCalls() != 2 {
		t.Fatalf("got %d containers after %d calls", len(got), f.ListCalls())
	}
	boom := errors.New("boom")
	f.SetListError(boom)
	if _, err := f.Containers(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("got %v, want boom", err)
	}
}

func TestFakeEvents(t *testing.T) {
	f := NewFake()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	msgs, _ := f.Events(ctx)
	f.Emit(docker.Event{Type: "container", Action: "start", ID: "1"})
	select {
	case ev := <-msgs:
		if ev.Action != "start" {
			t.Fatalf("got %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no event delivered")
	}
}
