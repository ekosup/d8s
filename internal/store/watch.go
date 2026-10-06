// Package store keeps a resource view fresh: it lists once, then refreshes
// when the daemon reports a relevant event, with polling as a fallback.
package store

import (
	"context"
	"slices"
	"time"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
)

// What triggered a snapshot.
const (
	SourceInitial = "initial"
	SourceEvent   = "event"
	SourcePoll    = "poll"
)

// Defaults for Options fields left at zero.
const (
	DefaultDebounce = 150 * time.Millisecond
	DefaultPoll     = 2 * time.Second
	DefaultRetry    = time.Second
)

// Snapshot is the result of one refresh. On error Rows is nil and the
// caller should keep showing what it had.
type Snapshot struct {
	Rows   []resource.Row
	Err    error
	At     time.Time
	Source string
}

// Options tunes a watcher. Zero values mean the defaults.
type Options struct {
	Debounce time.Duration // quiet time after an event before refreshing
	Poll     time.Duration // refresh interval regardless of events
	Retry    time.Duration // wait before subscribing to events again
}

func (o Options) withDefaults() Options {
	if o.Debounce <= 0 {
		o.Debounce = DefaultDebounce
	}
	if o.Poll <= 0 {
		o.Poll = DefaultPoll
	}
	if o.Retry <= 0 {
		o.Retry = DefaultRetry
	}
	return o
}

// Watch refreshes res until ctx is cancelled and hands every result to
// onUpdate, always from the same goroutine. The returned channel is closed
// once that goroutine has exited, after which onUpdate is never called again.
func Watch(ctx context.Context, client docker.Client, res resource.Resource, opts Options, onUpdate func(Snapshot)) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx, client, res, opts.withDefaults(), onUpdate)
	}()
	return done
}

func run(ctx context.Context, client docker.Client, res resource.Resource, opts Options, onUpdate func(Snapshot)) {
	refresh := func(source string) {
		rows, err := res.List(ctx, client)
		if ctx.Err() != nil {
			return
		}
		onUpdate(Snapshot{Rows: rows, Err: err, At: time.Now(), Source: source})
	}
	poll := time.NewTicker(opts.Poll)
	defer poll.Stop()

	// The debounce timer is created stopped; an event arms it.
	debounce := time.NewTimer(time.Hour)
	debounce.Stop()
	defer debounce.Stop()

	var (
		events <-chan docker.Event
		errs   <-chan error
		retry  <-chan time.Time
	)
	subscribe := func() {
		if len(res.EventTypes) > 0 {
			events, errs = client.Events(ctx)
		}
	}
	// Subscribe before the first list: an event that lands in between would
	// otherwise go unnoticed until the next poll.
	subscribe()
	refresh(SourceInitial)

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				events, errs = nil, nil
				retry = time.After(opts.Retry)
				continue
			}
			if slices.Contains(res.EventTypes, ev.Type) {
				debounce.Reset(opts.Debounce)
			}
		case <-errs:
			events, errs = nil, nil
			retry = time.After(opts.Retry)
		case <-retry:
			retry = nil
			subscribe()
			// Events may have been missed while disconnected.
			debounce.Reset(opts.Debounce)
		case <-debounce.C:
			refresh(SourceEvent)
		case <-poll.C:
			refresh(SourcePoll)
		}
	}
}
