package action

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
	"github.com/ekosup/d8s/internal/resource"
)

func rows(names ...string) []resource.Row {
	out := make([]resource.Row, len(names))
	for i, n := range names {
		out[i] = resource.Row{ID: "id-" + n, Cells: []string{n}}
	}
	return out
}

func TestExecutorRunsEveryRow(t *testing.T) {
	var seen []string
	act := resource.Action{Name: "Restart", Mutates: true, Run: func(_ context.Context, _ docker.Client, r resource.Row) error {
		seen = append(seen, r.ID)
		return nil
	}}
	if err := New(dockertest.NewFake(), false).Run(context.Background(), act, rows("a", "b")); err != nil {
		t.Fatal(err)
	}
	if strings.Join(seen, ",") != "id-a,id-b" {
		t.Fatalf("ran on %v", seen)
	}
}

func TestExecutorCollectsErrorsAndKeepsGoing(t *testing.T) {
	boom := errors.New("boom")
	ran := 0
	act := resource.Action{Name: "Stop", Mutates: true, Run: func(_ context.Context, _ docker.Client, r resource.Row) error {
		ran++
		if r.ID == "id-a" {
			return boom
		}
		return nil
	}}
	err := New(dockertest.NewFake(), false).Run(context.Background(), act, rows("a", "b"))
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "a:") || ran != 2 {
		t.Fatalf("err=%v ran=%d", err, ran)
	}
}

func TestExecutorReadOnly(t *testing.T) {
	ran := false
	run := func(context.Context, docker.Client, resource.Row) error { ran = true; return nil }
	ex := New(dockertest.NewFake(), true)

	err := ex.Run(context.Background(), resource.Action{Name: "Delete", Mutates: true, Run: run}, rows("a"))
	if !errors.Is(err, ErrReadOnly) || ran {
		t.Fatalf("mutating action in read-only mode: err=%v ran=%v", err, ran)
	}
	if err := ex.Run(context.Background(), resource.Action{Name: "Peek", Run: run}, rows("a")); err != nil || !ran {
		t.Fatalf("non-mutating action blocked: err=%v ran=%v", err, ran)
	}
}

func TestExecutorNothingSelected(t *testing.T) {
	act := resource.Action{Name: "Stop", Mutates: true, Run: func(context.Context, docker.Client, resource.Row) error { return nil }}
	if err := New(dockertest.NewFake(), false).Run(context.Background(), act, nil); !errors.Is(err, ErrNoTarget) {
		t.Fatalf("got %v", err)
	}
}
