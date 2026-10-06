package resource

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

// Networks lists networks with how many containers are attached.
func Networks(now func() time.Time) Resource {
	return Resource{
		Name:    "networks",
		Aliases: []string{"n", "network", "net"},
		Title:   "Networks",
		Columns: []Column{
			{Name: "NAME"}, {Name: "DRIVER"}, {Name: "SCOPE"}, {Name: "SUBNET"}, {Name: "CONTAINERS", Right: true},
		},
		EventTypes: []string{"network", "container"},
		List: func(ctx context.Context, c docker.Client) ([]Row, error) {
			networks, err := c.Networks(ctx)
			if err != nil {
				return nil, err
			}
			containers, err := c.Containers(ctx)
			if err != nil {
				return nil, err
			}
			attached := map[string]int{}
			for _, x := range containers {
				for _, a := range x.Networks {
					attached[a.NetworkID]++
				}
			}
			rows := make([]Row, 0, len(networks))
			for _, n := range networks {
				count := attached[n.ID]
				rows = append(rows, Row{
					ID:       n.ID,
					Cells:    []string{n.Name, n.Driver, n.Scope, strings.Join(n.Subnets, ", "), strconv.Itoa(count)},
					SortKeys: []string{"", "", "", "", fmt.Sprintf("%010d", count)},
					Attrs:    map[string]string{attrUsed: strconv.Itoa(count)},
				})
			}
			return rows, nil
		},
		Actions: []Action{{
			Key: "ctrl-d", Name: "Delete", Confirm: true, Mutates: true,
			Run: func(ctx context.Context, c docker.Client, row Row) error {
				return c.Remove(ctx, docker.KindNetwork, row.ID)
			},
			Warn: func(row Row) string {
				n, _ := strconv.Atoi(row.Attrs[attrUsed])
				if n == 0 {
					return ""
				}
				return fmt.Sprintf("%s still attached; the daemon will refuse while they are.", plural(n, "container"))
			},
		}},
		Inspect: inspectAs(docker.KindNetwork),
		Open: func(row Row) (Resource, bool) {
			id := row.ID
			address := func(x docker.Container) (string, bool) {
				for _, a := range x.Networks {
					if a.NetworkID == id {
						return a.IP, true
					}
				}
				return "", false
			}
			res := containersWhere(now, "Containers(network "+row.Name()+")", func(x docker.Container) bool {
				_, ok := address(x)
				return ok
			})
			return withColumn(res, 1, Column{Name: "IP"}, func(x docker.Container) string {
				ip, _ := address(x)
				return ip
			}), true
		},
	}
}
