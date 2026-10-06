package ui

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
	"github.com/ekosup/d8s/internal/resource"
)

var swarmClock = func() time.Time { return created.Add(time.Hour) }

func swarmHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, fastWatch)
	h.fake.SetSwarm(docker.SwarmInfo{Active: true, Manager: true, Leader: true, NodeID: "n1"})
	h.app.info.Swarm = docker.SwarmInfo{Active: true, Manager: true, Leader: true, NodeID: "n1"}
	h.app.drawHeader()
	h.fake.SetNodes(
		docker.Node{ID: "n1", Hostname: "mgr", State: "ready", Availability: "active", Role: "manager", Leader: true},
		docker.Node{ID: "n2", Hostname: "w1", State: "ready", Availability: "active", Role: "worker"},
	)
	h.fake.SetServices(
		docker.Service{ID: "s1", Name: "shop_web", Stack: "shop", Mode: "replicated", Image: "nginx:alpine", Desired: 3, Running: 3, Created: created},
		docker.Service{ID: "s2", Name: "shop_api", Stack: "shop", Mode: "replicated", Image: "nginx:alpine", Desired: 2, Running: 2, Created: created},
	)
	h.fake.SetTasks(
		docker.Task{ID: "t1", ServiceID: "s1", Slot: 1, NodeID: "n1", DesiredState: "running", State: "running", Image: "nginx:alpine", ContainerID: "c1", Timestamp: created},
		docker.Task{ID: "t2", ServiceID: "s1", Slot: 2, NodeID: "n2", DesiredState: "running", State: "running", Image: "nginx:alpine", ContainerID: "c2", Timestamp: created},
		docker.Task{ID: "t3", ServiceID: "s1", Slot: 3, NodeID: "n2", DesiredState: "shutdown", State: "shutdown", Image: "nginx:alpine", Timestamp: created},
	)
	h.register(resource.Services(swarmClock), resource.Tasks(swarmClock), resource.Nodes(swarmClock), resource.Stacks(swarmClock))
	return h
}

func TestHeaderShowsSwarmRole(t *testing.T) {
	h := swarmHarness(t)
	h.show("services")
	h.step()
	if line := render(t, h.app.root, 120, 24)[0]; !strings.Contains(line, "Swarm: manager (leader)") {
		t.Fatalf("header: %q", line)
	}
	h.app.info.Swarm = docker.SwarmInfo{Active: true}
	h.app.drawHeader()
	if line := render(t, h.app.root, 120, 24)[0]; !strings.Contains(line, "Swarm: worker") {
		t.Fatalf("header: %q", line)
	}
	h.app.info.Swarm = docker.SwarmInfo{}
	h.app.drawHeader()
	if line := render(t, h.app.root, 120, 24)[0]; strings.Contains(line, "Swarm") {
		t.Fatalf("header mentions swarm on a standalone engine: %q", line)
	}
}

func TestSwarmViewExplainsWhenNotAManager(t *testing.T) {
	tests := []struct {
		name string
		info docker.SwarmInfo
		want string
	}{
		{"standalone engine", docker.SwarmInfo{}, "not part of a swarm"},
		{"worker node", docker.SwarmInfo{Active: true}, "worker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := swarmHarness(t)
			h.app.info.Swarm = tt.info
			before := h.fake.ListCalls()
			h.show("services")
			s := h.screen()
			if !strings.Contains(s, tt.want) || !strings.Contains(s, "manager") || !strings.Contains(s, "<services>") {
				t.Fatalf("no explanation:\n%s", s)
			}
			time.Sleep(20 * time.Millisecond)
			if h.fake.ListCalls() != before {
				t.Fatal("the daemon was queried for a view it cannot serve")
			}
			// The rest of the app still works from here.
			h.command("containers")
			h.step()
			if !strings.Contains(h.screen(), "Containers[0]") {
				t.Fatal("could not leave the explanation page")
			}
		})
	}
}

func TestScaleAsksForANumber(t *testing.T) {
	h := swarmHarness(t)
	h.show("services")
	h.step()
	h.app.key(tcell.KeyRune, 'j') // shop_web (after shop_api)
	h.app.key(tcell.KeyRune, 's')
	s := h.screen()
	if !strings.Contains(s, "Scale shop_web") || !strings.Contains(s, "Replicas") {
		t.Fatalf("no input dialog:\n%s", s)
	}
	if got := h.app.input.GetText(); got != "3" {
		t.Fatalf("dialog should start with the current value, got %q", got)
	}
	// Typed characters go to the field, not to the table under it.
	h.app.key(tcell.KeyBackspace2, 0)
	h.app.typeText("5")
	h.app.key(tcell.KeyEnter, 0)
	h.until("scaled", func() bool { return strings.Contains(h.screen(), "3/5") })
	if got := h.fake.Log(); !slices.Equal(got, []string{"service s1 replicas=5"}) {
		t.Fatalf("log: %v", got)
	}
	if len(h.app.stack) != 1 || !strings.Contains(h.screen(), "Scale shop_web: done") {
		t.Fatalf("dialog not closed or no result:\n%s", h.screen())
	}
}

func TestInputDialogCancelAndValidation(t *testing.T) {
	h := swarmHarness(t)
	h.show("services")
	h.step()
	h.app.key(tcell.KeyRune, 's')
	h.app.key(tcell.KeyEscape, 0)
	if len(h.app.stack) != 1 || len(h.fake.Log()) != 0 {
		t.Fatal("esc did not cancel the dialog")
	}

	h.app.key(tcell.KeyRune, 's')
	h.app.key(tcell.KeyBackspace2, 0)
	h.app.typeText("many")
	h.app.key(tcell.KeyEnter, 0)
	h.until("validation error", func() bool { return strings.Contains(h.screen(), "whole number") })
	if len(h.fake.Log()) != 0 {
		t.Fatal("an invalid value reached the daemon")
	}
}

func TestDeleteServiceNeedsItsName(t *testing.T) {
	h := swarmHarness(t)
	h.show("services")
	h.step()
	h.app.key(tcell.KeyCtrlD, 0) // shop_api
	s := h.screen()
	if !strings.Contains(s, "Delete shop_api") || !strings.Contains(s, "type its name") {
		t.Fatalf("no typed confirmation:\n%s", s)
	}

	h.app.typeText("shop_ap")
	h.app.key(tcell.KeyEnter, 0)
	if len(h.fake.Log()) != 0 || len(h.app.stack) != 2 {
		t.Fatal("a wrong name was accepted")
	}
	if !strings.Contains(h.screen(), "does not match") {
		t.Fatalf("no feedback for the wrong name:\n%s", h.screen())
	}

	h.app.typeText("i")
	h.app.key(tcell.KeyEnter, 0)
	h.until("service removed", func() bool { return strings.Contains(h.screen(), "Services[1]") })
	if got := h.fake.Log(); !slices.Equal(got, []string{"remove service s2"}) {
		t.Fatalf("log: %v", got)
	}
}

func TestQuietToggleRefreshesAtOnce(t *testing.T) {
	h := swarmHarness(t)
	h.show("tasks")
	h.step()
	if !strings.Contains(h.screen(), "Tasks[3]") {
		t.Fatalf("expected all tasks:\n%s", h.screen())
	}
	h.app.key(tcell.KeyRune, 'h')
	// No event and a one-hour poll: only an explicit refresh can update the table.
	h.until("history hidden", func() bool { return strings.Contains(h.screen(), "Tasks[2]") })
	if strings.Contains(h.screen(), "History") && strings.Contains(h.screen(), ": done") {
		t.Fatalf("a quiet action left a status message:\n%s", h.screen())
	}
}

func TestStackToServiceToTaskDrillDown(t *testing.T) {
	h := swarmHarness(t)
	h.show("stacks")
	h.step()
	h.app.key(tcell.KeyEnter, 0)
	h.until("services of stack", func() bool { return strings.Contains(h.screen(), "shop_web") })
	h.app.key(tcell.KeyRune, 'j')
	h.app.key(tcell.KeyEnter, 0)
	h.until("tasks of service", func() bool { return strings.Contains(h.screen(), "shop_web.2") })
	if s := h.screen(); !strings.Contains(s, "<stacks> <services> <tasks>") || !strings.Contains(s, "w1") {
		t.Fatalf("drill-down chain wrong:\n%s", s)
	}
}

func TestShellOnAnotherNodeOffersItsContext(t *testing.T) {
	term := &fakeTerm{in: strings.NewReader(""), sizes: [][2]int{{24, 80}}}
	h := swarmHarness(t)
	h.app.openTerminal = func() (terminal, error) { return term, nil }
	h.app.suspend = func(f func()) bool { f(); return true }
	worker := dockertest.NewFake(dockertest.WithInfo(docker.Info{Context: "w1", ServerVersion: "27.5.1", APIVersion: "1.47"}))
	h.app.contexts = func() ([]docker.Endpoint, error) {
		return []docker.Endpoint{{Context: "default", Host: docker.DefaultHost}, {Context: "w1", Host: "tcp://10.0.0.2:2375"}}, nil
	}
	h.app.connect = func(ctx context.Context, _ docker.Endpoint) (docker.Client, docker.Info, error) {
		info, _ := worker.Info(ctx)
		return worker, info, nil
	}
	h.show("tasks")
	h.step()

	h.app.key(tcell.KeyRune, 'j') // shop_web.2, on w1
	h.app.key(tcell.KeyRune, 's')
	s := h.screen()
	if !strings.Contains(s, "runs on node w1") || !strings.Contains(s, "Switch to context w1?") {
		t.Fatalf("no explanation and offer:\n%s", s)
	}
	if term.raw || len(h.fake.Execs()) != 0 {
		t.Fatal("a shell was attempted on the wrong daemon")
	}
	h.app.key(tcell.KeyRune, 'y')
	h.until("switched", func() bool { return h.app.Context() == "w1" })
}

func TestShellOnAnotherNodeWithoutAContext(t *testing.T) {
	term := &fakeTerm{in: strings.NewReader(""), sizes: [][2]int{{24, 80}}}
	h := swarmHarness(t)
	h.app.openTerminal = func() (terminal, error) { return term, nil }
	h.app.suspend = func(f func()) bool { f(); return true }
	h.show("tasks")
	h.step()
	h.app.key(tcell.KeyRune, 'j')
	h.app.key(tcell.KeyRune, 's')
	if s := h.screen(); !strings.Contains(s, "runs on node w1") || len(h.app.stack) != 1 {
		t.Fatalf("expected a plain explanation:\n%s", s)
	}
}
