// Package dockertest provides an in-memory docker.Client for unit tests.
package dockertest

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

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

	logs       map[string][]string
	logErr     error
	logOpts    docker.LogOptions
	logStreams map[*logStream]struct{}

	execs    []*FakeExec
	execErr  error
	execCode int
}

type logStream struct {
	id string
	ch chan string
}

// LogTime is the timestamp the fake puts on every log line.
var LogTime = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

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

// ContainerLogs implements docker.Client. With Follow the stream stays
// open and delivers lines added through AppendLog until ctx is cancelled
// or the reader is closed.
func (f *Fake) ContainerLogs(ctx context.Context, id string, opts docker.LogOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logOpts = opts
	if f.logErr != nil {
		return nil, f.logErr
	}
	lines := f.logs[id]
	if opts.Tail > 0 && len(lines) > opts.Tail {
		lines = lines[len(lines)-opts.Tail:]
	}
	lines = append([]string(nil), lines...)

	stream := &logStream{id: id, ch: make(chan string, 1<<16)}
	if f.logStreams == nil {
		f.logStreams = map[*logStream]struct{}{}
	}
	f.logStreams[stream] = struct{}{}

	pr, pw := io.Pipe()
	format := func(l string) string {
		if opts.Timestamps {
			l = LogTime.Format(time.RFC3339Nano) + " " + l
		}
		return l + "\n"
	}
	go func() {
		defer func() {
			f.mu.Lock()
			delete(f.logStreams, stream)
			f.mu.Unlock()
			_ = pw.Close()
		}()
		for _, l := range lines {
			if _, err := io.WriteString(pw, format(l)); err != nil {
				return
			}
		}
		if !opts.Follow {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case l := <-stream.ch:
				if _, err := io.WriteString(pw, format(l)); err != nil {
					return
				}
			}
		}
	}()
	return pr, nil
}

// SetLogs sets the lines a container has already written.
func (f *Fake) SetLogs(id string, lines ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logs == nil {
		f.logs = map[string][]string{}
	}
	f.logs[id] = lines
}

// AppendLog makes the container write one more line.
func (f *Fake) AppendLog(id, line string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logs == nil {
		f.logs = map[string][]string{}
	}
	f.logs[id] = append(f.logs[id], line)
	for s := range f.logStreams {
		if s.id == id {
			s.ch <- line
		}
	}
}

// SetLogError makes ContainerLogs fail with err; nil restores it.
func (f *Fake) SetLogError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logErr = err
}

// LastLogOptions returns the options of the most recent ContainerLogs call.
func (f *Fake) LastLogOptions() docker.LogOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logOpts
}

// OpenLogStreams reports how many log streams are still being served.
func (f *Fake) OpenLogStreams() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.logStreams)
}

// FakeExec is an exec session served by the fake. It echoes every input
// line as "echo: <line>" and ends when it reads the line "exit".
type FakeExec struct {
	ID         string
	Cmd        []string
	Env        []string
	Rows, Cols uint

	mu      sync.Mutex
	input   []byte
	pending []byte // input not yet terminated by a newline
	resizes [][2]uint
	code    int
	outR    *io.PipeReader
	outW    *io.PipeWriter
}

func (e *FakeExec) Read(p []byte) (int, error) { return e.outR.Read(p) }

func (e *FakeExec) Write(p []byte) (int, error) {
	e.mu.Lock()
	e.input = append(e.input, p...)
	e.pending = append(e.pending, p...)
	var lines []string
	for {
		i := bytes.IndexByte(e.pending, '\n')
		if i < 0 {
			break
		}
		lines = append(lines, string(e.pending[:i]))
		e.pending = e.pending[i+1:]
	}
	e.mu.Unlock()
	for _, l := range lines {
		if l == "exit" {
			_ = e.outW.Close()
			break
		}
		_, _ = io.WriteString(e.outW, "echo: "+l+"\r\n")
	}
	return len(p), nil
}

// Close implements io.Closer.
func (e *FakeExec) Close() error {
	_ = e.outW.Close()
	return e.outR.Close()
}

// Resize implements docker.ExecSession.
func (e *FakeExec) Resize(_ context.Context, rows, cols uint) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.resizes = append(e.resizes, [2]uint{rows, cols})
	return nil
}

// ExitCode implements docker.ExecSession.
func (e *FakeExec) ExitCode(context.Context) (int, error) { return e.code, nil }

// Input returns everything written to the session.
func (e *FakeExec) Input() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return string(e.input)
}

// Resizes returns the sizes the session was resized to.
func (e *FakeExec) Resizes() [][2]uint {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([][2]uint(nil), e.resizes...)
}

// Exec implements docker.Client.
func (f *Fake) Exec(_ context.Context, id string, opts docker.ExecOptions) (docker.ExecSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.execErr != nil {
		return nil, f.execErr
	}
	pr, pw := io.Pipe()
	e := &FakeExec{ID: id, Cmd: opts.Cmd, Env: opts.Env, Rows: opts.Rows, Cols: opts.Cols, code: f.execCode, outR: pr, outW: pw}
	f.execs = append(f.execs, e)
	return e, nil
}

// Execs returns the exec sessions started so far.
func (f *Fake) Execs() []*FakeExec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*FakeExec(nil), f.execs...)
}

// SetExecError makes Exec fail with err; nil restores it.
func (f *Fake) SetExecError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execErr = err
}

// SetExecExitCode sets the exit code of sessions started from now on.
func (f *Fake) SetExecExitCode(code int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execCode = code
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
