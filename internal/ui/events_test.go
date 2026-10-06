package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
)

func TestEventsViewIsLiveAndNewestFirst(t *testing.T) {
	h := newHarness(t, fastWatch)
	h.register(resource.Events(h.app.Events, time.UTC))
	h.show("events")
	h.step()
	if !strings.Contains(h.screen(), "Events[0]") {
		t.Fatalf("expected no events yet:\n%s", h.screen())
	}

	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	h.fake.Emit(docker.Event{Type: "container", Action: "start", ID: "aaaaaaaaaaaaaaaa", Name: "web", Time: at})
	h.until("first event", func() bool { return strings.Contains(h.screen(), "start") })
	h.fake.Emit(docker.Event{Type: "container", Action: "die", ID: "aaaaaaaaaaaaaaaa", Name: "web", Time: at.Add(time.Second)})
	h.until("second event", func() bool { return strings.Contains(h.screen(), "die") })

	lines := strings.Split(h.screen(), "\n")
	die, start := lineContaining(lines, "die"), lineContaining(lines, "start")
	if die < 0 || start < 0 || die > start {
		t.Fatalf("newest event is not on top:\n%s", h.screen())
	}
	if !strings.Contains(h.screen(), "TIME↓") || !strings.Contains(h.screen(), "12:00:01") {
		t.Fatalf("time column wrong:\n%s", h.screen())
	}
}
