package resource

import (
	"context"
	"sync"

	"github.com/ekosup/d8s/internal/docker"
)

// ContextPolicy is what the user's settings say about one Docker context.
type ContextPolicy struct {
	ReadOnly   bool
	Production bool
}

// ContextsOption adjusts the context list.
type ContextsOption func(*contextsOptions)

type contextsOptions struct {
	policy func(name string) ContextPolicy
}

// WithContextPolicy adds the MODE and PROD columns, filled from policy, so
// that a context left changeable by mistake shows before it is opened.
func WithContextPolicy(policy func(name string) ContextPolicy) ContextsOption {
	return func(o *contextsOptions) { o.policy = policy }
}

// Contexts lists the Docker contexts on this machine. list reads them and
// active names the one in use; neither needs a daemon.
func Contexts(list func() ([]docker.Endpoint, error), active func() string, opts ...ContextsOption) Resource {
	var (
		mu    sync.Mutex
		known = map[string]docker.Endpoint{}
		o     contextsOptions
	)
	for _, opt := range opts {
		opt(&o)
	}
	columns := []Column{{Name: "NAME"}, {Name: "ENDPOINT"}}
	if o.policy != nil {
		columns = append(columns, Column{Name: "MODE"}, Column{Name: "PROD"})
	}
	columns = append(columns, Column{Name: "ACTIVE"})
	return Resource{
		Name:    "contexts",
		Aliases: []string{"ctx", "context"},
		Title:   "Contexts",
		Columns: columns,
		List: func(context.Context, docker.Client) ([]Row, error) {
			eps, err := list()
			if err != nil {
				return nil, err
			}
			mu.Lock()
			defer mu.Unlock()
			clear(known)
			rows := make([]Row, 0, len(eps))
			for _, ep := range eps {
				known[ep.Context] = ep
				cells := []string{ep.Context, ep.Host}
				mark, tone := "", ToneNormal
				if ep.Context == active() {
					mark, tone = "*", ToneGood
				}
				if o.policy != nil {
					p := o.policy(ep.Context)
					mode, prod := "read-write", ""
					if p.ReadOnly {
						mode = "read-only"
					}
					if p.Production {
						prod = "yes"
					}
					if p.Production && !p.ReadOnly {
						tone = ToneWarn
					}
					cells = append(cells, mode, prod)
				}
				rows = append(rows, Row{ID: ep.Context, Cells: append(cells, mark), Tone: tone})
			}
			return rows, nil
		},
		Connect: func(row Row) (docker.Endpoint, bool) {
			mu.Lock()
			defer mu.Unlock()
			ep, ok := known[row.ID]
			return ep, ok
		},
	}
}
