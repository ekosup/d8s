package resource

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

func statsFake() *dockertest.Fake {
	f := dockertest.NewFake(dockertest.WithContainers(
		docker.Container{ID: "c1", Name: "busy", State: "running", Status: "Up", Created: now},
		docker.Container{ID: "c2", Name: "idle", State: "running", Status: "Up", Created: now},
		docker.Container{ID: "c3", Name: "stopped", State: "exited", Status: "Exited (0)", Created: now},
	))
	f.SetStats("c1", docker.Stats{CPUTotal: 1_000, CPUSystem: 100_000, OnlineCPUs: 4, MemUsage: 200 << 20, MemLimit: 1 << 30})
	f.SetStats("c2", docker.Stats{CPUTotal: 500, CPUSystem: 100_000, OnlineCPUs: 4, MemUsage: 10 << 20, MemLimit: 1 << 30})
	return f
}

func cell(t *testing.T, res Resource, rows []Row, name, col string) string {
	t.Helper()
	ci := slices.IndexFunc(res.Columns, func(c Column) bool { return c.Name == col })
	for _, r := range rows {
		if r.Name() == name {
			return r.Cells[ci]
		}
	}
	t.Fatalf("no row %q", name)
	return ""
}

func TestStatsColumns(t *testing.T) {
	f := statsFake()
	res := WithStats(Containers(fixedNow))
	names := make([]string, len(res.Columns))
	for i, c := range res.Columns {
		names[i] = c.Name
	}
	if !slices.Equal(names, []string{"NAME", "IMAGE", "STATE", "STATUS", "CPU%", "MEM", "PORTS", "AGE"}) {
		t.Fatalf("columns: %v", names)
	}

	// First refresh: memory is known, CPU needs a second sample.
	rows, err := res.List(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if got := cell(t, res, rows, "busy", "MEM"); got != "200.0MiB" {
		t.Fatalf("MEM = %q", got)
	}
	if got := cell(t, res, rows, "busy", "CPU%"); got != "-" {
		t.Fatalf("CPU%% on the first sample = %q", got)
	}
	if got := cell(t, res, rows, "stopped", "MEM"); got != "-" {
		t.Fatalf("MEM of a stopped container = %q", got)
	}
	for _, r := range rows {
		if len(r.Cells) != len(res.Columns) || len(r.SortKeys) != len(res.Columns) {
			t.Fatalf("row shape: %d cells, %d keys, %d columns", len(r.Cells), len(r.SortKeys), len(res.Columns))
		}
	}

	// Second refresh: busy used 1000 of 2000 system ticks on 4 CPUs = 200%.
	f.SetStats("c1", docker.Stats{CPUTotal: 2_000, CPUSystem: 102_000, OnlineCPUs: 4, MemUsage: 300 << 20, MemLimit: 1 << 30})
	f.SetStats("c2", docker.Stats{CPUTotal: 510, CPUSystem: 102_000, OnlineCPUs: 4, MemUsage: 10 << 20, MemLimit: 1 << 30})
	rows, _ = res.List(context.Background(), f)
	if got := cell(t, res, rows, "busy", "CPU%"); got != "200.0" {
		t.Fatalf("CPU%% = %q", got)
	}
	if got := cell(t, res, rows, "idle", "CPU%"); got != "2.0" {
		t.Fatalf("CPU%% = %q", got)
	}

	// Sorting by CPU or MEM is numeric: "2.0" < "200.0", and "-" comes first.
	cpu := slices.IndexFunc(res.Columns, func(c Column) bool { return c.Name == "CPU%" })
	slices.SortFunc(rows, func(a, b Row) int { return strings.Compare(a.SortKeys[cpu], b.SortKeys[cpu]) })
	order := []string{}
	for _, r := range rows {
		order = append(order, r.Name())
	}
	if !slices.Equal(order, []string{"stopped", "idle", "busy"}) {
		t.Fatalf("CPU order: %v", order)
	}
}

func TestStatsKeepContainerCapabilities(t *testing.T) {
	res := WithStats(Containers(fixedNow))
	if len(res.Actions) == 0 || res.Logs == nil || res.Exec == nil || res.Inspect == nil {
		t.Fatal("decorating with stats dropped capabilities")
	}
	if !slices.ContainsFunc(res.Pages, func(p TextPage) bool { return p.Key == "m" && p.Refresh > 0 }) {
		t.Fatalf("no live stats page: %+v", res.Pages)
	}
}

func TestStatsSkippedForVeryManyContainers(t *testing.T) {
	cs := make([]docker.Container, maxStatsContainers+1)
	for i := range cs {
		cs[i] = docker.Container{ID: fmt.Sprintf("c%03d", i), Name: fmt.Sprintf("n%03d", i), State: "running", Created: now}
	}
	f := dockertest.NewFake(dockertest.WithContainers(cs...))
	res := WithStats(Containers(fixedNow))
	rows, err := res.List(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if f.StatsCalls() != 0 {
		t.Fatalf("%d stats calls for %d containers", f.StatsCalls(), len(cs))
	}
	if got := cell(t, res, rows, "n000", "MEM"); got != "-" {
		t.Fatalf("MEM = %q", got)
	}
}

func TestStatsFailureDoesNotBreakTheList(t *testing.T) {
	f := statsFake()
	f.SetStatsError(fmt.Errorf("stats unavailable"))
	res := WithStats(Containers(fixedNow))
	rows, err := res.List(context.Background(), f)
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows %d, err %v", len(rows), err)
	}
	if got := cell(t, res, rows, "busy", "MEM"); got != "-" {
		t.Fatalf("MEM = %q", got)
	}
}

func TestStatsPage(t *testing.T) {
	f := statsFake()
	f.SetStats("c1", docker.Stats{CPUTotal: 1_000, CPUSystem: 100_000, OnlineCPUs: 4, MemUsage: 256 << 20, MemLimit: 1 << 30,
		NetRx: 3 << 20, NetTx: 2048, BlkRead: 0, BlkWrite: 5 << 20, PIDs: 7})
	res := WithStats(Containers(fixedNow))
	rows, _ := res.List(context.Background(), f)
	page := res.Pages[slices.IndexFunc(res.Pages, func(p TextPage) bool { return p.Key == "m" })]

	f.SetStats("c1", docker.Stats{CPUTotal: 1_500, CPUSystem: 102_000, OnlineCPUs: 4, MemUsage: 256 << 20, MemLimit: 1 << 30,
		NetRx: 3 << 20, NetTx: 2048, BlkWrite: 5 << 20, PIDs: 7})
	lines, err := page.Fetch(context.Background(), f, rows[0])
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lines, "\n")
	for _, want := range []string{"100.0%", "4 CPUs", "256.0MiB / 1.0GiB", "25.0%", "3.0MiB", "2.0KiB", "5.0MiB", "7"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}
