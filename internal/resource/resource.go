// Package resource declares what d8s can show: one Resource per kind of
// Docker object, with its columns and how to list it. Views are generic and
// render whatever is registered here.
package resource

import (
	"context"
	"io"
	"time"

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

// attrName, when present in Attrs, overrides the row's display name.
const attrName = "name"

// Name is what the row is called in messages: its name attribute if it has
// one, otherwise its first cell.
func (r Row) Name() string {
	if n := r.Attrs[attrName]; n != "" {
		return n
	}
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
	// Warn, when set, returns a caution to show in the confirmation for
	// this particular row, or "" when there is nothing to warn about.
	Warn func(row Row) string
	// Input makes the action ask for a value first; RunInput then runs
	// instead of Run and receives what was typed.
	Input    *Input
	RunInput func(ctx context.Context, c docker.Client, row Row, value string) error
	// ConfirmName asks the user to type the row's name before running. It
	// is for deletions that take a whole workload down.
	ConfirmName bool
	// Quiet suppresses the status message; for toggles whose effect is
	// visible in the table itself.
	Quiet bool
	// Target makes the action global: it applies to Target (for example
	// "dangling images") instead of the selected row, and Run gets an
	// empty Row.
	Target string
}

// Input describes the value an action asks for.
type Input struct {
	Label   string               // shown before the field
	Default func(row Row) string // pre-filled value; may be nil
}

// TextPage is a read-only text view about one row, opened with a key.
type TextPage struct {
	Key   string
	Name  string // "History"
	Fetch func(ctx context.Context, c docker.Client, row Row) ([]string, error)
	// Refresh, when positive, makes the page live: Fetch runs again at this
	// interval for as long as the page is open.
	Refresh time.Duration
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
	// Note, when set, returns a short remark for the table title about how
	// the list is currently narrowed, such as "active only", or "".
	Note func() string
	// Swarm marks a resource that only a swarm manager can list.
	Swarm bool
	// SortColumn and SortDesc are the order a view opens with. The zero
	// values mean the first column, ascending.
	SortColumn int
	SortDesc   bool
	List       func(ctx context.Context, c docker.Client) ([]Row, error)
	Actions    []Action
	// Inspect returns the object's full description as JSON. Nil means the
	// resource has nothing more to show than its row.
	Inspect func(ctx context.Context, c docker.Client, row Row) ([]byte, error)
	// Logs opens the row's log stream as plain text. Nil means it has none.
	Logs func(ctx context.Context, c docker.Client, row Row, opts docker.LogOptions) (io.ReadCloser, error)
	// Open returns the view Enter drills down into for a row.
	Open func(row Row) (Resource, bool)
	// Connect returns the daemon Enter switches the whole application to.
	// Only the context list sets it.
	Connect func(row Row) (docker.Endpoint, bool)
	// Pages are extra text views about a row.
	Pages []TextPage
	// Exec starts an interactive command in the row's container. Nil means
	// the resource has no shell.
	Exec func(ctx context.Context, c docker.Client, row Row, opts docker.ExecOptions) (docker.ExecSession, error)
}
