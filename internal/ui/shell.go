package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
)

// shellCommand prefers bash and falls back to sh, in one exec.
var shellCommand = []string{"sh", "-c", "command -v bash >/dev/null 2>&1 && exec bash || exec sh"}

// terminal is the user's terminal while the TUI is suspended.
type terminal interface {
	io.ReadWriteCloser
	// MakeRaw switches to raw mode and returns the function that undoes it.
	MakeRaw() (restore func(), err error)
	Size() (rows, cols int, err error)
}

// tty is the controlling terminal, opened on its own descriptor. Closing
// it unblocks a pending read, which a read on os.Stdin would not: the
// stranded reader would then eat the first key typed back in the TUI.
type tty struct{ *os.File }

func openTTY() (terminal, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open terminal: %w", err)
	}
	return tty{f}, nil
}

// withFd runs f with the terminal's descriptor. It goes through
// SyscallConn on purpose: File.Fd would switch the descriptor to blocking
// mode, and then closing the file could no longer interrupt a pending
// read. That read would survive the shell and swallow the first key typed
// back in the TUI.
func (t tty) withFd(f func(fd int) error) error {
	conn, err := t.SyscallConn()
	if err != nil {
		return err
	}
	var inner error
	if err := conn.Control(func(fd uintptr) { inner = f(int(fd)) }); err != nil {
		return err
	}
	return inner
}

func (t tty) MakeRaw() (func(), error) {
	var state *term.State
	err := t.withFd(func(fd int) error {
		var err error
		state, err = term.MakeRaw(fd)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("set terminal raw mode: %w", err)
	}
	return func() {
		_ = t.withFd(func(fd int) error { return term.Restore(fd, state) })
	}, nil
}

func (t tty) Size() (rows, cols int, err error) {
	err = t.withFd(func(fd int) error {
		var err error
		cols, rows, err = term.GetSize(fd)
		return err
	})
	return rows, cols, err
}

// openShell suspends the TUI and attaches the terminal to a shell in the
// selected row's container.
func (a *App) openShell(res resource.Resource, view *tableView) {
	row, ok := view.SelectedRow()
	if !ok || a.client == nil {
		return
	}
	var (
		code int
		err  error
	)
	ran := a.suspend(func() {
		code, err = a.runShell(func(ctx context.Context, rows, cols uint) (docker.ExecSession, error) {
			return res.Exec(ctx, a.client, row, docker.ExecOptions{
				Cmd:  shellCommand,
				Env:  []string{"TERM=" + termName()},
				Rows: rows,
				Cols: cols,
			})
		})
	})
	var other *docker.ErrOtherNode
	switch {
	case !ran:
		a.Flash(flashError, "shell: cannot suspend the screen")
	case errors.As(err, &other):
		// Exec cannot cross nodes. Say so, and offer the way there when a
		// context for that node is known.
		reason := strings.ToUpper(err.Error()[:1]) + err.Error()[1:] + "."
		if ep, ok := a.contextNamed(other.Node); ok {
			a.confirm(reason+"\nSwitch to context "+ep.Context+"?", func() { a.switchTo(ep) })
		} else {
			a.Flash(flashWarn, reason)
		}
	case err != nil:
		a.Flash(flashError, "shell on "+row.Name()+": "+oneLine(err.Error()))
	case code != 0:
		a.Flash(flashWarn, fmt.Sprintf("shell on %s exited with code %d", row.Name(), code))
	default:
		a.Flash(flashInfo, "shell on "+row.Name()+" exited")
	}
}

func termName() string {
	if t := os.Getenv("TERM"); t != "" {
		return t
	}
	return "xterm-256color"
}

// runShell connects the terminal to a session until the session ends. The
// terminal is always put back the way it was.
func (a *App) runShell(open func(ctx context.Context, rows, cols uint) (docker.ExecSession, error)) (int, error) {
	t, err := a.openTerminal()
	if err != nil {
		return 0, err
	}
	defer func() { _ = t.Close() }()

	rows, cols, err := t.Size()
	if err != nil {
		return 0, fmt.Errorf("read terminal size: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess, err := open(ctx, uint(max(rows, 0)), uint(max(cols, 0)))
	if err != nil {
		return 0, err
	}
	defer func() { _ = sess.Close() }()

	restore, err := t.MakeRaw()
	if err != nil {
		return 0, err
	}
	defer restore()

	// Keys to the container. This ends when the terminal is closed above.
	go func() { _, _ = io.Copy(sess, t) }()

	// Follow the terminal's size. Polling needs no signal handling and
	// behaves the same on every platform.
	go func() {
		tick := time.NewTicker(a.resizePoll)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				r, c, err := t.Size()
				if err != nil || (r == rows && c == cols) {
					continue
				}
				rows, cols = r, c
				_ = sess.Resize(ctx, uint(max(r, 0)), uint(max(c, 0)))
			}
		}
	}()

	// Output to the terminal, until the shell exits.
	if _, err := io.Copy(t, sess); err != nil && !errors.Is(err, io.EOF) && ctx.Err() == nil {
		return 0, fmt.Errorf("shell stream: %w", err)
	}
	codeCtx, codeCancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer codeCancel()
	return sess.ExitCode(codeCtx)
}
