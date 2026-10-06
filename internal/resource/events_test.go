package resource

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

func TestEventRows(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	events := []docker.Event{
		{Type: "container", Action: "start", ID: "aaaaaaaaaaaaaaaaaaaa", Name: "web", Time: at},
		{Type: "container", Action: "die", ID: "aaaaaaaaaaaaaaaaaaaa", Name: "web", Time: at.Add(2 * time.Second)},
		{Type: "network", Action: "connect", ID: "nnnnnnnnnnnnnnnn", Name: "app", Time: at.Add(2 * time.Second)},
	}
	res := Events(func() []docker.Event { return events }, time.UTC)
	rows, err := res.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"12:00:00 | container | start | web | aaaaaaaaaaaa",
		"12:00:02 | container | die | web | aaaaaaaaaaaa",
		"12:00:02 | network | connect | app | nnnnnnnnnnnn",
	}
	if got := cellsOf(rows); !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
	if rows[1].Tone != ToneWarn || rows[0].Tone != ToneNormal {
		t.Fatalf("tones: %v %v", rows[0].Tone, rows[1].Tone)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.ID] {
			t.Fatalf("duplicate row ID %q", r.ID)
		}
		seen[r.ID] = true
	}
	// Newest first by default, and time sorts chronologically.
	if res.SortColumn != 0 || !res.SortDesc {
		t.Fatalf("default sort: column %d desc %v", res.SortColumn, res.SortDesc)
	}
	if rows[0].SortKeys[0] >= rows[1].SortKeys[0] || rows[1].SortKeys[0] >= rows[2].SortKeys[0] {
		t.Fatal("time sort keys are not chronological and unique")
	}
	if !slices.Equal(res.EventTypes, []string{"*"}) {
		t.Fatalf("event types: %v", res.EventTypes)
	}
}
