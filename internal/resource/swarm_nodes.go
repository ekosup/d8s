package resource

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

const (
	attrAvailability = "availability"
	attrRole         = "role"
)

var availabilities = []string{"active", "pause", "drain"}

// Nodes lists the nodes of the swarm.
func Nodes(now func() time.Time) Resource {
	return Resource{
		Name:    "nodes",
		Aliases: []string{"no", "node"},
		Title:   "Nodes",
		Swarm:   true,
		Columns: []Column{
			{Name: "HOSTNAME"}, {Name: "STATUS"}, {Name: "AVAILABILITY"}, {Name: "ROLE"}, {Name: "MANAGER"}, {Name: "ENGINE"}, {Name: "ADDRESS"},
		},
		EventTypes: []string{"node"},
		List: func(ctx context.Context, c docker.Client) ([]Row, error) {
			nodes, err := c.Nodes(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([]Row, 0, len(nodes))
			for _, n := range nodes {
				manager := n.Reachability
				if n.Leader {
					manager = "leader"
				}
				tone := ToneNormal
				switch {
				case n.State != "ready":
					tone = ToneBad // gone or unreachable
				case n.Availability != "active":
					tone = ToneWarn // healthy but deliberately not taking work
				}
				rows = append(rows, Row{
					ID:    n.ID,
					Cells: []string{n.Hostname, n.State, n.Availability, n.Role, manager, n.EngineVersion, n.Addr},
					Tone:  tone,
					Attrs: map[string]string{attrAvailability: n.Availability, attrRole: n.Role},
				})
			}
			return rows, nil
		},
		Actions: []Action{
			{
				Key: "a", Name: "Availability", Mutates: true,
				Input: &Input{Label: "Availability (" + strings.Join(availabilities, ", ") + ")", Default: func(row Row) string { return row.Attrs[attrAvailability] }},
				RunInput: func(ctx context.Context, c docker.Client, row Row, value string) error {
					value = strings.ToLower(strings.TrimSpace(value))
					if !slices.Contains(availabilities, value) {
						return fmt.Errorf("availability must be one of %s", strings.Join(availabilities, ", "))
					}
					return c.NodeUpdate(ctx, row.ID, docker.NodeChange{Availability: value})
				},
			},
			{
				Key: "p", Name: "Promote/demote", Confirm: true, Mutates: true,
				Run: func(ctx context.Context, c docker.Client, row Row) error {
					role := "manager"
					if row.Attrs[attrRole] == "manager" {
						role = "worker"
					}
					return c.NodeUpdate(ctx, row.ID, docker.NodeChange{Role: role})
				},
				Warn: func(row Row) string {
					if row.Attrs[attrRole] == "manager" {
						return "It is a manager and will become a worker. The swarm refuses if it is the last one."
					}
					return "It is a worker and will become a manager."
				},
			},
			{
				Key: "b", Name: "Label", Mutates: true,
				Input: &Input{Label: "Label (key=value to set, key- to remove)"},
				RunInput: func(ctx context.Context, c docker.Client, row Row, value string) error {
					change, err := parseLabelChange(value)
					if err != nil {
						return err
					}
					return c.NodeUpdate(ctx, row.ID, change)
				},
			},
		},
		Inspect: inspectAs(docker.KindNode),
		Open: func(row Row) (Resource, bool) {
			id := row.ID
			return tasksWhere(now, "Tasks(node "+row.Name()+")", func(t docker.Task) bool { return t.NodeID == id }), true
		},
	}
}

// parseLabelChange reads "key=value" (set) or "key-" (remove).
func parseLabelChange(value string) (docker.NodeChange, error) {
	value = strings.TrimSpace(value)
	if key, ok := strings.CutSuffix(value, "-"); ok && key != "" && !strings.Contains(key, "=") {
		return docker.NodeChange{RemoveLabels: []string{key}}, nil
	}
	key, val, ok := strings.Cut(value, "=")
	if !ok || key == "" {
		return docker.NodeChange{}, errors.New("write key=value to set a label, or key- to remove one")
	}
	return docker.NodeChange{SetLabels: map[string]string{key: val}}, nil
}
