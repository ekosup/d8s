//go:build integration

package docker

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// These tests need the development swarm: `make swarm-up`, then
// `make test-integration`, which points DOCKER_CONTEXT at it.

func liveClient(t *testing.T) (Client, context.Context) {
	t.Helper()
	ep, err := ResolveEndpoint(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ep.Context, "d8s-swarm") {
		t.Fatalf("refusing to run against context %q; these tests change cluster state and only run on the development swarm", ep.Context)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	c, err := Connect(ctx, ep)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, ctx
}

func serviceNamed(t *testing.T, c Client, ctx context.Context, name string) Service {
	t.Helper()
	services, err := c.Services(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range services {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("service %s not found", name)
	return Service{}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func TestLiveInfoAndNodes(t *testing.T) {
	c, ctx := liveClient(t)
	info, err := c.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Swarm.Active || !info.Swarm.Manager || !info.Swarm.Leader || info.Swarm.NodeID == "" {
		t.Fatalf("swarm info: %+v", info.Swarm)
	}
	nodes, err := c.Nodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	managers, ready := 0, 0
	for _, n := range nodes {
		if n.Role == "manager" {
			managers++
		}
		if n.State == "ready" {
			ready++
		}
		if n.Hostname == "" || n.EngineVersion == "" || n.Availability == "" {
			t.Errorf("incomplete node: %+v", n)
		}
	}
	if len(nodes) != 3 || managers != 1 || ready != 3 {
		t.Fatalf("%d nodes, %d managers, %d ready", len(nodes), managers, ready)
	}
}

func TestLiveServicesTasksAndStackObjects(t *testing.T) {
	c, ctx := liveClient(t)
	web := serviceNamed(t, c, ctx, "shop_web")
	if web.Stack != "shop" || web.Mode != "replicated" || web.Desired != 3 || !strings.HasPrefix(web.Image, "nginx") {
		t.Fatalf("service: %+v", web)
	}
	if len(web.Ports) != 1 || web.Ports[0].Public != 8080 || web.Ports[0].Private != 80 {
		t.Fatalf("ports: %+v", web.Ports)
	}
	if len(web.Secrets) != 1 || len(web.Configs) != 1 {
		t.Fatalf("secrets %v configs %v", web.Secrets, web.Configs)
	}

	tasks, err := c.Tasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	running := 0
	for _, task := range tasks {
		if task.ServiceID == web.ID && task.State == "running" {
			running++
			if task.NodeID == "" || task.Slot == 0 || task.ContainerID == "" {
				t.Errorf("incomplete task: %+v", task)
			}
		}
	}
	if running != 3 {
		t.Fatalf("%d running tasks of shop_web", running)
	}

	configs, err := c.Configs(ctx)
	if err != nil || len(configs) != 1 || configs[0].Stack != "shop" {
		t.Fatalf("configs %+v, err %v", configs, err)
	}
	data, err := c.ConfigData(ctx, configs[0].ID)
	if err != nil || !strings.Contains(string(data), "d8s sample stack") {
		t.Fatalf("config data %q, err %v", data, err)
	}
	secrets, err := c.Secrets(ctx)
	if err != nil || len(secrets) != 1 || secrets[0].Name != "shop_token" {
		t.Fatalf("secrets %+v, err %v", secrets, err)
	}
	raw, err := c.Inspect(ctx, KindSecret, secrets[0].ID)
	if err != nil || strings.Contains(string(raw), "not-a-real-secret") {
		t.Fatalf("secret inspect leaked the value or failed: %v", err)
	}
}

func TestLiveServiceLogsAreLabelled(t *testing.T) {
	c, ctx := liveClient(t)
	web := serviceNamed(t, c, ctx, "shop_web")
	rc, err := c.ServiceLogs(ctx, web.ID, LogOptions{Tail: 50, Timestamps: true})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	sc := bufio.NewScanner(rc)
	lines, nodes := 0, map[string]bool{}
	for sc.Scan() {
		line := sc.Text()
		stamp, rest, _ := strings.Cut(line, " ")
		if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
			t.Fatalf("no timestamp in %q", line)
		}
		label, _, ok := strings.Cut(rest, " | ")
		task, node, ok2 := strings.Cut(label, "@")
		if !ok || !ok2 || !strings.HasPrefix(task, "shop_web.") || !strings.HasPrefix(node, "d8s-swarm-") {
			t.Fatalf("line not labelled with task and node: %q", line)
		}
		nodes[node] = true
		lines++
	}
	if lines == 0 || len(nodes) < 2 {
		t.Fatalf("%d lines from %d nodes; expected output from tasks on several nodes", lines, len(nodes))
	}
}

func TestLiveScaleAndNodeAvailability(t *testing.T) {
	c, ctx := liveClient(t)
	api := serviceNamed(t, c, ctx, "shop_api")
	four := uint64(4)
	if err := c.ServiceUpdate(ctx, api.ID, ServiceChange{Replicas: &four}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "4/4 replicas", func() bool {
		s := serviceNamed(t, c, ctx, "shop_api")
		return s.Desired == 4 && s.Running == 4
	})
	two := uint64(2)
	if err := c.ServiceUpdate(ctx, api.ID, ServiceChange{Replicas: &two}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "2/2 replicas", func() bool {
		s := serviceNamed(t, c, ctx, "shop_api")
		return s.Desired == 2 && s.Running == 2
	})

	nodes, err := c.Nodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var worker Node
	for _, n := range nodes {
		if n.Role == "worker" {
			worker = n
			break
		}
	}
	if err := c.NodeUpdate(ctx, worker.ID, NodeChange{Availability: "drain", SetLabels: map[string]string{"d8s.test": "1"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = c.NodeUpdate(context.Background(), worker.ID, NodeChange{Availability: "active", RemoveLabels: []string{"d8s.test"}})
	})
	eventually(t, "node drained and labelled", func() bool {
		ns, _ := c.Nodes(ctx)
		for _, n := range ns {
			if n.ID == worker.ID {
				return n.Availability == "drain" && n.Labels["d8s.test"] == "1"
			}
		}
		return false
	})
}
