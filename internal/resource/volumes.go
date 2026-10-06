package resource

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

const attrUsers = "users" // names of the containers using the object

// Volumes lists named volumes with the containers that mount them.
func Volumes(now func() time.Time) Resource {
	return Resource{
		Name:       "volumes",
		Aliases:    []string{"v", "volume", "vol"},
		Title:      "Volumes",
		Columns:    []Column{{Name: "NAME"}, {Name: "DRIVER"}, {Name: "USED BY"}, {Name: "AGE"}},
		EventTypes: []string{"volume", "container"},
		List: func(ctx context.Context, c docker.Client) ([]Row, error) {
			volumes, err := c.Volumes(ctx)
			if err != nil {
				return nil, err
			}
			containers, err := c.Containers(ctx)
			if err != nil {
				return nil, err
			}
			users := map[string][]string{}
			for _, x := range containers {
				for _, v := range x.Volumes {
					users[v] = append(users[v], x.Name)
				}
			}
			rows := make([]Row, 0, len(volumes))
			for _, v := range volumes {
				by := users[v.Name]
				slices.Sort(by)
				tone := ToneNormal
				if len(by) == 0 {
					tone = ToneMuted
				}
				age, ageKey := "", ""
				if !v.Created.IsZero() {
					d := now().Sub(v.Created)
					age, ageKey = humanAge(d), fmt.Sprintf("%020d", max(int64(d/time.Second), 0))
				}
				rows = append(rows, Row{
					ID:       v.Name,
					Cells:    []string{v.Name, v.Driver, strings.Join(by, ", "), age},
					SortKeys: []string{"", "", "", ageKey},
					Tone:     tone,
					Attrs:    map[string]string{attrUsers: strings.Join(by, ", ")},
				})
			}
			return rows, nil
		},
		Actions: []Action{
			{
				Key: "ctrl-d", Name: "Delete", Confirm: true, Mutates: true,
				Run: func(ctx context.Context, c docker.Client, row Row) error {
					return c.Remove(ctx, docker.KindVolume, row.ID)
				},
				Warn: func(row Row) string {
					if by := row.Attrs[attrUsers]; by != "" {
						return "It is mounted by " + by + "; the daemon will refuse while they exist."
					}
					return ""
				},
			},
			{
				Key: "ctrl-p", Name: "Prune", Target: "unused anonymous volumes", Confirm: true, Mutates: true,
				Run: func(ctx context.Context, c docker.Client, _ Row) error {
					_, err := c.Prune(ctx, docker.KindVolume)
					return err
				},
			},
		},
		Inspect: inspectAs(docker.KindVolume),
		Open: func(row Row) (Resource, bool) {
			name := row.ID
			return containersWhere(now, "Containers(volume "+name+")", func(x docker.Container) bool {
				return slices.Contains(x.Volumes, name)
			}), true
		},
	}
}
