package resource

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

func cellsOf(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = strings.Join(r.Cells, " | ")
	}
	return out
}

func namesOf(t *testing.T, res Resource, f *dockertest.Fake) []string {
	t.Helper()
	rows, err := res.List(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Name()
	}
	return out
}

func volumeFake() *dockertest.Fake {
	f := dockertest.NewFake(dockertest.WithContainers(
		docker.Container{ID: "c1", Name: "db", State: "running", Created: now, Volumes: []string{"pgdata"}},
		docker.Container{ID: "c2", Name: "backup", State: "exited", Status: "Exited (0)", Created: now, Volumes: []string{"pgdata", "dumps"}},
		docker.Container{ID: "c3", Name: "web", State: "running", Created: now},
	))
	f.SetVolumes(
		docker.Volume{Name: "pgdata", Driver: "local", Created: now.Add(-72 * time.Hour)},
		docker.Volume{Name: "dumps", Driver: "local", Created: now.Add(-time.Hour)},
		docker.Volume{Name: "orphan", Driver: "local", Created: now.Add(-time.Minute)},
	)
	return f
}

func TestVolumeRows(t *testing.T) {
	res := Volumes(fixedNow)
	rows, err := res.List(context.Background(), volumeFake())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"pgdata | local | backup, db | 3d",
		"dumps | local | backup | 1h",
		"orphan | local |  | 1m",
	}
	if got := cellsOf(rows); !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
	if rows[2].Tone != ToneMuted || rows[0].Tone != ToneNormal {
		t.Fatal("unused volume should be dimmed")
	}
}

func TestVolumeActions(t *testing.T) {
	f := volumeFake()
	res := Volumes(fixedNow)
	rows, _ := res.List(context.Background(), f)

	del := actionByKey(t, res, "ctrl-d")
	if w := del.Warn(rows[0]); !strings.Contains(w, "backup, db") {
		t.Fatalf("no in-use warning: %q", w)
	}
	if w := del.Warn(rows[2]); w != "" {
		t.Fatalf("warning for an unused volume: %q", w)
	}
	if err := del.Run(context.Background(), f, rows[2]); err != nil {
		t.Fatal(err)
	}
	prune := actionByKey(t, res, "ctrl-p")
	if prune.Target == "" {
		t.Fatal("prune must be global")
	}
	if err := prune.Run(context.Background(), f, Row{}); err != nil {
		t.Fatal(err)
	}
	if got := f.Log(); !slices.Equal(got, []string{"remove volume orphan", "prune volume"}) {
		t.Fatalf("log: %v", got)
	}
}

func TestVolumeOpenListsItsContainers(t *testing.T) {
	f := volumeFake()
	res := Volumes(fixedNow)
	rows, _ := res.List(context.Background(), f)
	child, ok := res.Open(rows[0])
	if !ok {
		t.Fatal("no drill-down")
	}
	if got := namesOf(t, child, f); !slices.Equal(got, []string{"db", "backup"}) {
		t.Fatalf("children: %v", got)
	}
}
