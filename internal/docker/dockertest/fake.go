// Package dockertest provides an in-memory docker.Client for unit tests.
package dockertest

import (
	"context"
	"sync"

	"github.com/ekosup/d8s/internal/docker"
)

// Fake is a docker.Client backed by memory. It is safe for concurrent use.
type Fake struct {
	mu         sync.Mutex
	info       docker.Info
	containers []docker.Container
	listErr    error
	listCalls  int
	eventCalls int
	subs       []subscription
}

type subscription struct {
	ctx  context.Context
	msgs chan docker.Event
	errs chan error
}

// Option configures a Fake.
type Option func(*Fake)

// WithContainers sets the initial container list.
func WithContainers(cs ...docker.Container) Option {
	return func(f *Fake) { f.containers = cs }
}

// WithInfo sets what Info returns.
func WithInfo(info docker.Info) Option {
	return func(f *Fake) { f.info = info }
}

// NewFake returns a Fake with the given options applied.
func NewFake(opts ...Option) *Fake {
	f := &Fake{info: docker.Info{Context: "fake", Host: "unix:///fake.sock", ServerVersion: "27.3.1", APIVersion: "1.47"}}
	for _, o := range opts {
		o(f)
	}
	return f
}

var _ docker.Client = (*Fake)(nil)

// Info implements docker.Client.
func (f *Fake) Info(context.Context) (docker.Info, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.info, nil
}

// Containers implements docker.Client.
func (f *Fake) Containers(context.Context) ([]docker.Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]docker.Container(nil), f.containers...), nil
}

// Events implements docker.Client. Every call is a new subscription.
func (f *Fake) Events(ctx context.Context) (<-chan docker.Event, <-chan error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.eventCalls++
	s := subscription{ctx: ctx, msgs: make(chan docker.Event, 64), errs: make(chan error, 1)}
	f.subs = append(f.subs, s)
	return s.msgs, s.errs
}

// Close implements docker.Client.
func (f *Fake) Close() error { return nil }

// SetContainers replaces the container list.
func (f *Fake) SetContainers(cs ...docker.Container) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.containers = cs
}

// SetListError makes Containers fail with err; nil restores it.
func (f *Fake) SetListError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listErr = err
}

// Emit delivers ev to every live subscription.
func (f *Fake) Emit(ev docker.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.subs {
		if s.ctx.Err() != nil {
			continue
		}
		select {
		case s.msgs <- ev:
		default:
		}
	}
}

// FailEvents ends every live subscription with err, as a dropped connection would.
func (f *Fake) FailEvents(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.subs {
		select {
		case s.errs <- err:
		default:
		}
	}
	f.subs = nil
}

// ListCalls reports how many times Containers was called.
func (f *Fake) ListCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls
}

// EventCalls reports how many times Events was called.
func (f *Fake) EventCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.eventCalls
}
