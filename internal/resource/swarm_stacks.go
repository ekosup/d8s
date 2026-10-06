package resource

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

// Stacks lists stacks, derived from the stack label on services.
func Stacks(now func() time.Time) Resource {
	return Resource{
		Name:       "stacks",
		Aliases:    []string{"stk", "stack"},
		Title:      "Stacks",
		Swarm:      true,
		Columns:    []Column{{Name: "NAME"}, {Name: "SERVICES", Right: true}, {Name: "REPLICAS", Right: true}},
		EventTypes: []string{"service"},
		List: func(ctx context.Context, c docker.Client) ([]Row, error) {
			services, err := c.Services(ctx)
			if err != nil {
				return nil, err
			}
			type stack struct{ services, running, desired uint64 }
			stacks := map[string]*stack{}
			for _, s := range services {
				if s.Stack == "" {
					continue
				}
				st := stacks[s.Stack]
				if st == nil {
					st = &stack{}
					stacks[s.Stack] = st
				}
				st.services++
				st.running += s.Running
				st.desired += s.Desired
			}
			names := make([]string, 0, len(stacks))
			for n := range stacks {
				names = append(names, n)
			}
			sort.Strings(names)
			rows := make([]Row, 0, len(names))
			for _, n := range names {
				st := stacks[n]
				tone := ToneNormal
				if st.running < st.desired {
					tone = ToneWarn
				}
				rows = append(rows, Row{
					ID:       n,
					Cells:    []string{n, strconv.FormatUint(st.services, 10), fmt.Sprintf("%d/%d", st.running, st.desired)},
					SortKeys: []string{"", fmt.Sprintf("%020d", st.services), fmt.Sprintf("%020d", st.running)},
					Tone:     tone,
				})
			}
			return rows, nil
		},
		Actions: []Action{{
			Key: "ctrl-d", Name: "Delete", ConfirmName: true, Mutates: true,
			Run: func(ctx context.Context, c docker.Client, row Row) error {
				return c.StackRemove(ctx, row.ID)
			},
			Warn: func(Row) string { return "Its services, networks, secrets and configs are all removed." },
		}},
		Open: func(row Row) (Resource, bool) {
			name := row.ID
			return servicesWhere(now, "Services(stack "+name+")", func(s docker.Service) bool { return s.Stack == name }), true
		},
	}
}

// usedBy maps each name to the services that reference it, sorted.
func usedBy(services []docker.Service, refs func(docker.Service) []string) map[string]string {
	users := map[string][]string{}
	for _, s := range services {
		for _, name := range refs(s) {
			users[name] = append(users[name], s.Name)
		}
	}
	out := make(map[string]string, len(users))
	for name, by := range users {
		slices.Sort(by)
		out[name] = strings.Join(by, ", ")
	}
	return out
}

func inUseWarning(row Row) string {
	if by := row.Attrs[attrUsers]; by != "" {
		return "It is used by " + by + "; the swarm will refuse while it is."
	}
	return ""
}

func ageCell(now, t time.Time) (cell, key string) {
	d := now.Sub(t)
	return humanAge(d), fmt.Sprintf("%020d", max(int64(d/time.Second), 0))
}

// Secrets lists swarm secrets. Only metadata: the API never returns a
// secret's value, and d8s never asks for it.
func Secrets(now func() time.Time) Resource {
	return Resource{
		Name:       "secrets",
		Aliases:    []string{"sec", "secret"},
		Title:      "Secrets",
		Swarm:      true,
		Columns:    []Column{{Name: "NAME"}, {Name: "STACK"}, {Name: "USED BY"}, {Name: "CREATED"}, {Name: "UPDATED"}},
		EventTypes: []string{"secret", "service"},
		List: func(ctx context.Context, c docker.Client) ([]Row, error) {
			secrets, err := c.Secrets(ctx)
			if err != nil {
				return nil, err
			}
			services, err := c.Services(ctx)
			if err != nil {
				return nil, err
			}
			users := usedBy(services, func(s docker.Service) []string { return s.Secrets })
			rows := make([]Row, 0, len(secrets))
			for _, s := range secrets {
				created, createdKey := ageCell(now(), s.Created)
				updated, updatedKey := ageCell(now(), s.Updated)
				rows = append(rows, Row{
					ID:       s.ID,
					Cells:    []string{s.Name, s.Stack, users[s.Name], created, updated},
					SortKeys: []string{"", "", "", createdKey, updatedKey},
					Attrs:    map[string]string{attrUsers: users[s.Name]},
				})
			}
			return rows, nil
		},
		Actions: []Action{{
			Key: "ctrl-d", Name: "Delete", Confirm: true, Mutates: true,
			Run: func(ctx context.Context, c docker.Client, row Row) error {
				return c.Remove(ctx, docker.KindSecret, row.ID)
			},
			Warn: inUseWarning,
		}},
		Inspect: inspectAs(docker.KindSecret),
	}
}

// Configs lists swarm configs. Unlike a secret, a config's content can be read.
func Configs(now func() time.Time) Resource {
	return Resource{
		Name:       "configs",
		Aliases:    []string{"cfg", "config"},
		Title:      "Configs",
		Swarm:      true,
		Columns:    []Column{{Name: "NAME"}, {Name: "STACK"}, {Name: "USED BY"}, {Name: "AGE"}},
		EventTypes: []string{"config", "service"},
		List: func(ctx context.Context, c docker.Client) ([]Row, error) {
			configs, err := c.Configs(ctx)
			if err != nil {
				return nil, err
			}
			services, err := c.Services(ctx)
			if err != nil {
				return nil, err
			}
			users := usedBy(services, func(s docker.Service) []string { return s.Configs })
			rows := make([]Row, 0, len(configs))
			for _, cfg := range configs {
				age, ageKey := ageCell(now(), cfg.Created)
				rows = append(rows, Row{
					ID:       cfg.ID,
					Cells:    []string{cfg.Name, cfg.Stack, users[cfg.Name], age},
					SortKeys: []string{"", "", "", ageKey},
					Attrs:    map[string]string{attrUsers: users[cfg.Name]},
				})
			}
			return rows, nil
		},
		Actions: []Action{{
			Key: "ctrl-d", Name: "Delete", Confirm: true, Mutates: true,
			Run: func(ctx context.Context, c docker.Client, row Row) error {
				return c.Remove(ctx, docker.KindConfig, row.ID)
			},
			Warn: inUseWarning,
		}},
		Inspect: inspectAs(docker.KindConfig),
		Pages: []TextPage{{
			Key: "enter", Name: "Content",
			Fetch: func(ctx context.Context, c docker.Client, row Row) ([]string, error) {
				data, err := c.ConfigData(ctx, row.ID)
				if err != nil {
					return nil, err
				}
				return strings.Split(strings.TrimRight(string(data), "\n"), "\n"), nil
			},
		}},
	}
}
