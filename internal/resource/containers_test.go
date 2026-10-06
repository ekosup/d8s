package resource

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

var now = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func fixedNow() time.Time { return now }

func TestContainerRows(t *testing.T) {
	fake := dockertest.NewFake(dockertest.WithContainers(
		docker.Container{
			ID: "aaa", Name: "web", Image: "nginx:alpine", State: "running", Status: "Up 2 hours",
			Created: now.Add(-2 * time.Hour),
			Ports: []docker.Port{
				{IP: "0.0.0.0", Public: 8080, Private: 80, Proto: "tcp"},
				{IP: "::", Public: 8080, Private: 80, Proto: "tcp"},
				{Private: 443, Proto: "tcp"},
			},
		},
		docker.Container{ID: "bbb", Name: "job", Image: "busybox", State: "exited", Status: "Exited (0) 3 days ago", Created: now.Add(-72 * time.Hour)},
		docker.Container{ID: "ccc", Name: "crash", Image: "app:1", State: "exited", Status: "Exited (137) 5 minutes ago", Created: now.Add(-5 * time.Minute)},
		docker.Container{ID: "ddd", Name: "loop", Image: "app:2", State: "restarting", Status: "Restarting (1) 2 seconds ago", Created: now.Add(-40 * time.Second)},
		docker.Container{ID: "eee", Name: "local", Image: "db", State: "running", Status: "Up 1 minute", Created: now.Add(-time.Minute),
			Ports: []docker.Port{{IP: "127.0.0.1", Public: 5432, Private: 5432, Proto: "tcp"}}},
	))
	res := Containers(fixedNow)
	rows, err := res.List(context.Background(), fake)
	if err != nil {
		t.Fatal(err)
	}

	want := []struct {
		id    string
		cells []string
		tone  Tone
	}{
		{"aaa", []string{"web", "nginx:alpine", "running", "Up 2 hours", "443/tcp, 8080->80/tcp", "2h"}, ToneNormal},
		{"bbb", []string{"job", "busybox", "exited", "Exited (0) 3 days ago", "", "3d"}, ToneMuted},
		{"ccc", []string{"crash", "app:1", "exited", "Exited (137) 5 minutes ago", "", "5m"}, ToneBad},
		{"ddd", []string{"loop", "app:2", "restarting", "Restarting (1) 2 seconds ago", "", "40s"}, ToneWarn},
		{"eee", []string{"local", "db", "running", "Up 1 minute", "127.0.0.1:5432->5432/tcp", "1m"}, ToneNormal},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows", len(rows))
	}
	for i, w := range want {
		r := rows[i]
		if r.ID != w.id || !slices.Equal(r.Cells, w.cells) || r.Tone != w.tone {
			t.Errorf("row %d:\n got  %q %q tone %d\n want %q %q tone %d", i, r.ID, r.Cells, r.Tone, w.id, w.cells, w.tone)
		}
		if len(r.Cells) != len(res.Columns) {
			t.Errorf("row %d has %d cells for %d columns", i, len(r.Cells), len(res.Columns))
		}
	}

	// AGE sorts by real age, not by its text: 40s < 1m < 5m < 2h < 3d.
	age := slices.IndexFunc(res.Columns, func(c Column) bool { return c.Name == "AGE" })
	slices.SortFunc(rows, func(a, b Row) int {
		switch {
		case a.SortKeys[age] < b.SortKeys[age]:
			return -1
		case a.SortKeys[age] > b.SortKeys[age]:
			return 1
		}
		return 0
	})
	got := make([]string, len(rows))
	for i, r := range rows {
		got[i] = r.Cells[age]
	}
	if !slices.Equal(got, []string{"40s", "1m", "5m", "2h", "3d"}) {
		t.Fatalf("age order: %v", got)
	}
}

func TestHumanAge(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{-time.Second, "0s"},
		{0, "0s"},
		{59 * time.Second, "59s"},
		{time.Minute, "1m"},
		{59*time.Minute + 59*time.Second, "59m"},
		{time.Hour, "1h"},
		{47 * time.Hour, "47h"},
		{48 * time.Hour, "2d"},
		{400 * 24 * time.Hour, "400d"},
	}
	for _, tt := range tests {
		if got := humanAge(tt.d); got != tt.want {
			t.Errorf("humanAge(%s) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestDefaultRegistryHasContainers(t *testing.T) {
	reg, err := Default(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"c", "containers", "container", "cont", "ps"} {
		if res, err := reg.Lookup(cmd); err != nil || res.Name != "containers" {
			t.Errorf("Lookup(%q) = %q, %v", cmd, res.Name, err)
		}
	}
}
