package store

import (
	"context"
	"sync"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

// EventLog remembers the most recent daemon events, for the events view.
type EventLog struct {
	mu     sync.Mutex
	max    int
	events []docker.Event
}

// NewEventLog returns a log that keeps the newest max events.
func NewEventLog(maxEvents int) *EventLog {
	return &EventLog{max: maxEvents}
}

// Run records events until ctx is cancelled, subscribing again after retry
// whenever the stream breaks.
func (l *EventLog) Run(ctx context.Context, client docker.Client, retry time.Duration) {
	for ctx.Err() == nil {
		events, errs := client.Events(ctx)
		l.drain(ctx, events, errs)
		select {
		case <-ctx.Done():
			return
		case <-time.After(retry):
		}
	}
}

func (l *EventLog) drain(ctx context.Context, events <-chan docker.Event, errs <-chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-errs:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			l.add(ev)
		}
	}
}

func (l *EventLog) add(ev docker.Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, ev)
	if over := len(l.events) - l.max; over > 0 {
		l.events = append(l.events[:0], l.events[over:]...)
	}
}

// Recent returns the remembered events, oldest first.
func (l *EventLog) Recent() []docker.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]docker.Event(nil), l.events...)
}
