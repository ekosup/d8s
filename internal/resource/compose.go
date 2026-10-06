package resource

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

// Labels Docker Compose puts on the containers it creates. A project is not
// an API object; it exists only as this label.
const (
	labelComposeProject = "com.docker.compose.project"
	labelComposeWorkdir = "com.docker.compose.project.working_dir"
)

// Compose lists Compose projects, derived from container labels.
func Compose(now func() time.Time) Resource {
	inProject := func(project string) func(docker.Container) bool {
		return func(x docker.Container) bool { return x.Labels[labelComposeProject] == project }
	}
	// all applies op to every container of the row's project.
	all := func(key, name string, op docker.ContainerOp) Action {
		return Action{Key: key, Name: name, Mutates: true,
			Run: func(ctx context.Context, c docker.Client, row Row) error {
				cs, err := c.Containers(ctx)
				if err != nil {
					return err
				}
				var errs []error
				for _, x := range cs {
					if !inProject(row.ID)(x) {
						continue
					}
					if err := c.ContainerAction(ctx, x.ID, op); err != nil {
						errs = append(errs, fmt.Errorf("%s: %w", x.Name, err))
					}
				}
				return errors.Join(errs...)
			}}
	}
	return Resource{
		Name:       "compose",
		Aliases:    []string{"cp", "project", "projects"},
		Title:      "Compose projects",
		Columns:    []Column{{Name: "PROJECT"}, {Name: "CONTAINERS", Right: true}, {Name: "RUNNING", Right: true}, {Name: "WORKDIR"}},
		EventTypes: []string{"container"},
		List: func(ctx context.Context, c docker.Client) ([]Row, error) {
			cs, err := c.Containers(ctx)
			if err != nil {
				return nil, err
			}
			type project struct {
				total, running int
				workdir        string
			}
			projects := map[string]*project{}
			for _, x := range cs {
				name := x.Labels[labelComposeProject]
				if name == "" {
					continue
				}
				p := projects[name]
				if p == nil {
					p = &project{workdir: x.Labels[labelComposeWorkdir]}
					projects[name] = p
				}
				p.total++
				if x.State == "running" {
					p.running++
				}
			}
			names := make([]string, 0, len(projects))
			for n := range projects {
				names = append(names, n)
			}
			sort.Strings(names)
			rows := make([]Row, 0, len(names))
			for _, n := range names {
				p := projects[n]
				tone := ToneNormal
				switch {
				case p.running == 0:
					tone = ToneMuted
				case p.running < p.total:
					tone = ToneWarn
				}
				rows = append(rows, Row{
					ID:       n,
					Cells:    []string{n, strconv.Itoa(p.total), fmt.Sprintf("%d/%d", p.running, p.total), p.workdir},
					SortKeys: []string{"", fmt.Sprintf("%010d", p.total), fmt.Sprintf("%010d", p.running), ""},
					Tone:     tone,
				})
			}
			return rows, nil
		},
		Actions: []Action{
			all("a", "Start all", docker.OpStart),
			all("x", "Stop all", docker.OpStop),
			all("r", "Restart all", docker.OpRestart),
		},
		Open: func(row Row) (Resource, bool) {
			return containersWhere(now, "Containers(project "+row.ID+")", inProject(row.ID)), true
		},
	}
}
