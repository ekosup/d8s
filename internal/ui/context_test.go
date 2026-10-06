package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
	"github.com/ekosup/d8s/internal/resource"
)

type contextFixture struct {
	*harness
	staging    *dockertest.Fake
	connectErr error
	dialled    []string
}

func contextHarness(t *testing.T) *contextFixture {
	t.Helper()
	fx := &contextFixture{
		harness: newHarness(t, fastWatch, docker.Container{ID: "1", Name: "local-web", State: "running", Created: created}),
		staging: dockertest.NewFake(
			dockertest.WithInfo(docker.Info{Context: "staging", Host: "tcp://10.0.0.9:2376", ServerVersion: "26.1.4", APIVersion: "1.45"}),
			dockertest.WithContainers(docker.Container{ID: "9", Name: "staging-api", State: "running", Created: created}),
		),
	}
	eps := []docker.Endpoint{
		{Context: "default", Host: docker.DefaultHost},
		{Context: "staging", Host: "tcp://10.0.0.9:2376"},
	}
	fx.register(resource.Contexts(func() ([]docker.Endpoint, error) { return eps, nil }, fx.app.Context))
	fx.app.connect = func(ctx context.Context, ep docker.Endpoint) (docker.Client, docker.Info, error) {
		fx.dialled = append(fx.dialled, ep.Context)
		if fx.connectErr != nil {
			return nil, docker.Info{}, fx.connectErr
		}
		info, _ := fx.staging.Info(ctx)
		return fx.staging, info, nil
	}
	fx.show("contexts")
	fx.step()
	return fx
}

func TestContextViewMarksActive(t *testing.T) {
	fx := contextHarness(t)
	s := fx.screen()
	for _, want := range []string{"Contexts[2]", "ENDPOINT", "staging", "tcp://10.0.0.9:2376"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
	if row, _ := hintAt(headerLines(t, fx.app, 120), "enter", "Use"); row < 0 {
		t.Fatalf("no <enter> Use hint:\n%s", s)
	}
	lines := strings.Split(s, "\n")
	if i := lineContaining(lines, "unix:///var/run/docker.sock"); i < 0 || !strings.Contains(lines[i], "*") {
		t.Fatalf("active context not marked:\n%s", s)
	}
}

func TestSwitchContext(t *testing.T) {
	fx := contextHarness(t)
	old := fx.fake
	fx.app.key(tcell.KeyRune, 'j') // staging
	fx.app.key(tcell.KeyEnter, 0)
	fx.until("new context", func() bool { return strings.Contains(fx.screen(), "staging-api") })

	s := fx.screen()
	for _, want := range []string{"Context: staging", "Engine:  26.1.4", "API:     1.45", "<containers>", "connected to staging"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "local-web") {
		t.Fatalf("old daemon's containers still shown:\n%s", s)
	}
	if fx.app.Context() != "staging" || len(fx.dialled) != 1 || fx.dialled[0] != "staging" {
		t.Fatalf("context %q, dialled %v", fx.app.Context(), fx.dialled)
	}
	fx.until("old client closed", old.Closed)

	// Actions now go to the new daemon.
	fx.app.key(tcell.KeyRune, 'r')
	fx.until("restart on staging", func() bool { return len(fx.staging.Calls()) == 1 })
	if len(old.Calls()) != 0 {
		t.Fatal("action went to the old daemon")
	}
}

func TestSwitchContextFailureKeepsCurrent(t *testing.T) {
	fx := contextHarness(t)
	fx.connectErr = errors.New("docker socket not found at /run/user/1000/docker.sock")
	fx.app.key(tcell.KeyRune, 'j')
	fx.app.key(tcell.KeyEnter, 0)
	fx.until("error", func() bool { return strings.Contains(fx.screen(), "socket not found") })
	s := fx.screen()
	if !strings.Contains(s, "Context: default") || !strings.Contains(s, "<contexts>") || fx.app.Context() != "default" {
		t.Fatalf("a failed switch changed the connection:\n%s", s)
	}
	if fx.fake.Closed() {
		t.Fatal("the working client was closed")
	}
}

func TestSwitchToActiveContextIsNoOp(t *testing.T) {
	fx := contextHarness(t)
	fx.app.key(tcell.KeyEnter, 0) // default, already active
	if len(fx.dialled) != 0 {
		t.Fatalf("reconnected to the active context: %v", fx.dialled)
	}
}
