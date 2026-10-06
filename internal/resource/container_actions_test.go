package resource

import (
	"context"
	"slices"
	"testing"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

func actionByKey(t *testing.T, res Resource, key string) Action {
	t.Helper()
	for _, a := range res.Actions {
		if a.Key == key {
			return a
		}
	}
	t.Fatalf("no action on key %q", key)
	return Action{}
}

func TestContainerActions(t *testing.T) {
	tests := []struct {
		key     string
		state   string
		wantOp  docker.ContainerOp
		confirm bool
	}{
		{"a", "exited", docker.OpStart, false},
		{"x", "running", docker.OpStop, false},
		{"r", "running", docker.OpRestart, false},
		{"p", "running", docker.OpPause, false},
		{"p", "paused", docker.OpUnpause, false},
		{"ctrl-k", "running", docker.OpKill, true},
		{"ctrl-d", "running", docker.OpRemove, true},
	}
	for _, tt := range tests {
		t.Run(tt.key+"/"+tt.state, func(t *testing.T) {
			fake := dockertest.NewFake(dockertest.WithContainers(docker.Container{ID: "1", Name: "web", State: tt.state, Created: now}))
			res := Containers(fixedNow)
			rows, err := res.List(context.Background(), fake)
			if err != nil {
				t.Fatal(err)
			}
			act := actionByKey(t, res, tt.key)
			if act.Confirm != tt.confirm || !act.Mutates {
				t.Fatalf("confirm=%v mutates=%v", act.Confirm, act.Mutates)
			}
			if err := act.Run(context.Background(), fake, rows[0]); err != nil {
				t.Fatal(err)
			}
			want := []dockertest.Call{{Op: tt.wantOp, ID: "1"}}
			if got := fake.Calls(); !slices.Equal(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}

func TestActionKeysAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Containers(fixedNow).Actions {
		if seen[a.Key] {
			t.Fatalf("key %q used twice", a.Key)
		}
		seen[a.Key] = true
	}
}
