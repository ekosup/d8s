package docker

import (
	"bytes"
	"strings"
	"testing"
)

func TestRelabelServiceLogs(t *testing.T) {
	names := func(taskID, nodeID string) string {
		task := map[string]string{"t1": "shop_web.1", "t2": "shop_web.2"}[taskID]
		node := map[string]string{"n1": "manager", "n2": "worker1"}[nodeID]
		if task == "" {
			task = taskID
		}
		if node == "" {
			node = nodeID
		}
		return task + "@" + node
	}
	in := strings.Join([]string{
		"2026-10-06T12:00:00.000000001Z com.docker.swarm.node.id=n1,com.docker.swarm.service.id=s1,com.docker.swarm.task.id=t1 GET / 200",
		"2026-10-06T12:00:01.000000001Z com.docker.swarm.node.id=n2,com.docker.swarm.service.id=s1,com.docker.swarm.task.id=t2 starting up",
		"2026-10-06T12:00:02.000000001Z com.docker.swarm.node.id=n9,com.docker.swarm.service.id=s1,com.docker.swarm.task.id=t9 ",
		"a line without the expected prefix",
		"",
	}, "\n")
	var out bytes.Buffer
	if err := relabel(&out, strings.NewReader(in), names, true); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"2026-10-06T12:00:00.000000001Z shop_web.1@manager | GET / 200",
		"2026-10-06T12:00:01.000000001Z shop_web.2@worker1 | starting up",
		"2026-10-06T12:00:02.000000001Z t9@n9 | ",
		"a line without the expected prefix",
		"",
	}, "\n")
	if out.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.String(), want)
	}

	out.Reset()
	if err := relabel(&out, strings.NewReader(in), names, false); err != nil {
		t.Fatal(err)
	}
	if first := strings.SplitN(out.String(), "\n", 2)[0]; first != "shop_web.1@manager | GET / 200" {
		t.Fatalf("without timestamps: %q", first)
	}
}

func TestTaskName(t *testing.T) {
	tests := []struct {
		service string
		slot    int
		nodeID  string
		want    string
	}{
		{"shop_web", 2, "abcdefghijklmnop", "shop_web.2"},
		{"mon_probe", 0, "abcdefghijklmnop", "mon_probe.abcdefghijkl"}, // global service: one task per node
		{"", 1, "n", "1"},
	}
	for _, tt := range tests {
		if got := TaskName(tt.service, tt.slot, tt.nodeID); got != tt.want {
			t.Errorf("TaskName(%q, %d, %q) = %q, want %q", tt.service, tt.slot, tt.nodeID, got, tt.want)
		}
	}
}
