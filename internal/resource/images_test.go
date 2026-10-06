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

func imageFake() *dockertest.Fake {
	f := dockertest.NewFake(dockertest.WithContainers(
		docker.Container{ID: "c1", Name: "web", ImageID: "sha256:aaaaaaaaaaaaaaaa", State: "running", Created: now},
		docker.Container{ID: "c2", Name: "web2", ImageID: "sha256:aaaaaaaaaaaaaaaa", State: "exited", Status: "Exited (0)", Created: now},
		docker.Container{ID: "c3", Name: "db", ImageID: "sha256:cccccccccccccccc", State: "running", Created: now},
	))
	f.SetImages(
		docker.Image{ID: "sha256:aaaaaaaaaaaaaaaa", Tags: []string{"nginx:alpine", "nginx:1.27"}, Size: 45 << 20, Created: now.Add(-48 * time.Hour)},
		docker.Image{ID: "sha256:bbbbbbbbbbbbbbbb", Size: 1536, Created: now.Add(-time.Hour)},
		docker.Image{ID: "sha256:cccccccccccccccc", Tags: []string{"registry.local:5000/team/db:15"}, Size: 3 << 30, Created: now.Add(-time.Minute)},
	)
	return f
}

func TestImageRows(t *testing.T) {
	res := Images(fixedNow)
	rows, err := res.List(context.Background(), imageFake())
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(rows))
	for i, r := range rows {
		got[i] = strings.Join(r.Cells, " | ")
	}
	want := []string{
		"nginx | alpine | aaaaaaaaaaaa | 45.0MiB | 2 | 2d",
		"nginx | 1.27 | aaaaaaaaaaaa | 45.0MiB | 2 | 2d",
		"<none> | <none> | bbbbbbbbbbbb | 1.5KiB | 0 | 1h",
		"registry.local:5000/team/db | 15 | cccccccccccc | 3.0GiB | 1 | 1m",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	ids := map[string]bool{}
	for _, r := range rows {
		if ids[r.ID] {
			t.Fatalf("row ID %q is not unique", r.ID)
		}
		ids[r.ID] = true
	}
	// Unused images are dimmed, so the ones safe to delete stand out.
	if rows[2].Tone != ToneMuted || rows[0].Tone != ToneNormal {
		t.Fatalf("tones: %v %v", rows[0].Tone, rows[2].Tone)
	}
	// SIZE sorts by bytes: 1.5KiB < 45MiB < 3GiB.
	size := slices.IndexFunc(res.Columns, func(c Column) bool { return c.Name == "SIZE" })
	if a, b, c := rows[2].SortKeys[size], rows[0].SortKeys[size], rows[3].SortKeys[size]; a >= b || b >= c {
		t.Fatal("size sort keys are not ordered by bytes")
	}
}

func TestImageDelete(t *testing.T) {
	f := imageFake()
	res := Images(fixedNow)
	rows, _ := res.List(context.Background(), f)
	del := actionByKey(t, res, "ctrl-d")
	if !del.Confirm || !del.Mutates {
		t.Fatal("delete must confirm and mutate")
	}
	// A tagged image is removed by reference (untag); a dangling one by ID.
	if err := del.Run(context.Background(), f, rows[1]); err != nil {
		t.Fatal(err)
	}
	if err := del.Run(context.Background(), f, rows[2]); err != nil {
		t.Fatal(err)
	}
	if got := f.Log(); !slices.Equal(got, []string{"remove image nginx:1.27", "remove image sha256:bbbbbbbbbbbbbbbb"}) {
		t.Fatalf("log: %v", got)
	}
	if w := del.Warn(rows[0]); !strings.Contains(w, "2 containers") {
		t.Fatalf("no in-use warning: %q", w)
	}
	if w := del.Warn(rows[2]); w != "" {
		t.Fatalf("warning for an unused image: %q", w)
	}
}

func TestImagePruneIsGlobal(t *testing.T) {
	f := imageFake()
	f.SetPruneReport(docker.PruneReport{Count: 3, Reclaimed: 5 << 20})
	prune := actionByKey(t, Images(fixedNow), "ctrl-p")
	if prune.Target == "" || !prune.Confirm || !prune.Mutates {
		t.Fatalf("prune: %+v", prune)
	}
	if err := prune.Run(context.Background(), f, Row{}); err != nil {
		t.Fatal(err)
	}
	if got := f.Log(); !slices.Equal(got, []string{"prune image"}) {
		t.Fatalf("log: %v", got)
	}
}

func TestImageOpenListsItsContainers(t *testing.T) {
	f := imageFake()
	res := Images(fixedNow)
	rows, _ := res.List(context.Background(), f)
	child, ok := res.Open(rows[0])
	if !ok {
		t.Fatal("no drill-down")
	}
	crows, err := child.List(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, r := range crows {
		names = append(names, r.Name())
	}
	if !slices.Equal(names, []string{"web", "web2"}) || !strings.Contains(child.Title, "nginx:alpine") {
		t.Fatalf("children %v, title %q", names, child.Title)
	}
	if len(child.Actions) == 0 || child.Logs == nil {
		t.Fatal("the child view lost the container capabilities")
	}
}

func TestImageHistoryPage(t *testing.T) {
	f := imageFake()
	f.SetImageHistory("sha256:aaaaaaaaaaaaaaaa",
		docker.ImageLayer{ID: "sha256:aaaaaaaaaaaaaaaa", CreatedBy: "CMD [\"nginx\"]", Size: 0, Created: now.Add(-48 * time.Hour)},
		docker.ImageLayer{ID: "<missing>", CreatedBy: "RUN /bin/sh -c apk add nginx", Size: 12 << 20, Created: now.Add(-49 * time.Hour)},
	)
	res := Images(fixedNow)
	rows, _ := res.List(context.Background(), f)
	if len(res.Pages) != 1 || res.Pages[0].Key != "h" {
		t.Fatalf("pages: %+v", res.Pages)
	}
	lines, err := res.Pages[0].Fetch(context.Background(), f, rows[0])
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lines, "\n")
	for _, want := range []string{"CREATED BY", `CMD ["nginx"]`, "12.0MiB", "apk add nginx", "2d"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	tests := map[int64]string{0: "0B", 1023: "1023B", 1024: "1.0KiB", 1536: "1.5KiB", 45 << 20: "45.0MiB", 3 << 30: "3.0GiB", 5 << 40: "5.0TiB", -1: "0B"}
	for in, want := range tests {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
