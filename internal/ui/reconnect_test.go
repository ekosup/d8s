package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

func TestLostConnectionIsMarkedAndRecovers(t *testing.T) {
	opts := fastWatch
	opts.Poll = 10 * time.Millisecond
	h := newHarness(t, opts, docker.Container{ID: "1", Name: "web", State: "running", Created: created})
	h.show("containers")
	h.step()
	header := func() string { return strings.Join(headerLines(t, h.app, 120), "\n") }
	if strings.Contains(header(), "DISCONNECTED") || strings.Contains(h.screen(), "stale") {
		t.Fatalf("marked as disconnected while healthy:\n%s", h.screen())
	}

	h.fake.SetListError(errors.New("cannot connect to the Docker daemon"))
	h.until("disconnected marker", func() bool { return strings.Contains(header(), "DISCONNECTED") })
	s := h.screen()
	// The last good rows stay, clearly labelled as old.
	if !strings.Contains(s, "web") || !strings.Contains(s, "Containers[1] (stale)") {
		t.Fatalf("stale data not kept or not labelled:\n%s", s)
	}

	// The daemon comes back with different content; nobody presses a key.
	h.fake.SetContainers(
		docker.Container{ID: "1", Name: "web", State: "running", Created: created},
		docker.Container{ID: "2", Name: "api", State: "running", Created: created},
	)
	h.fake.SetListError(nil)
	h.until("recovered", func() bool { return strings.Contains(h.screen(), "Containers[2]") })
	if strings.Contains(header(), "DISCONNECTED") || strings.Contains(h.screen(), "stale") {
		t.Fatalf("markers not cleared after recovery:\n%s", h.screen())
	}
}

func TestDisconnectedMarkerClearsWhenSwitchingViews(t *testing.T) {
	opts := fastWatch
	opts.Poll = 10 * time.Millisecond
	h := newHarness(t, opts, docker.Container{ID: "1", Name: "web", State: "running", Created: created})
	h.register(stubResource("things"))
	h.show("containers")
	h.step()
	h.fake.SetListError(errors.New("cannot connect to the Docker daemon"))
	h.until("disconnected", func() bool { return strings.Contains(strings.Join(headerLines(t, h.app, 120), "\n"), "DISCONNECTED") })

	// A view that does not depend on the failing call works; the marker
	// must not linger from the previous view.
	h.show("things")
	h.until("other view healthy", func() bool {
		return strings.Contains(h.screen(), "Things[1]") && !strings.Contains(strings.Join(headerLines(t, h.app, 120), "\n"), "DISCONNECTED")
	})
}
