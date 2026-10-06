package resource

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

func TestProjectSelectsAndOrdersColumns(t *testing.T) {
	f := dockertest.NewFake(dockertest.WithContainers(
		docker.Container{ID: "1", Name: "web", Image: "nginx", State: "running", Status: "Up", Created: now.Add(-2 * 3600e9)},
	))
	res, err := Project(Containers(fixedNow), []string{"age", "NAME", "State"})
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(res.Columns))
	for i, c := range res.Columns {
		names[i] = c.Name
	}
	if !slices.Equal(names, []string{"AGE", "NAME", "STATE"}) {
		t.Fatalf("columns: %v", names)
	}
	rows, err := res.List(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rows[0].Cells, " | "); got != "2h | web | running" {
		t.Fatalf("cells: %q", got)
	}
	// Sort keys travel with their column: AGE still sorts by time.
	if len(rows[0].SortKeys) != 3 || rows[0].SortKeys[0] == "" || rows[0].SortKeys[1] != "" {
		t.Fatalf("sort keys: %q", rows[0].SortKeys)
	}
	// The row is still the same object for actions and messages.
	if rows[0].ID != "1" || rows[0].Name() != "web" {
		t.Fatalf("identity lost: id %q name %q", rows[0].ID, rows[0].Name())
	}
	if len(res.Actions) == 0 || res.Logs == nil {
		t.Fatal("capabilities lost")
	}
}

func TestProjectKeepsTheNameWhenItsColumnIsHidden(t *testing.T) {
	f := dockertest.NewFake(dockertest.WithContainers(docker.Container{ID: "1", Name: "web", State: "running", Created: now}))
	res, err := Project(Containers(fixedNow), []string{"STATE", "AGE"})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := res.List(context.Background(), f)
	if rows[0].Name() != "web" {
		t.Fatalf("a confirmation would now call this row %q", rows[0].Name())
	}
}

func TestProjectRejectsBadColumnLists(t *testing.T) {
	for name, cols := range map[string][]string{
		"unknown column": {"NAME", "COLOUR"},
		"duplicate":      {"NAME", "name"},
		"empty name":     {"NAME", ""},
	} {
		if _, err := Project(Containers(fixedNow), cols); err == nil {
			t.Errorf("%s: accepted %v", name, cols)
		} else if name == "unknown column" && (!strings.Contains(err.Error(), "COLOUR") || !strings.Contains(err.Error(), "NAME, IMAGE")) {
			t.Errorf("the error should name the column and list the valid ones: %v", err)
		}
	}
	// No list means no change.
	res, err := Project(Containers(fixedNow), nil)
	if err != nil || len(res.Columns) != len(Containers(fixedNow).Columns) {
		t.Fatalf("empty projection changed the resource: %v", err)
	}
}
