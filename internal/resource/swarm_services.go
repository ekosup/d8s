package resource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

const (
	attrDesired  = "desired"
	attrImage    = "image"
	attrPrevious = "previous_image"
)

const rolloutRefresh = time.Second

// Services lists swarm services.
func Services(now func() time.Time) Resource {
	res := servicesWhere(now, "Services", nil)
	res.Aliases = []string{"svc", "service"}
	return res
}

// servicesWhere is the service list narrowed to those keep accepts.
func servicesWhere(now func() time.Time, title string, keep func(docker.Service) bool) Resource {
	return Resource{
		Name:  "services",
		Title: title,
		Swarm: true,
		Columns: []Column{
			{Name: "NAME"}, {Name: "STACK"}, {Name: "MODE"}, {Name: "REPLICAS", Right: true}, {Name: "IMAGE"}, {Name: "PORTS"}, {Name: "UPDATE"}, {Name: "AGE"},
		},
		EventTypes: []string{"service"},
		List: func(ctx context.Context, c docker.Client) ([]Row, error) {
			services, err := c.Services(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([]Row, 0, len(services))
			for _, s := range services {
				if keep != nil && !keep(s) {
					continue
				}
				rows = append(rows, serviceRow(s, now()))
			}
			return rows, nil
		},
		Actions: serviceActions(),
		Inspect: inspectAs(docker.KindService),
		Logs: func(ctx context.Context, c docker.Client, row Row, opts docker.LogOptions) (io.ReadCloser, error) {
			return c.ServiceLogs(ctx, row.ID, opts)
		},
		Open: func(row Row) (Resource, bool) {
			id := row.ID
			return tasksWhere(now, "Tasks(service "+row.Name()+")", func(t docker.Task) bool { return t.ServiceID == id }), true
		},
		Pages: []TextPage{{Key: "o", Name: "Rollout", Refresh: rolloutRefresh, Fetch: rolloutLines}},
	}
}

func serviceRow(s docker.Service, now time.Time) Row {
	age := now.Sub(s.Created)
	tone := ToneNormal
	switch {
	case s.UpdateState == "paused" || s.UpdateState == "rollback_paused":
		tone = ToneBad // a rollout stopped on a failure and needs a decision
	case s.Running < s.Desired:
		tone = ToneWarn
	case s.Desired == 0:
		tone = ToneMuted
	}
	return Row{
		ID: s.ID,
		Cells: []string{s.Name, s.Stack, s.Mode, fmt.Sprintf("%d/%d", s.Running, s.Desired), imageRef(s.Image),
			dash(formatPorts(s.Ports)), dash(s.UpdateState), humanAge(age)},
		SortKeys: []string{"", "", "", fmt.Sprintf("%020d", s.Running), "", "", "", fmt.Sprintf("%020d", max(int64(age/time.Second), 0))},
		Tone:     tone,
		Attrs: map[string]string{
			attrDesired:  strconv.FormatUint(s.Desired, 10),
			attrImage:    imageRef(s.Image),
			attrPrevious: imageRef(s.PreviousImage),
		},
	}
}

func serviceActions() []Action {
	return []Action{
		{
			Key: "s", Name: "Scale", Mutates: true,
			Input: &Input{Label: "Replicas", Default: func(row Row) string { return row.Attrs[attrDesired] }},
			RunInput: func(ctx context.Context, c docker.Client, row Row, value string) error {
				n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
				if err != nil {
					return errors.New("replicas must be a whole number, 0 or more")
				}
				return c.ServiceUpdate(ctx, row.ID, docker.ServiceChange{Replicas: &n})
			},
		},
		{
			Key: "r", Name: "Restart", Confirm: true, Mutates: true,
			Run: func(ctx context.Context, c docker.Client, row Row) error {
				return c.ServiceUpdate(ctx, row.ID, docker.ServiceChange{Force: true})
			},
			Warn: func(Row) string { return "Every task is replaced, following the service's update settings." },
		},
		{
			Key: "i", Name: "Image", Mutates: true,
			Input: &Input{Label: "Image", Default: func(row Row) string { return row.Attrs[attrImage] }},
			RunInput: func(ctx context.Context, c docker.Client, row Row, value string) error {
				value = strings.TrimSpace(value)
				if value == "" {
					return errors.New("the image cannot be empty")
				}
				return c.ServiceUpdate(ctx, row.ID, docker.ServiceChange{Image: value})
			},
		},
		{
			Key: "u", Name: "Rollback", Confirm: true, Mutates: true,
			Run: func(ctx context.Context, c docker.Client, row Row) error {
				return c.ServiceUpdate(ctx, row.ID, docker.ServiceChange{Rollback: true})
			},
			Warn: func(row Row) string {
				if prev := row.Attrs[attrPrevious]; prev != "" {
					return "It returns to its previous spec, with image " + prev + "."
				}
				return "It has no previous spec; the swarm will refuse."
			},
		},
		{
			Key: "ctrl-d", Name: "Delete", ConfirmName: true, Mutates: true,
			Run: func(ctx context.Context, c docker.Client, row Row) error {
				return c.Remove(ctx, docker.KindService, row.ID)
			},
		},
	}
}

// rolloutLines describes an update in progress: where the service stands,
// then the tasks on the new image and those still on an old one.
func rolloutLines(ctx context.Context, c docker.Client, row Row) ([]string, error) {
	services, err := c.Services(ctx)
	if err != nil {
		return nil, err
	}
	var svc *docker.Service
	for i := range services {
		if services[i].ID == row.ID {
			svc = &services[i]
		}
	}
	if svc == nil {
		return nil, errors.New("the service no longer exists")
	}
	tasks, err := c.Tasks(ctx)
	if err != nil {
		return nil, err
	}
	nodes, err := c.Nodes(ctx)
	if err != nil {
		return nil, err
	}
	hostname := make(map[string]string, len(nodes))
	for _, n := range nodes {
		hostname[n.ID] = n.Hostname
	}

	image := imageRef(svc.Image)
	lines := []string{
		fmt.Sprintf("%-9s %s", "SERVICE", svc.Name),
		fmt.Sprintf("%-9s %s", "IMAGE", image),
	}
	if svc.PreviousImage != "" {
		lines = append(lines, fmt.Sprintf("%-9s %s", "PREVIOUS", imageRef(svc.PreviousImage)))
	}
	lines = append(lines,
		fmt.Sprintf("%-9s %d/%d", "REPLICAS", svc.Running, svc.Desired),
		fmt.Sprintf("%-9s %s", "UPDATE", dash(svc.UpdateState)),
	)
	if svc.UpdateMessage != "" {
		lines = append(lines, fmt.Sprintf("%-9s %s", "MESSAGE", svc.UpdateMessage))
	}

	var fresh, old []string
	for _, t := range tasks {
		if t.ServiceID != svc.ID || !taskIsCurrent(t) {
			continue
		}
		line := fmt.Sprintf("  %-22s %-20s %-10s %s", docker.TaskName(svc.Name, t.Slot, t.NodeID), hostname[t.NodeID], t.State, t.Err)
		if imageRef(t.Image) == image {
			fresh = append(fresh, strings.TrimRight(line, " "))
		} else {
			old = append(old, strings.TrimRight(line, " "))
		}
	}
	lines = append(lines, "", fmt.Sprintf("NEW TASKS on %s (%d)", image, len(fresh)))
	lines = append(lines, fresh...)
	lines = append(lines, "", fmt.Sprintf("OLD TASKS still on another image (%d)", len(old)))
	lines = append(lines, old...)
	return lines, nil
}

// imageRef drops the digest the swarm pins an image to; the tag is what
// people recognise.
func imageRef(image string) string {
	ref, _, _ := strings.Cut(image, "@")
	return ref
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
