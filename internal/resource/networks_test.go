package resource

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

func networkFake() *dockertest.Fake {
	f := dockertest.NewFake(dockertest.WithContainers(
		docker.Container{ID: "c1", Name: "web", State: "running", Created: now, Networks: []docker.Attachment{
			{Network: "app", NetworkID: "n1", IP: "172.20.0.2"}, {Network: "bridge", NetworkID: "n0", IP: "172.17.0.2"}}},
		docker.Container{ID: "c2", Name: "db", State: "running", Created: now, Networks: []docker.Attachment{{Network: "app", NetworkID: "n1", IP: "172.20.0.3"}}},
	))
	f.SetNetworks(
		docker.Network{ID: "n0", Name: "bridge", Driver: "bridge", Scope: "local", Subnets: []string{"172.17.0.0/16"}},
		docker.Network{ID: "n1", Name: "app", Driver: "bridge", Scope: "local", Subnets: []string{"172.20.0.0/16", "fd00::/64"}},
		docker.Network{ID: "n2", Name: "unused", Driver: "overlay", Scope: "swarm"},
	)
	return f
}

func TestNetworkRows(t *testing.T) {
	res := Networks(fixedNow)
	rows, err := res.List(context.Background(), networkFake())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"bridge | bridge | local | 172.17.0.0/16 | 1",
		"app | bridge | local | 172.20.0.0/16, fd00::/64 | 2",
		"unused | overlay | swarm |  | 0",
	}
	if got := cellsOf(rows); !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
}

func TestNetworkOpenShowsContainersWithIP(t *testing.T) {
	f := networkFake()
	res := Networks(fixedNow)
	rows, _ := res.List(context.Background(), f)
	child, ok := res.Open(rows[1])
	if !ok {
		t.Fatal("no drill-down")
	}
	if child.Columns[1].Name != "IP" {
		t.Fatalf("columns: %+v", child.Columns)
	}
	crows, err := child.List(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, r := range crows {
		if len(r.Cells) != len(child.Columns) || len(r.SortKeys) != len(child.Columns) {
			t.Fatalf("row shape does not match the columns: %d cells, %d keys, %d columns", len(r.Cells), len(r.SortKeys), len(child.Columns))
		}
		got = append(got, r.Cells[0]+" "+r.Cells[1])
	}
	if !slices.Equal(got, []string{"web 172.20.0.2", "db 172.20.0.3"}) {
		t.Fatalf("children: %v", got)
	}
}

func TestNetworkDelete(t *testing.T) {
	f := networkFake()
	res := Networks(fixedNow)
	rows, _ := res.List(context.Background(), f)
	del := actionByKey(t, res, "ctrl-d")
	if w := del.Warn(rows[1]); !strings.Contains(w, "2 containers") {
		t.Fatalf("no warning: %q", w)
	}
	if err := del.Run(context.Background(), f, rows[2]); err != nil {
		t.Fatal(err)
	}
	if got := f.Log(); !slices.Equal(got, []string{"remove network n2"}) {
		t.Fatalf("log: %v", got)
	}
}
