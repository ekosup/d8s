package resource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

const (
	attrNodeID    = "node_id"
	attrNode      = "node"
	attrContainer = "container_id"
)

// Tasks lists the tasks of every service.
func Tasks(now func() time.Time) Resource {
	res := tasksWhere(now, "Tasks", nil)
	res.Aliases = []string{"task", "ts"}
	return res
}

// taskIsCurrent reports whether a task still matters: it should be
// running, or it failed. Tasks that were shut down cleanly are history.
func taskIsCurrent(t docker.Task) bool {
	return t.DesiredState == "running" || t.State == "failed" || t.State == "rejected"
}

// tasksWhere is the task list narrowed to those keep accepts.
func tasksWhere(now func() time.Time, title string, keep func(docker.Task) bool) Resource {
	var hideHistory atomic.Bool
	return Resource{
		Name:  "tasks",
		Title: title,
		Swarm: true,
		Columns: []Column{
			{Name: "NAME"}, {Name: "NODE"}, {Name: "DESIRED"}, {Name: "STATE"}, {Name: "ERROR"}, {Name: "IMAGE"}, {Name: "AGE"},
		},
		// Tasks emit no events of their own; service and node events are
		// the closest signal, and polling covers the rest.
		EventTypes: []string{"service", "node"},
		List: func(ctx context.Context, c docker.Client) ([]Row, error) {
			tasks, err := c.Tasks(ctx)
			if err != nil {
				return nil, err
			}
			services, err := c.Services(ctx)
			if err != nil {
				return nil, err
			}
			nodes, err := c.Nodes(ctx)
			if err != nil {
				return nil, err
			}
			serviceName := make(map[string]string, len(services))
			for _, s := range services {
				serviceName[s.ID] = s.Name
			}
			hostname := make(map[string]string, len(nodes))
			for _, n := range nodes {
				hostname[n.ID] = n.Hostname
			}
			rows := make([]Row, 0, len(tasks))
			for _, t := range tasks {
				if keep != nil && !keep(t) {
					continue
				}
				if hideHistory.Load() && !taskIsCurrent(t) {
					continue
				}
				rows = append(rows, taskRow(t, serviceName[t.ServiceID], hostname[t.NodeID], now()))
			}
			return rows, nil
		},
		Actions: []Action{{
			Key: "h", Name: "History", Target: "task history", Quiet: true,
			Run: func(context.Context, docker.Client, Row) error {
				hideHistory.Store(!hideHistory.Load())
				return nil
			},
		}},
		Inspect: inspectAs(docker.KindTask),
		Logs: func(ctx context.Context, c docker.Client, row Row, opts docker.LogOptions) (io.ReadCloser, error) {
			return c.TaskLogs(ctx, row.ID, opts)
		},
		Exec: func(ctx context.Context, c docker.Client, row Row, opts docker.ExecOptions) (docker.ExecSession, error) {
			container := row.Attrs[attrContainer]
			if container == "" {
				return nil, errors.New("the task has no container")
			}
			info, err := c.Info(ctx)
			if err != nil {
				return nil, err
			}
			// Exec is served by the daemon that runs the container; a
			// manager cannot relay it to another node.
			if info.Swarm.NodeID != row.Attrs[attrNodeID] {
				return nil, &docker.ErrOtherNode{Node: row.Attrs[attrNode]}
			}
			return c.Exec(ctx, container, opts)
		},
	}
}

func taskRow(t docker.Task, service, node string, now time.Time) Row {
	since := t.Timestamp
	if since.IsZero() {
		since = t.Created
	}
	age := now.Sub(since)
	tone := ToneWarn // pending, preparing, starting: on its way
	switch t.State {
	case "running":
		tone = ToneNormal
	case "failed", "rejected":
		tone = ToneBad
	case "shutdown", "complete", "remove", "orphaned":
		tone = ToneMuted
	}
	name := docker.TaskName(service, t.Slot, t.NodeID)
	return Row{
		ID:       t.ID,
		Cells:    []string{name, node, t.DesiredState, t.State, t.Err, imageRef(t.Image), humanAge(age)},
		SortKeys: []string{"", "", "", "", "", "", fmt.Sprintf("%020d", max(int64(age/time.Second), 0))},
		Tone:     tone,
		Attrs:    map[string]string{attrNodeID: t.NodeID, attrNode: node, attrContainer: t.ContainerID},
	}
}
