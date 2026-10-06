package ui

import (
	"strings"
	"testing"

	"github.com/ekosup/d8s/internal/resource"
)

func wideRows() []resource.Row {
	return []resource.Row{
		{ID: "1", Cells: []string{"chatbotai-minio-1", "registry.example.internal/team/vendors/minio:latest", "running", "Up 44 hours", "9000->9000/tcp, 9001->9001/tcp", "43h"}},
		{ID: "2", Cells: []string{"web", "nginx:alpine", "exited", "Exited (0) 2 months ago", "", "67d"}},
	}
}

var wideCols = []resource.Column{{Name: "NAME"}, {Name: "IMAGE"}, {Name: "STATE"}, {Name: "STATUS"}, {Name: "PORTS"}, {Name: "AGE"}}

func TestTableShrinksWideColumnsToFit(t *testing.T) {
	v := newTableView("Containers", wideCols)
	v.SetRows(wideRows())

	for _, width := range []int{80, 100, 120} {
		lines := render(t, v, width, 8)
		joined := strings.Join(lines, "\n")
		// Every column header and the last column's values stay on screen.
		for _, want := range []string{"NAME", "IMAGE", "STATE", "STATUS", "PORTS", "AGE", "43h", "67d", "chatbotai-minio-1"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("width %d: %q pushed off screen:\n%s", width, want, joined)
			}
		}
		if !strings.Contains(joined, "…") {
			t.Fatalf("width %d: nothing was shortened:\n%s", width, joined)
		}
	}
}

func TestTableShowsFullTextWhenThereIsRoom(t *testing.T) {
	v := newTableView("Containers", wideCols)
	v.SetRows(wideRows())
	joined := strings.Join(render(t, v, 200, 8), "\n")
	if !strings.Contains(joined, "registry.example.internal/team/vendors/minio:latest") || strings.Contains(joined, "…") {
		t.Fatalf("text shortened although it fits:\n%s", joined)
	}
	// Narrow, then wide again: the shortening is undone.
	render(t, v, 80, 8)
	if joined = strings.Join(render(t, v, 200, 8), "\n"); strings.Contains(joined, "…") {
		t.Fatalf("still shortened after widening:\n%s", joined)
	}
}

func TestTableFitsAfterRowsChange(t *testing.T) {
	v := newTableView("Containers", wideCols)
	v.SetRows(wideRows()[1:])
	render(t, v, 80, 8)
	v.SetRows(wideRows()) // a wide row arrives at the same terminal width
	if joined := strings.Join(render(t, v, 80, 8), "\n"); !strings.Contains(joined, "43h") {
		t.Fatalf("AGE lost after refresh:\n%s", joined)
	}
}
