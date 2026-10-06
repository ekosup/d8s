package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
)

const (
	defaultLogBuffer = 5000 // lines kept in memory per log page
	defaultLogTail   = 1000 // lines fetched when a log page opens
	logFlushInterval = 50 * time.Millisecond
	maxLogLineBytes  = 1 << 20
)

// logRanges are the choices on the number keys: how far back to read.
var logRanges = []struct {
	key   rune
	since time.Duration // 0 = the last defaultLogTail lines
	label string
}{
	{'0', 0, "tail"},
	{'1', time.Minute, "1m"},
	{'2', 5 * time.Minute, "5m"},
	{'3', 15 * time.Minute, "15m"},
	{'4', 30 * time.Minute, "30m"},
	{'5', time.Hour, "1h"},
}

type logLine struct {
	stamp string // as received; empty when the line had none
	text  string
}

// logView is a pager fed by a live log stream.
type logView struct {
	app    *App
	pager  *pager
	name   string
	open   func(ctx context.Context, opts docker.LogOptions) (io.ReadCloser, error)
	lines  []logLine
	stamps bool
	since  time.Duration
	ended  bool

	gen    int // identifies the current stream; older ones are ignored
	cancel context.CancelFunc
}

// openLogs shows the logs of the selected row and follows them.
func (a *App) openLogs(res resource.Resource, view *tableView) {
	row, ok := view.SelectedRow()
	if !ok || a.client == nil {
		return
	}
	lv := &logView{
		app:   a,
		pager: newPager("", a.logBuffer),
		name:  row.Name(),
		open: func(ctx context.Context, opts docker.LogOptions) (io.ReadCloser, error) {
			return res.Logs(ctx, a.client, row, opts)
		},
	}
	lv.pager.follow = true
	lv.pager.saveName = "logs-" + row.Name()
	a.logs = lv

	keys := []binding{
		runeBinding('s', "s", "Autoscroll", lv.toggleFollow),
		runeBinding('t', "t", "Timestamps", lv.toggleStamps),
	}
	// Six range keys would crowd the header; one summary entry stands in
	// for them there, and help lists each.
	keys = append(keys, binding{label: "0-5", desc: "Range"})
	for _, r := range logRanges {
		desc := "Last " + r.label
		if r.since == 0 {
			desc = fmt.Sprintf("Last %d lines", a.logTail)
		}
		b := runeBinding(r.key, string(r.key), desc, func() { lv.start(r.since) })
		b.noHint = true
		keys = append(keys, b)
	}
	a.pushPager("logs", lv.pager, lv.stop, keys...)
	lv.start(0)
}

// start (re)opens the stream, reading back as far as since.
func (lv *logView) start(since time.Duration) {
	lv.stop()
	lv.gen++
	gen := lv.gen
	lv.since, lv.ended, lv.lines = since, false, nil
	lv.pager.SetLines(nil)
	lv.drawTitle()

	ctx, cancel := context.WithCancel(context.Background())
	lv.cancel = cancel
	opts := docker.LogOptions{Follow: true, Timestamps: true, Since: since}
	if since == 0 {
		opts.Tail = lv.app.logTail
	}
	go lv.pump(ctx, gen, opts)
}

func (lv *logView) stop() {
	if lv.cancel != nil {
		lv.cancel()
		lv.cancel = nil
	}
}

// pump reads the stream and hands lines to the UI in batches: one screen
// update per interval however fast the container writes.
func (lv *logView) pump(ctx context.Context, gen int, opts docker.LogOptions) {
	rc, err := lv.open(ctx, opts)
	if err != nil {
		if ctx.Err() == nil {
			lv.app.queue(func() { lv.app.Flash(flashError, "logs "+lv.name+": "+oneLine(err.Error())) })
		}
		return
	}
	defer func() { _ = rc.Close() }()
	go func() { // unblock the scanner when the page closes
		<-ctx.Done()
		_ = rc.Close()
	}()

	lines := make(chan logLine, 4096)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(rc)
		sc.Buffer(make([]byte, 64*1024), maxLogLineBytes)
		for sc.Scan() {
			select {
			case lines <- parseLogLine(sc.Text()):
			case <-ctx.Done():
				return
			}
		}
	}()

	tick := time.NewTicker(logFlushInterval)
	defer tick.Stop()
	var batch []logLine
	flush := func() {
		if len(batch) == 0 {
			return
		}
		out := batch
		batch = nil
		lv.app.queue(func() { lv.append(gen, out) })
	}
	for {
		select {
		case <-ctx.Done():
			return
		case l, ok := <-lines:
			if !ok {
				flush()
				lv.app.queue(func() { lv.markEnded(gen) })
				return
			}
			batch = append(batch, l)
		case <-tick.C:
			flush()
		}
	}
}

func (lv *logView) append(gen int, batch []logLine) {
	if gen != lv.gen {
		return
	}
	lv.lines = append(lv.lines, batch...)
	if over := len(lv.lines) - lv.pager.max; lv.pager.max > 0 && over > 0 {
		lv.lines = lv.lines[over:]
	}
	shown := make([]string, len(batch))
	for i, l := range batch {
		shown[i] = lv.format(l)
	}
	lv.pager.Append(shown)
}

func (lv *logView) markEnded(gen int) {
	if gen == lv.gen {
		lv.ended = true
		lv.drawTitle()
	}
}

func (lv *logView) format(l logLine) string {
	if lv.stamps && l.stamp != "" {
		return l.stamp + " " + l.text
	}
	return l.text
}

func (lv *logView) toggleStamps() {
	lv.stamps = !lv.stamps
	shown := make([]string, len(lv.lines))
	for i, l := range lv.lines {
		shown[i] = lv.format(l)
	}
	lv.pager.SetLines(shown)
}

func (lv *logView) toggleFollow() {
	lv.pager.follow = !lv.pager.follow
	if lv.pager.follow {
		lv.pager.ScrollToEnd()
	}
	lv.drawTitle()
}

func (lv *logView) drawTitle() {
	scope := fmt.Sprintf("last %d lines", lv.app.logTail)
	if lv.since > 0 {
		scope = "last " + shortDuration(lv.since)
	}
	state := "following"
	switch {
	case lv.ended:
		state = "stream ended"
	case !lv.pager.follow:
		state = "paused"
	}
	lv.pager.title = fmt.Sprintf("Logs: %s (%s, %s)", lv.name, scope, state)
	lv.pager.drawTitle()
}

func shortDuration(d time.Duration) string {
	if d >= time.Hour {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	return fmt.Sprintf("%dm", int(d/time.Minute))
}

// parseLogLine splits off the RFC 3339 timestamp the daemon prefixes.
func parseLogLine(s string) logLine {
	stamp, rest, found := strings.Cut(s, " ")
	if !found {
		if _, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return logLine{stamp: s} // an empty line still carries its timestamp
		}
		return logLine{text: s}
	}
	t, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return logLine{text: s}
	}
	return logLine{stamp: t.Format("2006-01-02T15:04:05Z07:00"), text: rest}
}
