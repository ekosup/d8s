package resource

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

func composeFake() *dockertest.Fake {
	lbl := func(project string) map[string]string {
		return map[string]string{"com.docker.compose.project": project, "com.docker.compose.project.working_dir": "/srv/" + project}
	}
	return dockertest.NewFake(dockertest.WithContainers(
		docker.Container{ID: "c1", Name: "shop-web-1", State: "running", Created: now, Labels: lbl("shop")},
		docker.Container{ID: "c2", Name: "shop-db-1", State: "exited", Status: "Exited (1)", Created: now, Labels: lbl("shop")},
		docker.Container{ID: "c3", Name: "blog-app-1", State: "running", Created: now, Labels: lbl("blog")},
		docker.Container{ID: "c4", Name: "old-job-1", State: "exited", Status: "Exited (0)", Created: now, Labels: lbl("old")},
		docker.Container{ID: "c5", Name: "standalone", State: "running", Created: now},
	))
}

func TestComposeRows(t *testing.T) {
	res := Compose(fixedNow)
	rows, err := res.List(context.Background(), composeFake())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"blog | 1 | 1/1 | /srv/blog",
		"old | 1 | 0/1 | /srv/old",
		"shop | 2 | 1/2 | /srv/shop",
	}
	if got := cellsOf(rows); !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
	if rows[0].Tone != ToneNormal || rows[1].Tone != ToneMuted || rows[2].Tone != ToneWarn {
		t.Fatalf("tones: %v %v %v", rows[0].Tone, rows[1].Tone, rows[2].Tone)
	}
}

func TestComposeActionsApplyToWholeProject(t *testing.T) {
	tests := []struct {
		key string
		op  docker.ContainerOp
	}{{"a", docker.OpStart}, {"x", docker.OpStop}, {"r", docker.OpRestart}}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			f := composeFake()
			res := Compose(fixedNow)
			rows, _ := res.List(context.Background(), f)
			if err := actionByKey(t, res, tt.key).Run(context.Background(), f, rows[2]); err != nil {
				t.Fatal(err)
			}
			want := []dockertest.Call{{Op: tt.op, ID: "c1"}, {Op: tt.op, ID: "c2"}}
			if got := f.Calls(); !slices.Equal(got, want) {
				t.Fatalf("calls: %v", got)
			}
		})
	}
}

func TestComposeOpenListsProjectContainers(t *testing.T) {
	f := composeFake()
	res := Compose(fixedNow)
	rows, _ := res.List(context.Background(), f)
	child, ok := res.Open(rows[2])
	if !ok {
		t.Fatal("no drill-down")
	}
	if got := namesOf(t, child, f); !slices.Equal(got, []string{"shop-web-1", "shop-db-1"}) {
		t.Fatalf("children: %v", got)
	}
}

func TestDefaultRegistryCommands(t *testing.T) {
	reg, err := Default(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	for cmd, want := range map[string]string{"i": "images", "v": "volumes", "n": "networks", "compose": "compose", "vol": "volumes", "net": "networks"} {
		if res, err := reg.Lookup(cmd); err != nil || res.Name != want {
			t.Errorf("Lookup(%q) = %q, %v; want %q", cmd, res.Name, err, want)
		}
	}
}
