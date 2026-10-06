package ui

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
)

// fakeTerm is a scripted terminal.
type fakeTerm struct {
	mu       sync.Mutex
	in       io.Reader
	out      bytes.Buffer
	sizes    [][2]int // returned by successive Size calls; the last one repeats
	raw      bool
	rawCalls int
	restored int
	closed   bool
}

func (f *fakeTerm) Read(p []byte) (int, error) { return f.in.Read(p) }
func (f *fakeTerm) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out.Write(p)
}
func (f *fakeTerm) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}
func (f *fakeTerm) MakeRaw() (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.raw = true
	f.rawCalls++
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.raw = false
		f.restored++
	}, nil
}
func (f *fakeTerm) Size() (rows, cols int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.sizes[0]
	if len(f.sizes) > 1 {
		f.sizes = f.sizes[1:]
	}
	return s[0], s[1], nil
}
func (f *fakeTerm) output() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out.String()
}

func shellHarness(t *testing.T, term *fakeTerm) *harness {
	t.Helper()
	h := newHarness(t, fastWatch, docker.Container{ID: "1", Name: "web", Image: "nginx", State: "running", Created: created})
	h.show("containers")
	h.step()
	h.app.openTerminal = func() (terminal, error) { return term, nil }
	h.app.suspend = func(f func()) bool { f(); return true }
	return h
}

func TestShellRunsInteractiveSession(t *testing.T) {
	term := &fakeTerm{in: strings.NewReader("ls\nexit\n"), sizes: [][2]int{{24, 80}}}
	h := shellHarness(t, term)
	suspended := false
	h.app.suspend = func(f func()) bool { suspended = true; f(); return true }

	h.app.key(tcell.KeyRune, 's')

	if !suspended {
		t.Fatal("the TUI was not suspended for the shell")
	}
	execs := h.fake.Execs()
	if len(execs) != 1 {
		t.Fatalf("exec sessions: %d", len(execs))
	}
	e := execs[0]
	if e.ID != "1" || e.Rows != 24 || e.Cols != 80 {
		t.Fatalf("session: %+v", e)
	}
	if got := strings.Join(e.Cmd, " "); !strings.Contains(got, "bash") || !strings.Contains(got, "exec sh") {
		t.Fatalf("command does not fall back from bash to sh: %q", got)
	}
	if !slices.ContainsFunc(e.Env, func(s string) bool { return strings.HasPrefix(s, "TERM=") }) {
		t.Fatalf("TERM not passed: %v", e.Env)
	}
	if got := e.Input(); got != "ls\nexit\n" {
		t.Fatalf("input forwarded: %q", got)
	}
	if out := term.output(); !strings.Contains(out, "echo: ls") {
		t.Fatalf("session output not shown: %q", out)
	}
	if term.rawCalls != 1 || term.restored != 1 || term.raw || !term.closed {
		t.Fatalf("terminal not restored: raw=%v calls=%d restored=%d closed=%v", term.raw, term.rawCalls, term.restored, term.closed)
	}
	if s := h.screen(); !strings.Contains(s, "shell on web exited") || !strings.Contains(s, "Containers[1]") {
		t.Fatalf("table not back after the shell:\n%s", s)
	}
}

func TestShellForwardsResize(t *testing.T) {
	pr, pw := io.Pipe()
	term := &fakeTerm{in: pr, sizes: [][2]int{{24, 80}, {24, 80}, {40, 120}}}
	h := shellHarness(t, term)
	h.app.resizePoll = 1 // nanosecond: check on every loop turn

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.app.key(tcell.KeyRune, 's')
	}()
	h.until("resize", func() bool {
		execs := h.fake.Execs()
		return len(execs) == 1 && slices.Contains(execs[0].Resizes(), [2]uint{40, 120})
	})
	_, _ = pw.Write([]byte("exit\n"))
	<-done
	_ = pw.Close()
}

func TestShellRestoresTerminalWhenExecFails(t *testing.T) {
	term := &fakeTerm{in: strings.NewReader(""), sizes: [][2]int{{24, 80}}}
	h := shellHarness(t, term)
	h.fake.SetExecError(errors.New("container is not running"))
	h.app.key(tcell.KeyRune, 's')
	if term.raw || !term.closed {
		t.Fatal("terminal left in raw mode")
	}
	if s := h.screen(); !strings.Contains(s, "container is not running") {
		t.Fatalf("error not shown:\n%s", s)
	}
}

func TestShellReportsExitCode(t *testing.T) {
	term := &fakeTerm{in: strings.NewReader("exit\n"), sizes: [][2]int{{24, 80}}}
	h := shellHarness(t, term)
	h.fake.SetExecExitCode(130)
	h.app.key(tcell.KeyRune, 's')
	if s := h.screen(); !strings.Contains(s, "exited with code 130") {
		t.Fatalf("exit code not shown:\n%s", s)
	}
}

func TestShellNeedsASelectedRow(t *testing.T) {
	term := &fakeTerm{in: strings.NewReader(""), sizes: [][2]int{{24, 80}}}
	h := newHarness(t, fastWatch)
	h.show("containers")
	h.step()
	h.app.openTerminal = func() (terminal, error) { return term, nil }
	h.app.key(tcell.KeyRune, 's')
	if len(h.fake.Execs()) != 0 {
		t.Fatal("shell opened with nothing selected")
	}
}
