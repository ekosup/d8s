package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
)

func logHarness(t *testing.T, lines ...string) *harness {
	t.Helper()
	h := newHarness(t, fastWatch, docker.Container{ID: "1", Name: "web", Image: "nginx", State: "running", Created: created})
	h.fake.SetLogs("1", lines...)
	h.show("containers")
	h.step()
	return h
}

func (h *harness) openLogs() *logView {
	h.t.Helper()
	h.app.key(tcell.KeyRune, 'l')
	h.until("log page", func() bool { return len(h.app.stack) == 2 })
	return h.app.logs
}

func TestLogsShowExistingLines(t *testing.T) {
	h := logHarness(t, "starting nginx", "listening on :80")
	h.openLogs()
	h.until("log lines", func() bool { return strings.Contains(h.screen(), "listening on :80") })
	s := h.screen()
	for _, want := range []string{"Logs: web", "starting nginx", "<containers> <logs>"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "2026-10-06T") {
		t.Fatalf("timestamps shown by default:\n%s", s)
	}
}

func TestLogsFollowNewLines(t *testing.T) {
	h := logHarness(t, "first")
	h.openLogs()
	h.until("first line", func() bool { return strings.Contains(h.screen(), "first") })
	for i := range 60 { // more than a screenful: following must scroll
		h.fake.AppendLog("1", fmt.Sprintf("request %02d", i))
	}
	h.until("last line on screen", func() bool { return strings.Contains(h.screen(), "request 59") })
}

func TestLogsToggles(t *testing.T) {
	h := logHarness(t, "hello")
	lv := h.openLogs()
	h.until("line", func() bool { return strings.Contains(h.screen(), "hello") })

	h.app.key(tcell.KeyRune, 't')
	if s := h.screen(); !strings.Contains(s, "2026-10-06T12:00:00") {
		t.Fatalf("t did not show timestamps:\n%s", s)
	}
	h.app.key(tcell.KeyRune, 't')
	if s := h.screen(); strings.Contains(s, "2026-10-06T") {
		t.Fatalf("t did not hide timestamps:\n%s", s)
	}

	if !lv.pager.follow {
		t.Fatal("follow should be on by default")
	}
	h.app.key(tcell.KeyRune, 's')
	if lv.pager.follow || !strings.Contains(h.screen(), "paused") {
		t.Fatalf("s did not pause following:\n%s", h.screen())
	}
}

func TestLogsRangeRestartsStream(t *testing.T) {
	h := logHarness(t, "old line")
	h.openLogs()
	h.until("line", func() bool { return strings.Contains(h.screen(), "old line") })
	if o := h.fake.LastLogOptions(); !o.Follow || o.Tail != defaultLogTail || o.Since != 0 || !o.Timestamps {
		t.Fatalf("initial options: %+v", o)
	}

	h.app.key(tcell.KeyRune, '2')
	h.until("restart with since", func() bool { return h.fake.LastLogOptions().Since == 5*time.Minute })
	if o := h.fake.LastLogOptions(); o.Tail != 0 {
		t.Fatalf("since stream should not be limited by tail: %+v", o)
	}
	h.until("title", func() bool { return strings.Contains(h.screen(), "last 5m") })
	h.until("one stream", func() bool { return h.fake.OpenLogStreams() == 1 })
}

func TestLogsStopWhenPageCloses(t *testing.T) {
	h := logHarness(t, "hello")
	h.openLogs()
	h.until("stream open", func() bool { return h.fake.OpenLogStreams() == 1 })
	h.app.key(tcell.KeyEscape, 0)
	h.until("stream closed", func() bool { return h.fake.OpenLogStreams() == 0 })
}

func TestLogsBufferIsCapped(t *testing.T) {
	h := logHarness(t)
	h.app.logBuffer = 100
	lv := h.openLogs()
	for i := range 250 {
		h.fake.AppendLog("1", fmt.Sprintf("line %03d", i))
	}
	h.until("last line", func() bool {
		return len(lv.pager.lines) > 0 && strings.HasSuffix(lv.pager.lines[len(lv.pager.lines)-1], "line 249")
	})
	if n := len(lv.pager.lines); n != 100 {
		t.Fatalf("kept %d lines, want 100", n)
	}
}

func TestLogsHighVolumeStaysResponsive(t *testing.T) {
	h := logHarness(t)
	lv := h.openLogs()
	start := time.Now()
	const total = 20000
	go func() {
		for i := range total {
			h.fake.AppendLog("1", fmt.Sprintf("GET /item/%05d 200 12ms", i))
		}
	}()
	updates := 0
	last := fmt.Sprintf("%05d 200 12ms", total-1)
	for {
		select {
		case f := <-h.queue:
			f()
			updates++
		case <-time.After(5 * time.Second):
			t.Fatalf("stalled after %d updates", updates)
		}
		if n := len(lv.pager.lines); n > 0 && strings.HasSuffix(lv.pager.lines[n-1], last) {
			break
		}
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("%d lines took %s", total, d)
	}
	// Lines are handed over in batches, not one UI update per line.
	if updates > total/10 {
		t.Fatalf("%d UI updates for %d lines", updates, total)
	}
	if n := len(lv.pager.lines); n != defaultLogBuffer {
		t.Fatalf("kept %d lines, want %d", n, defaultLogBuffer)
	}
}

func TestLogsSaveMatchesWhatIsShown(t *testing.T) {
	h := logHarness(t, "alpha", "beta")
	h.app.dumpDir = t.TempDir()
	h.openLogs()
	h.until("lines", func() bool { return strings.Contains(h.screen(), "beta") })
	h.app.key(tcell.KeyCtrlS, 0)
	files, _ := filepath.Glob(filepath.Join(h.app.dumpDir, "logs-web-*"))
	if len(files) != 1 {
		t.Fatalf("saved files: %v", files)
	}
	if data, _ := os.ReadFile(files[0]); string(data) != "alpha\nbeta\n" {
		t.Fatalf("saved %q", data)
	}
}

func TestLogsErrorIsShown(t *testing.T) {
	h := logHarness(t)
	h.fake.SetLogError(fmt.Errorf("no such container"))
	h.app.key(tcell.KeyRune, 'l')
	h.until("error", func() bool { return strings.Contains(h.screen(), "no such container") })
}
