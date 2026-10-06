package resource

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

func dfFake() *dockertest.Fake {
	f := dockertest.NewFake()
	f.SetDiskUsage(
		docker.DiskUsage{Kind: docker.KindImage, Total: 74, Active: 9, Size: 20 << 30, Reclaimable: 15 << 30},
		docker.DiskUsage{Kind: docker.KindContainer, Total: 11, Active: 4, Size: 300 << 20, Reclaimable: 100 << 20},
		docker.DiskUsage{Kind: docker.KindVolume, Total: 6, Active: 3, Size: 2 << 30, Reclaimable: 0},
		docker.DiskUsage{Kind: docker.KindBuildCache, Total: 120, Active: 0, Size: 4 << 30, Reclaimable: 4 << 30},
	)
	return f
}

func TestDiskUsageRows(t *testing.T) {
	res := DiskUsage()
	rows, err := res.List(context.Background(), dfFake())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Images | 74 | 9 | 20.0GiB | 15.0GiB (75%)",
		"Containers | 11 | 4 | 300.0MiB | 100.0MiB (33%)",
		"Volumes | 6 | 3 | 2.0GiB | 0B (0%)",
		"Build cache | 120 | 0 | 4.0GiB | 4.0GiB (100%)",
	}
	if got := cellsOf(rows); !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
}

func TestDiskUsagePrune(t *testing.T) {
	f := dfFake()
	res := DiskUsage()
	rows, _ := res.List(context.Background(), f)
	prune := actionByKey(t, res, "ctrl-p")
	if !prune.Confirm || !prune.Mutates || prune.Target != "" {
		t.Fatalf("prune: %+v", prune)
	}

	// The confirmation says what will go and how much space that frees.
	wantNames := []string{"dangling images", "stopped containers", "unused anonymous volumes", "build cache"}
	wantSizes := []string{"15.0GiB", "100.0MiB", "0B", "4.0GiB"}
	for i, r := range rows {
		if r.Name() != wantNames[i] {
			t.Errorf("row %d name %q, want %q", i, r.Name(), wantNames[i])
		}
		if w := prune.Warn(r); !strings.Contains(w, wantSizes[i]) {
			t.Errorf("row %d warning %q does not mention %s", i, w, wantSizes[i])
		}
	}
	if w := prune.Warn(rows[0]); !strings.Contains(w, "tagged images stay") {
		t.Fatalf("image warning does not explain the dangling-only limit: %q", w)
	}

	for _, r := range rows {
		if err := prune.Run(context.Background(), f, r); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.Log(); !slices.Equal(got, []string{"prune image", "prune container", "prune volume", "prune build cache"}) {
		t.Fatalf("log: %v", got)
	}
}
