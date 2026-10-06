package resource

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
)

func swarmFake() *dockertest.Fake {
	f := dockertest.NewFake()
	f.SetSwarm(docker.SwarmInfo{Active: true, Manager: true, Leader: true, NodeID: "n1"})
	f.SetNodes(
		docker.Node{ID: "n1", Hostname: "mgr", State: "ready", Availability: "active", Role: "manager", Leader: true, Reachability: "reachable", EngineVersion: "27.5.1", Addr: "10.0.0.1"},
		docker.Node{ID: "n2", Hostname: "w1", State: "ready", Availability: "drain", Role: "worker", EngineVersion: "27.5.1", Addr: "10.0.0.2", Labels: map[string]string{"zone": "a"}},
		docker.Node{ID: "n3", Hostname: "w2", State: "down", Availability: "active", Role: "worker", EngineVersion: "26.1.4", Addr: "10.0.0.3"},
	)
	f.SetServices(
		docker.Service{ID: "s1", Name: "shop_web", Stack: "shop", Mode: "replicated", Image: "nginx:bad", PreviousImage: "nginx:alpine", Desired: 3, Running: 1,
			Ports: []docker.Port{{Public: 8080, Private: 80, Proto: "tcp"}}, UpdateState: "paused", UpdateMessage: "update paused due to failure",
			Secrets: []string{"shop_token"}, Configs: []string{"shop_motd"}, Created: now.Add(-48 * time.Hour)},
		docker.Service{ID: "s2", Name: "shop_api", Stack: "shop", Mode: "replicated", Image: "nginx:alpine@sha256:abcdef", Desired: 2, Running: 2, UpdateState: "completed", Created: now.Add(-time.Hour)},
		docker.Service{ID: "s3", Name: "probe", Mode: "global", Image: "prom/node-exporter", Desired: 3, Running: 3, Created: now.Add(-time.Minute)},
	)
	f.SetTasks(
		docker.Task{ID: "t1", ServiceID: "s1", Slot: 1, NodeID: "n1", DesiredState: "running", State: "running", Image: "nginx:alpine", ContainerID: "c-t1", Timestamp: now.Add(-2 * time.Hour)},
		docker.Task{ID: "t2", ServiceID: "s1", Slot: 2, NodeID: "n2", DesiredState: "shutdown", State: "rejected", Image: "nginx:bad", Timestamp: now.Add(-time.Minute),
			Err: `No such image: nginx:bad — pull access denied for nginx, repository does not exist or may require 'docker login'`},
		docker.Task{ID: "t3", ServiceID: "s1", Slot: 2, NodeID: "n2", DesiredState: "shutdown", State: "shutdown", Image: "nginx:alpine", Timestamp: now.Add(-3 * time.Minute)},
		docker.Task{ID: "t4", ServiceID: "s2", Slot: 1, NodeID: "n2", DesiredState: "running", State: "running", Image: "nginx:alpine", ContainerID: "c-t4", Timestamp: now.Add(-time.Hour)},
		docker.Task{ID: "t5", ServiceID: "s3", Slot: 0, NodeID: "n1", DesiredState: "running", State: "running", Image: "prom/node-exporter", ContainerID: "c-t5", Timestamp: now.Add(-time.Minute)},
	)
	f.SetSecrets(docker.Secret{ID: "sec1", Name: "shop_token", Stack: "shop", Created: now.Add(-48 * time.Hour), Updated: now.Add(-48 * time.Hour)},
		docker.Secret{ID: "sec2", Name: "unused", Created: now.Add(-time.Hour), Updated: now.Add(-time.Hour)})
	f.SetConfigs(docker.Config{ID: "cfg1", Name: "shop_motd", Stack: "shop", Created: now.Add(-48 * time.Hour)})
	f.SetConfigData("cfg1", []byte("Hello\nsecond line\n"))
	return f
}

func listRows(t *testing.T, res Resource, f *dockertest.Fake) []Row {
	t.Helper()
	rows, err := res.List(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if len(r.Cells) != len(res.Columns) {
			t.Fatalf("row %q has %d cells for %d columns", r.Name(), len(r.Cells), len(res.Columns))
		}
	}
	return rows
}

func TestNodeRows(t *testing.T) {
	res := Nodes(fixedNow)
	if !res.Swarm {
		t.Fatal("nodes must be marked as a swarm resource")
	}
	rows := listRows(t, res, swarmFake())
	want := []string{
		"mgr | ready | active | manager | leader | 27.5.1 | 10.0.0.1",
		"w1 | ready | drain | worker |  | 27.5.1 | 10.0.0.2",
		"w2 | down | active | worker |  | 26.1.4 | 10.0.0.3",
	}
	if got := cellsOf(rows); !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
	if rows[0].Tone != ToneNormal || rows[1].Tone != ToneWarn || rows[2].Tone != ToneBad {
		t.Fatalf("down and drained nodes must look different: %v %v %v", rows[0].Tone, rows[1].Tone, rows[2].Tone)
	}
}

func TestNodeActions(t *testing.T) {
	f := swarmFake()
	res := Nodes(fixedNow)
	rows := listRows(t, res, f)
	ctx := context.Background()

	avail := actionByKey(t, res, "a")
	if avail.Input == nil || avail.Input.Default(rows[1]) != "drain" {
		t.Fatalf("availability input: %+v", avail.Input)
	}
	if err := avail.RunInput(ctx, f, rows[0], "Drain "); err != nil {
		t.Fatal(err)
	}
	if err := avail.RunInput(ctx, f, rows[0], "sleepy"); err == nil {
		t.Fatal("accepted an unknown availability")
	}

	role := actionByKey(t, res, "p")
	if !role.Confirm {
		t.Fatal("changing a node's role must confirm")
	}
	if err := role.Run(ctx, f, rows[1]); err != nil { // worker -> manager
		t.Fatal(err)
	}
	if err := role.Run(ctx, f, rows[0]); err != nil { // manager -> worker
		t.Fatal(err)
	}

	label := actionByKey(t, res, "b")
	for _, v := range []string{"tier=db", "zone-"} {
		if err := label.RunInput(ctx, f, rows[1], v); err != nil {
			t.Fatal(err)
		}
	}
	for _, bad := range []string{"", "=x", "novalue"} {
		if err := label.RunInput(ctx, f, rows[1], bad); err == nil {
			t.Fatalf("accepted label input %q", bad)
		}
	}
	want := []string{"node n1 availability=drain", "node n2 role=manager", "node n1 role=worker", "node n2 label tier=db", "node n2 unlabel zone"}
	if got := f.Log(); !slices.Equal(got, want) {
		t.Fatalf("log: %v", got)
	}
}

func TestServiceRows(t *testing.T) {
	res := Services(fixedNow)
	rows := listRows(t, res, swarmFake())
	want := []string{
		"shop_web | shop | replicated | 1/3 | nginx:bad | 8080->80/tcp | paused | 2d",
		"shop_api | shop | replicated | 2/2 | nginx:alpine | - | completed | 1h",
		"probe |  | global | 3/3 | prom/node-exporter | - | - | 1m",
	}
	if got := cellsOf(rows); !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
	if rows[0].Tone != ToneBad || rows[1].Tone != ToneNormal {
		t.Fatalf("a paused update must stand out: %v %v", rows[0].Tone, rows[1].Tone)
	}
}

func TestServiceUnderReplicatedIsAWarning(t *testing.T) {
	f := swarmFake()
	f.SetServices(docker.Service{ID: "s", Name: "x", Mode: "replicated", Desired: 5, Running: 2, Created: now})
	if rows := listRows(t, Services(fixedNow), f); rows[0].Tone != ToneWarn {
		t.Fatalf("tone %v", rows[0].Tone)
	}
}

func TestServiceActions(t *testing.T) {
	f := swarmFake()
	res := Services(fixedNow)
	rows := listRows(t, res, f)
	ctx := context.Background()

	scale := actionByKey(t, res, "s")
	if scale.Input == nil || scale.Input.Default(rows[0]) != "3" {
		t.Fatalf("scale input: %+v", scale.Input)
	}
	if err := scale.RunInput(ctx, f, rows[0], " 5 "); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "-1", "many", "1.5"} {
		if err := scale.RunInput(ctx, f, rows[0], bad); err == nil {
			t.Fatalf("accepted replica count %q", bad)
		}
	}

	image := actionByKey(t, res, "i")
	if got := image.Input.Default(rows[1]); got != "nginx:alpine" {
		t.Fatalf("image default should drop the digest: %q", got)
	}
	if err := image.RunInput(ctx, f, rows[1], "nginx:1.27"); err != nil {
		t.Fatal(err)
	}
	if err := image.RunInput(ctx, f, rows[1], "  "); err == nil {
		t.Fatal("accepted an empty image")
	}

	if err := actionByKey(t, res, "r").Run(ctx, f, rows[2]); err != nil {
		t.Fatal(err)
	}
	rollback := actionByKey(t, res, "u")
	if !rollback.Confirm || !strings.Contains(rollback.Warn(rows[0]), "nginx:alpine") {
		t.Fatalf("rollback should confirm and name the image it returns to: %q", rollback.Warn(rows[0]))
	}
	if w := rollback.Warn(rows[2]); !strings.Contains(w, "no previous") {
		t.Fatalf("rollback without a previous spec should say so: %q", w)
	}
	if err := rollback.Run(ctx, f, rows[0]); err != nil {
		t.Fatal(err)
	}
	del := actionByKey(t, res, "ctrl-d")
	if !del.ConfirmName {
		t.Fatal("deleting a service must ask for its name")
	}
	if err := del.Run(ctx, f, rows[2]); err != nil {
		t.Fatal(err)
	}
	want := []string{"service s1 replicas=5", "service s2 image=nginx:1.27", "service s3 force", "service s1 rollback", "remove service s3"}
	if got := f.Log(); !slices.Equal(got, want) {
		t.Fatalf("log: %v", got)
	}
}

func TestTaskRows(t *testing.T) {
	f := swarmFake()
	rows := listRows(t, Tasks(fixedNow), f)
	want := []string{
		"shop_web.1 | mgr | running | running |  | nginx:alpine | 2h",
		"shop_web.2 | w1 | shutdown | rejected | No such image: nginx:bad — pull access denied for nginx, repository does not exist or may require 'docker login' | nginx:bad | 1m",
		"shop_web.2 | w1 | shutdown | shutdown |  | nginx:alpine | 3m",
		"shop_api.1 | w1 | running | running |  | nginx:alpine | 1h",
		"probe.n1 | mgr | running | running |  | prom/node-exporter | 1m",
	}
	if got := cellsOf(rows); !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
	if rows[1].Tone != ToneBad || rows[2].Tone != ToneMuted || rows[0].Tone != ToneNormal {
		t.Fatalf("tones: %v %v %v", rows[0].Tone, rows[1].Tone, rows[2].Tone)
	}
}

func TestTaskHistoryToggle(t *testing.T) {
	f := swarmFake()
	res := Tasks(fixedNow)
	toggle := actionByKey(t, res, "h")
	if toggle.Target == "" || toggle.Mutates || !toggle.Quiet {
		t.Fatalf("history toggle: %+v", toggle)
	}
	if err := toggle.Run(context.Background(), f, Row{}); err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, r := range listRows(t, res, f) {
		names = append(names, r.Cells[0]+"/"+r.Cells[3])
	}
	// Cleanly replaced tasks are hidden; the failed one stays, since it is the reason to look.
	if !slices.Equal(names, []string{"shop_web.1/running", "shop_web.2/rejected", "shop_api.1/running", "probe.n1/running"}) {
		t.Fatalf("after hiding history: %v", names)
	}
}

func TestDrillDowns(t *testing.T) {
	f := swarmFake()
	firstCells := func(res Resource) []string {
		out := []string{}
		for _, r := range listRows(t, res, f) {
			out = append(out, r.Cells[0])
		}
		return out
	}
	services := Services(fixedNow)
	child, ok := services.Open(listRows(t, services, f)[0])
	if !ok || !slices.Equal(firstCells(child), []string{"shop_web.1", "shop_web.2", "shop_web.2"}) {
		t.Fatalf("service -> tasks: %v", firstCells(child))
	}
	nodes := Nodes(fixedNow)
	child, ok = nodes.Open(listRows(t, nodes, f)[1])
	if !ok || !slices.Equal(firstCells(child), []string{"shop_web.2", "shop_web.2", "shop_api.1"}) {
		t.Fatalf("node -> tasks: %v", firstCells(child))
	}
	stacks := Stacks(fixedNow)
	child, ok = stacks.Open(listRows(t, stacks, f)[0])
	if !ok || !slices.Equal(firstCells(child), []string{"shop_web", "shop_api"}) {
		t.Fatalf("stack -> services: %v", firstCells(child))
	}
}

func TestTaskShellOnlyOnTheConnectedNode(t *testing.T) {
	f := swarmFake()
	res := Tasks(fixedNow)
	rows := listRows(t, res, f)
	ctx := context.Background()

	if _, err := res.Exec(ctx, f, rows[0], docker.ExecOptions{Cmd: []string{"sh"}}); err != nil {
		t.Fatal(err)
	}
	if execs := f.Execs(); len(execs) != 1 || execs[0].ID != "c-t1" {
		t.Fatalf("exec did not target the task's container: %+v", execs)
	}

	_, err := res.Exec(ctx, f, rows[3], docker.ExecOptions{Cmd: []string{"sh"}})
	var other *docker.ErrOtherNode
	if !errors.As(err, &other) || other.Node != "w1" {
		t.Fatalf("expected ErrOtherNode for w1, got %v", err)
	}
	if _, err := res.Exec(ctx, f, rows[1], docker.ExecOptions{}); err == nil {
		t.Fatal("exec into a task without a container should fail")
	}
}

func TestStackRows(t *testing.T) {
	f := swarmFake()
	res := Stacks(fixedNow)
	rows := listRows(t, res, f)
	if got := cellsOf(rows); !slices.Equal(got, []string{"shop | 2 | 3/5"}) {
		t.Fatalf("got %v", got)
	}
	if rows[0].Tone != ToneWarn {
		t.Fatal("a stack missing replicas should warn")
	}
	del := actionByKey(t, res, "ctrl-d")
	if !del.ConfirmName {
		t.Fatal("deleting a stack must ask for its name")
	}
	if err := del.Run(context.Background(), f, rows[0]); err != nil {
		t.Fatal(err)
	}
	if got := f.Log(); !slices.Equal(got, []string{"stack rm shop"}) {
		t.Fatalf("log: %v", got)
	}
}

func TestSecretsShowMetadataOnly(t *testing.T) {
	f := swarmFake()
	res := Secrets(fixedNow)
	rows := listRows(t, res, f)
	want := []string{"shop_token | shop | shop_web | 2d | 2d", "unused |  |  | 1h | 1h"}
	if got := cellsOf(rows); !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
	if len(res.Pages) != 0 {
		t.Fatal("a secret must not have a content page")
	}
	del := actionByKey(t, res, "ctrl-d")
	if w := del.Warn(rows[0]); !strings.Contains(w, "shop_web") {
		t.Fatalf("no in-use warning: %q", w)
	}
	if w := del.Warn(rows[1]); w != "" {
		t.Fatalf("warning for an unused secret: %q", w)
	}
}

func TestConfigsShowContent(t *testing.T) {
	f := swarmFake()
	res := Configs(fixedNow)
	rows := listRows(t, res, f)
	if got := cellsOf(rows); !slices.Equal(got, []string{"shop_motd | shop | shop_web | 2d"}) {
		t.Fatalf("got %v", got)
	}
	if len(res.Pages) != 1 || res.Pages[0].Key != "enter" {
		t.Fatalf("pages: %+v", res.Pages)
	}
	lines, err := res.Pages[0].Fetch(context.Background(), f, rows[0])
	if err != nil || !slices.Equal(lines, []string{"Hello", "second line"}) {
		t.Fatalf("content %q, err %v", lines, err)
	}
}

func TestRolloutPage(t *testing.T) {
	f := swarmFake()
	res := Services(fixedNow)
	rows := listRows(t, res, f)
	i := slices.IndexFunc(res.Pages, func(p TextPage) bool { return p.Key == "o" })
	if i < 0 || res.Pages[i].Refresh <= 0 {
		t.Fatalf("no live rollout page: %+v", res.Pages)
	}
	lines, err := res.Pages[i].Fetch(context.Background(), f, rows[0])
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lines, "\n")
	for _, want := range []string{"1/3", "paused", "update paused due to failure", "nginx:bad", "nginx:alpine",
		"NEW", "shop_web.2", "rejected", "No such image: nginx:bad", "OLD", "shop_web.1", "running"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if newAt, oldAt := strings.Index(text, "NEW"), strings.Index(text, "OLD"); newAt > oldAt {
		t.Fatalf("new tasks should come first:\n%s", text)
	}
}

func TestSwarmResourcesAreRegistered(t *testing.T) {
	reg, err := Default(fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	for cmd, want := range map[string]string{"svc": "services", "tasks": "tasks", "no": "nodes", "stk": "stacks", "sec": "secrets", "cfg": "configs", "service": "services", "node": "nodes"} {
		res, err := reg.Lookup(cmd)
		if err != nil || res.Name != want || !res.Swarm {
			t.Errorf("Lookup(%q) = %q swarm=%v, %v; want %q", cmd, res.Name, res.Swarm, err, want)
		}
	}
	// The older one-letter commands still resolve to what they did.
	for cmd, want := range map[string]string{"c": "containers", "n": "networks", "v": "volumes", "i": "images"} {
		if res, err := reg.Lookup(cmd); err != nil || res.Name != want {
			t.Errorf("Lookup(%q) = %q, %v; want %q", cmd, res.Name, err, want)
		}
	}
}
