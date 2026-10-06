package resource

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

// Containers is the container list. now is injected so ages are testable.
func Containers(now func() time.Time) Resource {
	return Resource{
		Name:    "containers",
		Aliases: []string{"c", "container", "ps"},
		Title:   "Containers",
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
				age := now().Sub(x.Created)
				rows = append(rows, Row{
					ID:       x.ID,
					Cells:    []string{x.Name, x.Image, x.State, x.Status, formatPorts(x.Ports), humanAge(age)},
					SortKeys: []string{"", "", "", "", "", fmt.Sprintf("%020d", max(int64(age/time.Second), 0))},
					Tone:     containerTone(x),
				})
			}
			return rows, nil
		},
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
