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
}
