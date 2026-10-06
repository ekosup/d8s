package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
	"github.com/ekosup/d8s/internal/resource"
)

var managerInfo = docker.SwarmInfo{Active: true, Manager: true, Leader: true, NodeID: "n1"}

func homeHarness(t *testing.T, swarm docker.SwarmInfo, home string) *harness {
	t.Helper()
	h := newHarness(t, fastWatch, docker.Container{ID: "1", Name: "web", State: "running", Created: created})
	h.app.info.Swarm = swarm
	h.fake.SetSwarm(swarm)
	h.fake.SetServices(docker.Service{ID: "s1", Name: "shop_web", Mode: "replicated", Desired: 1, Running: 1, Created: created})
	h.register(resource.Services(swarmClock), resource.Images(swarmClock))
	h.app.home = home
	return h
}

func TestHomeViewFollowsTheCluster(t *testing.T) {
	tests := []struct {
		name  string
		swarm docker.SwarmInfo
		home  string
		want  string
	}{
		{"auto on a swarm manager", managerInfo, "auto", "<services>"},
		{"unset behaves like auto", managerInfo, "", "<services>"},
		{"auto on a standalone engine", docker.SwarmInfo{}, "auto", "<containers>"},
		{"auto on a swarm worker", docker.SwarmInfo{Active: true}, "auto", "<containers>"},
		{"an explicit view wins on a manager", managerInfo, "containers", "<containers>"},
		{"an explicit view wins on a standalone engine", docker.SwarmInfo{}, "images", "<images>"},
		{"an alias of a view works", managerInfo, "i", "<images>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := homeHarness(t, tt.swarm, tt.home)
			if err := h.app.ShowHome(); err != nil {
				t.Fatal(err)
			}
			h.step()
			if s := h.screen(); !strings.Contains(s, tt.want) {
				t.Fatalf("want %s:\n%s", tt.want, s)
			}
		})
	}
}

func TestHomeViewRejectsAnUnknownView(t *testing.T) {
	h := homeHarness(t, docker.SwarmInfo{}, "nosuchview")
	if err := h.app.ShowHome(); err == nil || !strings.Contains(err.Error(), "nosuchview") {
		t.Fatalf("err = %v", err)
	}
}

func TestHomeViewIsChosenAgainAfterSwitchingContext(t *testing.T) {
	h := homeHarness(t, docker.SwarmInfo{}, "auto")
	if err := h.app.ShowHome(); err != nil {
		t.Fatal(err)
	}
	h.step()
	if !strings.Contains(h.screen(), "<containers>") {
		t.Fatalf("standalone engine should start on containers:\n%s", h.screen())
	}

	manager := dockertest.NewFake(dockertest.WithInfo(docker.Info{Context: "prod", ServerVersion: "27.5.1", APIVersion: "1.47", Swarm: managerInfo}))
	manager.SetServices(docker.Service{ID: "s9", Name: "prod_api", Mode: "replicated", Desired: 2, Running: 2, Created: created})
	h.app.connect = func(ctx context.Context, _ docker.Endpoint) (docker.Client, docker.Info, error) {
		info, _ := manager.Info(ctx)
		return manager, info, nil
	}
	h.app.switchTo(docker.Endpoint{Context: "prod", Host: "tcp://10.0.0.5:2376"})
	h.until("services of the manager", func() bool { return strings.Contains(h.screen(), "prod_api") })
	if !strings.Contains(h.screen(), "<services>") {
		t.Fatalf("a manager should open on services:\n%s", h.screen())
	}
}
