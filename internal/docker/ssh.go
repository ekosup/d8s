package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// sshArgs builds the ssh command line that reaches a remote daemon, the way
// the docker CLI does it: ssh runs `docker system dial-stdio` on the far
// side and the API travels over its stdin and stdout. Authentication is
// whatever the user's ssh already does (keys and ~/.ssh/config).
func sshArgs(host string) ([]string, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", host, err)
	}
	if u.Scheme != "ssh" {
		return nil, fmt.Errorf("%q is not an ssh address", host)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("%q has no host", host)
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		return nil, errors.New("a password in an ssh address is not supported; use key-based login")
	}
	args := []string{"-o", "ConnectTimeout=30", "-T"}
	if user := u.User.Username(); user != "" {
		args = append(args, "-l", user)
	}
	if port := u.Port(); port != "" {
		args = append(args, "-p", port)
	}
	args = append(args, "--", u.Hostname(), "docker")
	if u.Path != "" {
		args = append(args, "--host", "unix://"+u.Path)
	}
	return append(args, "system", "dial-stdio"), nil
}

// commandConn is a net.Conn over a child process's stdin and stdout.
type commandConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *bytes.Buffer

	closeOnce sync.Once
	closeErr  error
}

// dialCommand starts name with args and returns a connection to it. The
// process lives as long as the connection, not as long as ctx: ctx only
// bounds the dial.
func dialCommand(_ context.Context, name string, args ...string) (net.Conn, error) {
	cmd := exec.Command(name, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	return &commandConn{cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr}, nil
}

func (c *commandConn) Read(p []byte) (int, error) {
	n, err := c.stdout.Read(p)
	if err != nil && n == 0 {
		// The process ended; what it printed on stderr is the reason.
		if msg := strings.TrimSpace(c.stderr.String()); msg != "" {
			return 0, fmt.Errorf("%s: %s: %w", c.cmd.Path, msg, err)
		}
	}
	return n, err
}

func (c *commandConn) Write(p []byte) (int, error) { return c.stdin.Write(p) }

func (c *commandConn) Close() error {
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		_ = c.cmd.Wait() // reap; its error is the kill we just sent
	})
	return c.closeErr
}

type commandAddr string

func (a commandAddr) Network() string { return "command" }
func (a commandAddr) String() string  { return string(a) }

func (c *commandConn) LocalAddr() net.Addr  { return commandAddr("local") }
func (c *commandConn) RemoteAddr() net.Addr { return commandAddr(c.cmd.Path) }

// Deadlines cannot be applied to a pipe; requests are bounded by their
// context instead.
func (c *commandConn) SetDeadline(time.Time) error      { return nil }
func (c *commandConn) SetReadDeadline(time.Time) error  { return nil }
func (c *commandConn) SetWriteDeadline(time.Time) error { return nil }
