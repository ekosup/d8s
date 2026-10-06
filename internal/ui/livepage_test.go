package ui

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
)

func TestLiveTextPageRefreshesUntilClosed(t *testing.T) {
	var fetches atomic.Int64
	res := stubResource("gauges")
	res.Pages = []resource.TextPage{{
		Key: "m", Name: "Stats", Refresh: 5 * time.Millisecond,
		Fetch: func(context.Context, docker.Client, resource.Row) ([]string, error) {
			return []string{fmt.Sprintf("sample %d", fetches.Add(1))}, nil
		},
	}}
	h := newHarness(t, fastWatch)
	h.register(res)
	h.show("gauges")
	h.step()

	h.app.key(tcell.KeyRune, 'm')
	h.until("first sample", func() bool { return strings.Contains(h.screen(), "sample 1") })
	if !strings.Contains(h.screen(), "Stats: alpha") {
		t.Fatalf("title wrong:\n%s", h.screen())
	}
	h.until("later sample", func() bool { return strings.Contains(h.screen(), "sample 4") })

	h.app.key(tcell.KeyEscape, 0)
	time.Sleep(30 * time.Millisecond) // let an in-flight fetch finish
	seen := fetches.Load()
	time.Sleep(60 * time.Millisecond)
	if got := fetches.Load(); got != seen {
		t.Fatalf("page kept fetching after it was closed: %d -> %d", seen, got)
	}
}
