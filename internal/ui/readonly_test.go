package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
	"github.com/ekosup/d8s/internal/resource"
)

func readOnlyHarness(t *testing.T, policy func(string) Policy) *harness {
	t.Helper()
	h := newHarness(t, fastWatch,
		docker.Container{ID: "1", Name: "api", Image: "nginx", State: "running", Status: "Up", Created: created},
		docker.Container{ID: "2", Name: "old", Image: "nginx", State: "exited", Status: "Exited (0)", Created: created},
	)
	h.app.setPolicy(policy)
	h.show("containers")
	h.step()
	return h
}

func always(p Policy) func(string) Policy { return func(string) Policy { return p } }

func TestReadOnlyIsShownInTheHeader(t *testing.T) {
	h := readOnlyHarness(t, always(Policy{ReadOnly: true}))
	header := strings.Join(headerLines(t, h.app, 120), "\n")
	if !strings.Contains(header, "READ-ONLY") {
		t.Fatalf("header does not say read-only:\n%s", header)
	}
	h2 := readOnlyHarness(t, always(Policy{}))
	if strings.Contains(strings.Join(headerLines(t, h2.app, 120), "\n"), "READ-ONLY") {
		t.Fatal("header says read-only when it is not")
	}
}

func TestReadOnlyRefusesEveryChange(t *testing.T) {
	h := readOnlyHarness(t, always(Policy{ReadOnly: true}))
	for _, key := range []struct {
		k tcell.Key
		r rune
	}{{tcell.KeyRune, 'r'}, {tcell.KeyRune, 'x'}, {tcell.KeyRune, 'a'}, {tcell.KeyRune, 'p'}, {tcell.KeyCtrlK, 0}, {tcell.KeyCtrlD, 0}} {
		h.app.key(key.k, key.r)
		if len(h.app.stack) != 1 {
			t.Fatalf("key %v/%c opened a dialog in read-only mode", key.k, key.r)
		}
		if s := h.screen(); !strings.Contains(s, "read-only") {
			t.Fatalf("no explanation after key %v/%c:\n%s", key.k, key.r, s)
		}
	}
	if calls := h.fake.Calls(); len(calls) != 0 {
		t.Fatalf("changes reached the daemon: %v", calls)
	}
}

func TestReadOnlyIsEnforcedBelowTheUI(t *testing.T) {
	// Even if a key slipped through, the executor refuses.
	h := readOnlyHarness(t, always(Policy{ReadOnly: true}))
	row, _ := h.app.view.SelectedRow()
	res, _ := h.app.registry.Lookup("containers")
	for _, act := range res.Actions {
		if !act.Mutates {
			continue
		}
		h.app.runAction(act, []resource.Row{row})
	}
	h.until("refusals reported", func() bool { return strings.Contains(h.screen(), "read-only") })
	if calls := h.fake.Calls(); len(calls) != 0 {
		t.Fatalf("the executor let changes through: %v", calls)
	}
}

func TestReadOnlyStillAllowsLooking(t *testing.T) {
	h := readOnlyHarness(t, always(Policy{ReadOnly: true}))
	h.app.key(tcell.KeyRune, 'h') // hide inactive: changes the view, not the daemon
	h.until("toggle works", func() bool { return strings.Contains(h.screen(), "(active only)[1]") })
	h.app.key(tcell.KeyRune, 'd')
	h.until("inspect works", func() bool { return len(h.app.stack) == 2 })
	h.app.key(tcell.KeyEscape, 0)
	h.app.key(tcell.KeyRune, 'l')
	h.until("logs work", func() bool { return len(h.app.stack) == 2 })
}

func TestReadOnlyBlocksShell(t *testing.T) {
	term := &fakeTerm{in: strings.NewReader("exit\n"), sizes: [][2]int{{24, 80}}}
	h := readOnlyHarness(t, always(Policy{ReadOnly: true}))
	h.app.openTerminal = func() (terminal, error) { return term, nil }
	h.app.suspend = func(f func()) bool { f(); return true }
	h.app.key(tcell.KeyRune, 's')
	if len(h.fake.Execs()) != 0 || term.rawCalls != 0 {
		t.Fatal("a shell was opened in read-only mode")
	}
	if !strings.Contains(h.screen(), "read-only") {
		t.Fatalf("no explanation:\n%s", h.screen())
	}
}

func TestPolicyFollowsTheContext(t *testing.T) {
	prod := dockertest.NewFake(
		dockertest.WithInfo(docker.Info{Context: "prod", ServerVersion: "27.5.1", APIVersion: "1.47"}),
		dockertest.WithContainers(docker.Container{ID: "9", Name: "prod-api", State: "running", Created: created}),
	)
	h := readOnlyHarness(t, func(ctx string) Policy {
		if ctx == "prod" {
			return Policy{ReadOnly: true, Production: true}
		}
		return Policy{}
	})
	if strings.Contains(strings.Join(headerLines(t, h.app, 120), "\n"), "READ-ONLY") {
		t.Fatal("default context should be writable")
	}
	h.app.connect = func(ctx context.Context, _ docker.Endpoint) (docker.Client, docker.Info, error) {
		info, _ := prod.Info(ctx)
		return prod, info, nil
	}
	h.app.switchTo(docker.Endpoint{Context: "prod", Host: "tcp://10.0.0.5:2376"})
	h.until("on prod", func() bool { return strings.Contains(h.screen(), "prod-api") })

	header := strings.Join(headerLines(t, h.app, 120), "\n")
	if !strings.Contains(header, "READ-ONLY") || !strings.Contains(header, "prod (production)") {
		t.Fatalf("header does not show the policy of the new context:\n%s", header)
	}
	h.app.key(tcell.KeyRune, 'r')
	if len(prod.Calls()) != 0 || !strings.Contains(h.screen(), "read-only") {
		t.Fatal("a change went through on a read-only context")
	}
}
