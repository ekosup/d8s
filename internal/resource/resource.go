// Package resource declares what d8s can show: one Resource per kind of
// Docker object, with its columns and how to list it. Views are generic and
// render whatever is registered here.
package resource

import (
	"context"

	"github.com/ekosup/d8s/internal/docker"
)

// Tone is the semantic colouring of a row; the UI maps it to a colour.
type Tone int

// Row tones, from neutral to alarming.
const (
	ToneNormal Tone = iota
	ToneGood
	ToneWarn
	ToneBad
	ToneMuted
)

// Column describes one table column.
type Column struct {
	Name  string
	Right bool // right-align, for numbers
}

// Row is one object. Cells line up with the resource's Columns.
type Row struct {
	ID    string
	Cells []string
	// SortKeys, when set, replaces Cells for ordering (e.g. a timestamp
	// behind a human-readable age). Empty entries fall back to the cell.
	SortKeys []string
	Tone     Tone
	// Attrs carries facts actions and drill-downs need but no column shows.
	Attrs map[string]string
}

// Name is what the row is called in messages: its first cell.
func (r Row) Name() string {
	if len(r.Cells) > 0 && r.Cells[0] != "" {
		return r.Cells[0]
	}
	return r.ID
}

// Action is something the user can do to a row.
type Action struct {
	// Key is the key that triggers it: a single character such as "r", or
	// "ctrl-" plus a letter such as "ctrl-d".
	Key  string
	Name string // imperative, shown in hints and messages: "Restart"
	// Confirm asks before running; set it for anything that destroys data
	// or interrupts a workload abruptly.
	Confirm bool
	// Mutates marks actions that change state, which read-only mode refuses.
	Mutates bool
	Run     func(ctx context.Context, c docker.Client, row Row) error
}

// Resource is the declarative definition of one kind of object.
type Resource struct {
	Name    string   // canonical command, e.g. "containers"
	Aliases []string // other commands that open the same view
	Title   string   // shown in the table border
	Columns []Column
	// EventTypes lists the daemon event types that should trigger a refresh.
	// Empty means the resource can only be polled.
	EventTypes []string
	List       func(ctx context.Context, c docker.Client) ([]Row, error)
	Actions    []Action
	// Inspect returns the object's full description as JSON. Nil means the
	// resource has nothing more to show than its row.
	Inspect func(ctx context.Context, c docker.Client, row Row) ([]byte, error)
}
