package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/ekosup/d8s/internal/docker"
)

// Project returns res showing only the named columns, in the order given.
// Names are matched without regard to case. An empty list changes nothing.
func Project(res Resource, names []string) (Resource, error) {
	if len(names) == 0 {
		return res, nil
	}
	index := make([]int, 0, len(names))
	seen := map[int]bool{}
	for _, name := range names {
		at := -1
		for i, c := range res.Columns {
			if strings.EqualFold(c.Name, strings.TrimSpace(name)) {
				at = i
				break
			}
		}
		if at < 0 {
			valid := make([]string, len(res.Columns))
			for i, c := range res.Columns {
				valid[i] = c.Name
			}
			return Resource{}, fmt.Errorf("view %s has no column %q; it has %s", res.Name, name, strings.Join(valid, ", "))
		}
		if seen[at] {
			return Resource{}, fmt.Errorf("view %s: column %s is listed twice", res.Name, res.Columns[at].Name)
		}
		seen[at] = true
		index = append(index, at)
	}

	out := res
	out.Columns = make([]Column, len(index))
	for i, at := range index {
		out.Columns[i] = res.Columns[at]
	}
	// Keep the opening sort if its column is still there.
	out.SortColumn, out.SortDesc = 0, false
	for i, at := range index {
		if at == res.SortColumn {
			out.SortColumn, out.SortDesc = i, res.SortDesc
		}
	}

	list := res.List
	out.List = func(ctx context.Context, c docker.Client) ([]Row, error) {
		rows, err := list(ctx, c)
		if err != nil {
			return nil, err
		}
		for r, row := range rows {
			name := row.Name()
			cells := make([]string, len(index))
			keys := make([]string, len(index))
			for i, at := range index {
				if at < len(row.Cells) {
					cells[i] = row.Cells[at]
				}
				if at < len(row.SortKeys) {
					keys[i] = row.SortKeys[at]
				}
			}
			// A row is named after its first cell. With that column moved
			// or hidden, pin the name so messages still say the right thing.
			attrs := make(map[string]string, len(row.Attrs)+1)
			for k, v := range row.Attrs {
				attrs[k] = v
			}
			attrs[attrName] = name
			rows[r].Cells, rows[r].SortKeys, rows[r].Attrs = cells, keys, attrs
		}
		return rows, nil
	}
	return out, nil
}
