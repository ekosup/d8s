package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ekosup/d8s/internal/resource"
)

// SortStore remembers how each view was last sorted.
type SortStore interface {
	Sort(view string) (column string, desc, ok bool)
	SetSort(view, column string, desc bool) error
}

// WithSortStore makes views open with the sort they were left in.
func WithSortStore(s SortStore) Option { return func(a *App) { a.sorts = s } }

// SetViews installs the user's column choices per view. A list that names
// a view or column that does not exist is left out and reported in the
// returned warnings, sorted. Call it after the registry is complete.
func (a *App) SetViews(views map[string][]string) []string {
	var warnings []string
	a.views = map[string][]string{}
	for name, columns := range views {
		if len(columns) == 0 {
			continue
		}
		if a.registry == nil || !a.registry.Has(name) {
			warnings = append(warnings, fmt.Sprintf("views: %q is not a view; ignored", name))
			continue
		}
		res, _ := a.registry.Lookup(name)
		if _, err := resource.Project(res, columns); err != nil {
			warnings = append(warnings, fmt.Sprintf("views: %v; ignored", err))
			continue
		}
		a.views[res.Name] = columns
	}
	slices.Sort(warnings)
	return warnings
}

// customised applies the user's column choice to res. Drill-down variants
// of a view may lack a chosen column; they are then shown in full.
func (a *App) customised(res resource.Resource) resource.Resource {
	if columns := a.views[res.Name]; len(columns) > 0 {
		if projected, err := resource.Project(res, columns); err == nil {
			return projected
		}
	}
	return res
}

// restoreSort gives view the sort remembered for res, if its column is
// still shown, and arranges for later changes to be remembered.
func (a *App) restoreSort(res resource.Resource, view *tableView) {
	if a.sorts == nil {
		return
	}
	if column, desc, ok := a.sorts.Sort(res.Name); ok {
		for i, c := range res.Columns {
			if strings.EqualFold(c.Name, column) {
				view.SetSort(i, desc)
				break
			}
		}
	}
	view.onSort = func(column string, desc bool) {
		if err := a.sorts.SetSort(res.Name, column, desc); err != nil {
			a.log.Warn("remember sort", "view", res.Name, "error", errText(err))
		}
	}
}
