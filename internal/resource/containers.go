package resource

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

// Containers is the container list. now is injected so ages are testable.
func Containers(now func() time.Time) Resource {
	res := containersWhere(now, "Containers", nil)
	res.Aliases = []string{"c", "container", "ps"}
	return res
}

// containersWhere is the container list narrowed to those keep accepts. It
// is what other resources drill down into.
func containersWhere(now func() time.Time, title string, keep func(docker.Container) bool) Resource {
	return Resource{
		Name:  "containers",
		Title: title,
		Columns: []Column{
			{Name: "NAME"}, {Name: "IMAGE"}, {Name: "STATE"}, {Name: "STATUS"}, {Name: "PORTS"}, {Name: "AGE"},
		},
		EventTypes: []string{"container"},
		List: func(ctx context.Context, c docker.Client) ([]Row, error) {
			cs, err := c.Containers(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([]Row, 0, len(cs))
			for _, x := range cs {
				if keep != nil && !keep(x) {
					continue
				}
				age := now().Sub(x.Created)
				rows = append(rows, Row{
					ID:       x.ID,
					Cells:    []string{x.Name, x.Image, x.State, x.Status, formatPorts(x.Ports), humanAge(age)},
					SortKeys: []string{"", "", "", "", "", fmt.Sprintf("%020d", max(int64(age/time.Second), 0))},
					Tone:     containerTone(x),
					Attrs:    map[string]string{attrState: x.State},
				})
			}
			return rows, nil
		},
		Actions: containerActions(),
		Inspect: inspectAs(docker.KindContainer),
		Logs: func(ctx context.Context, c docker.Client, row Row, opts docker.LogOptions) (io.ReadCloser, error) {
			return c.ContainerLogs(ctx, row.ID, opts)
		},
		Exec: func(ctx context.Context, c docker.Client, row Row, opts docker.ExecOptions) (docker.ExecSession, error) {
			return c.Exec(ctx, row.ID, opts)
		},
	}
}

const attrState = "state"

func containerActions() []Action {
	op := func(key, name string, confirm bool, pick func(Row) docker.ContainerOp) Action {
		return Action{Key: key, Name: name, Confirm: confirm, Mutates: true,
			Run: func(ctx context.Context, c docker.Client, row Row) error {
				return c.ContainerAction(ctx, row.ID, pick(row))
			}}
	}
	always := func(o docker.ContainerOp) func(Row) docker.ContainerOp {
		return func(Row) docker.ContainerOp { return o }
	}
	return []Action{
		op("a", "Start", false, always(docker.OpStart)),
		op("x", "Stop", false, always(docker.OpStop)),
		op("r", "Restart", false, always(docker.OpRestart)),
		op("p", "Pause/resume", false, func(row Row) docker.ContainerOp {
			if row.Attrs[attrState] == "paused" {
				return docker.OpUnpause
			}
			return docker.OpPause
		}),
		op("ctrl-k", "Kill", true, always(docker.OpKill)),
		op("ctrl-d", "Delete", true, always(docker.OpRemove)),
	}
}

func containerTone(c docker.Container) Tone {
	switch c.State {
	case "running":
		return ToneNormal
	case "paused", "restarting", "removing":
		return ToneWarn
	case "dead":
		return ToneBad
	case "exited":
		if strings.HasPrefix(c.Status, "Exited (0)") {
			return ToneMuted
		}
		return ToneBad
	default: // created and anything newer daemons add
		return ToneMuted
	}
}

// formatPorts renders ports like `docker ps`, minus the noise: the wildcard
// address is dropped and the IPv4/IPv6 duplicates collapse into one entry.
func formatPorts(ports []docker.Port) string {
	var out []string
	for _, p := range ports {
		s := fmt.Sprintf("%d/%s", p.Private, p.Proto)
		if p.Public != 0 {
			s = fmt.Sprintf("%d->%s", p.Public, s)
			if p.IP != "" && p.IP != "0.0.0.0" && p.IP != "::" {
				s = p.IP + ":" + s
			}
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return strings.Join(out, ", ")
}

// humanAge shortens a duration to its largest unit, like k9s does.
func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", max(int(d/time.Second), 0))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}

// inspectAs returns an Inspect function for rows whose ID is the object's ID.
func inspectAs(kind docker.Kind) func(context.Context, docker.Client, Row) ([]byte, error) {
	return func(ctx context.Context, c docker.Client, row Row) ([]byte, error) {
		return c.Inspect(ctx, kind, row.ID)
	}
}

// withColumn returns a container resource with one more column at index at,
// filled by value. Drill-downs use it to show what ties each container to
// the parent, such as its address on a network.
func withColumn(res Resource, at int, col Column, value func(docker.Container) string) Resource {
	return withColumns(res, at, []Column{col}, func(_ context.Context, _ docker.Client, cs []docker.Container) map[string]extraCells {
		out := make(map[string]extraCells, len(cs))
		for _, x := range cs {
			out[x.ID] = extraCells{cells: []string{value(x)}, keys: []string{""}}
		}
		return out
	})
}

// extraCells are the values of added columns for one row.
type extraCells struct{ cells, keys []string }

// withColumns inserts cols at index at. fill computes their values for all
// containers at once, so it can do its work concurrently; a container it
// leaves out gets "-".
func withColumns(res Resource, at int, cols []Column, fill func(context.Context, docker.Client, []docker.Container) map[string]extraCells) Resource {
	res.Columns = slices.Insert(slices.Clone(res.Columns), at, cols...)
	blank := extraCells{cells: make([]string, len(cols)), keys: make([]string, len(cols))}
	for i := range blank.cells {
		blank.cells[i] = "-"
	}

	list := res.List
	res.List = func(ctx context.Context, c docker.Client) ([]Row, error) {
		rows, err := list(ctx, c)
		if err != nil {
			return nil, err
		}
		cs, err := c.Containers(ctx)
		if err != nil {
			return nil, err
		}
		extra := fill(ctx, c, cs)
		for i, r := range rows {
			e, ok := extra[r.ID]
			if !ok {
				e = blank
			}
			rows[i].Cells = slices.Insert(slices.Clone(r.Cells), at, e.cells...)
			rows[i].SortKeys = slices.Insert(slices.Clone(r.SortKeys), at, e.keys...)
		}
		return rows, nil
	}
	return res
}
