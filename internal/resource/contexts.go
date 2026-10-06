package resource

import (
	"context"
	"sync"

	"github.com/ekosup/d8s/internal/docker"
)

// Contexts lists the Docker contexts on this machine. list reads them and
// active names the one in use; neither needs a daemon.
func Contexts(list func() ([]docker.Endpoint, error), active func() string) Resource {
	var (
		mu    sync.Mutex
		known = map[string]docker.Endpoint{}
	)
	return Resource{
		Name:    "contexts",
		Aliases: []string{"ctx", "context"},
		Title:   "Contexts",
		Columns: []Column{{Name: "NAME"}, {Name: "ENDPOINT"}, {Name: "ACTIVE"}},
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
				mark, tone := "", ToneNormal
				if ep.Context == active() {
					mark, tone = "*", ToneGood
				}
				rows = append(rows, Row{ID: ep.Context, Cells: []string{ep.Context, ep.Host, mark}, Tone: tone})
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
