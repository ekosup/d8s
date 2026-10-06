package resource

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ekosup/d8s/internal/docker"
)

const (
	attrKind        = "kind"
	attrReclaimable = "reclaimable"
)

// diskKinds says, per kind of object, how its row is labelled and what a
// prune of it removes.
var diskKinds = map[docker.Kind]struct{ label, pruned string }{
	docker.KindImage:      {"Images", "dangling images"},
	docker.KindContainer:  {"Containers", "stopped containers"},
	docker.KindVolume:     {"Volumes", "unused anonymous volumes"},
	docker.KindBuildCache: {"Build cache", "build cache"},
}

// DiskUsage shows what takes space on the daemon's disk, like `docker
// system df`, and lets the user prune one kind at a time.
func DiskUsage() Resource {
	return Resource{
		Name:    "df",
		Aliases: []string{"du", "disk", "usage"},
		Title:   "Disk usage",
		Columns: []Column{
			{Name: "TYPE"}, {Name: "TOTAL", Right: true}, {Name: "ACTIVE", Right: true}, {Name: "SIZE", Right: true}, {Name: "RECLAIMABLE", Right: true},
		},
		EventTypes: []string{"image", "container", "volume"},
		List: func(ctx context.Context, c docker.Client) ([]Row, error) {
			usage, err := c.DiskUsage(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([]Row, 0, len(usage))
			for _, u := range usage {
				k := diskKinds[u.Kind]
				pct := 0
				if u.Size > 0 {
					pct = int(u.Reclaimable * 100 / u.Size)
				}
				rows = append(rows, Row{
					ID: string(u.Kind),
					Cells: []string{k.label, strconv.FormatInt(u.Total, 10), strconv.FormatInt(u.Active, 10),
						humanBytes(u.Size), fmt.Sprintf("%s (%d%%)", humanBytes(u.Reclaimable), pct)},
					SortKeys: []string{"", fmt.Sprintf("%020d", u.Total), fmt.Sprintf("%020d", u.Active),
						fmt.Sprintf("%020d", u.Size), fmt.Sprintf("%020d", u.Reclaimable)},
					Attrs: map[string]string{attrKind: string(u.Kind), attrName: k.pruned, attrReclaimable: humanBytes(u.Reclaimable)},
				})
			}
			return rows, nil
		},
		Actions: []Action{{
			Key: "ctrl-p", Name: "Prune", Confirm: true, Mutates: true,
			Run: func(ctx context.Context, c docker.Client, row Row) error {
				_, err := c.Prune(ctx, docker.Kind(row.Attrs[attrKind]))
				return err
			},
			Warn: func(row Row) string {
				w := "Up to " + row.Attrs[attrReclaimable] + " can be reclaimed."
				if docker.Kind(row.Attrs[attrKind]) == docker.KindImage {
					w += " Only dangling images are removed; unused tagged images stay."
				}
				return w
			},
		}},
	}
}
