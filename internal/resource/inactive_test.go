package resource

import (
	"context"
	"slices"
	"testing"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

func mixedStateFake() *dockertest.Fake {
	return dockertest.NewFake(dockertest.WithContainers(
		docker.Container{ID: "1", Name: "up", State: "running", Created: now},
		docker.Container{ID: "2", Name: "frozen", State: "paused", Created: now},
		docker.Container{ID: "3", Name: "looping", State: "restarting", Created: now},
		docker.Container{ID: "4", Name: "done", State: "exited", Status: "Exited (0)", Created: now},
		docker.Container{ID: "5", Name: "crashed", State: "exited", Status: "Exited (137)", Created: now},
		docker.Container{ID: "6", Name: "new", State: "created", Created: now},
		docker.Container{ID: "7", Name: "broken", State: "dead", Created: now},
	))
}

func TestContainerInactiveToggle(t *testing.T) {
	f := mixedStateFake()
	res := Containers(fixedNow)
	toggle := actionByKey(t, res, "h")
	if toggle.Target == "" || toggle.Mutates || !toggle.Quiet || toggle.Confirm {
		t.Fatalf("the toggle must be a quiet, global, non-mutating action: %+v", toggle)
	}

	all := []string{"up", "frozen", "looping", "done", "crashed", "new", "broken"}
	if got := namesOf(t, res, f); !slices.Equal(got, all) {
		t.Fatalf("everything is shown by default, got %v", got)
	}
	if res.Note() != "" {
		t.Fatalf("no note expected while everything is shown, got %q", res.Note())
	}

	if err := toggle.Run(context.Background(), f, Row{}); err != nil {
		t.Fatal(err)
	}
	// Paused and restarting containers are alive; only stopped ones go.
	if got := namesOf(t, res, f); !slices.Equal(got, []string{"up", "frozen", "looping"}) {
		t.Fatalf("with inactive hidden, got %v", got)
	}
	if res.Note() != "active only" {
		t.Fatalf("note = %q", res.Note())
	}

	if err := toggle.Run(context.Background(), f, Row{}); err != nil {
		t.Fatal(err)
	}
	if got := namesOf(t, res, f); !slices.Equal(got, all) || res.Note() != "" {
		t.Fatalf("second press should show everything again, got %v, note %q", got, res.Note())
	}
}

func TestInactiveToggleIsPerView(t *testing.T) {
	f := mixedStateFake()
	a, b := Containers(fixedNow), Containers(fixedNow)
	if err := actionByKey(t, a, "h").Run(context.Background(), f, Row{}); err != nil {
		t.Fatal(err)
	}
	if got := namesOf(t, b, f); len(got) != 7 {
		t.Fatalf("hiding in one view changed another: %v", got)
	}
}

func TestInactiveToggleSurvivesStatsDecoration(t *testing.T) {
	f := mixedStateFake()
	res := WithStats(Containers(fixedNow))
	if err := actionByKey(t, res, "h").Run(context.Background(), f, Row{}); err != nil {
		t.Fatal(err)
	}
	if got := namesOf(t, res, f); !slices.Equal(got, []string{"up", "frozen", "looping"}) || res.Note() != "active only" {
		t.Fatalf("got %v, note %q", got, res.Note())
	}
}
