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
	calls      []Call
	actionErr  error
	inspect    map[string][]byte
	inspectErr error
}

// Call records one mutating request made through the fake.
type Call struct {
	Op docker.ContainerOp
	ID string
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

// ContainerAction implements docker.Client. It records the call, applies
// the state change a daemon would, and emits the matching event.
func (f *Fake) ContainerAction(_ context.Context, id string, op docker.ContainerOp) error {
	f.mu.Lock()
	if f.actionErr != nil {
		err := f.actionErr
		f.mu.Unlock()
		return err
	}
	f.calls = append(f.calls, Call{Op: op, ID: id})
	kept := f.containers[:0:0]
	for _, c := range f.containers {
		if c.ID != id {
			kept = append(kept, c)
			continue
		}
		switch op {
		case docker.OpStart, docker.OpRestart, docker.OpUnpause:
			c.State, c.Status = "running", "Up 1 second"
		case docker.OpStop:
			c.State, c.Status = "exited", "Exited (0) 1 second ago"
		case docker.OpKill:
			c.State, c.Status = "exited", "Exited (137) 1 second ago"
		case docker.OpPause:
			c.State, c.Status = "paused", "Up 1 second (Paused)"
		case docker.OpRemove:
			continue
		}
		kept = append(kept, c)
	}
	f.containers = kept
	f.mu.Unlock()
	f.Emit(docker.Event{Type: "container", Action: string(op), ID: id})
	return nil
}

// Calls returns the mutating requests made so far.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// SetActionError makes every mutating request fail with err; nil restores it.
func (f *Fake) SetActionError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actionErr = err
}

// Inspect implements docker.Client.
func (f *Fake) Inspect(_ context.Context, kind docker.Kind, id string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.inspectErr != nil {
		return nil, f.inspectErr
	}
	if raw, ok := f.inspect[string(kind)+"/"+id]; ok {
		return raw, nil
	}
	return []byte("{}"), nil
}

// SetInspect sets the JSON returned for one object.
func (f *Fake) SetInspect(kind docker.Kind, id string, raw []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.inspect == nil {
		f.inspect = map[string][]byte{}
	}
	f.inspect[string(kind)+"/"+id] = raw
}

// SetInspectError makes Inspect fail with err; nil restores it.
func (f *Fake) SetInspectError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspectErr = err
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
