// Package action runs what the user asks to do to rows. Every state change
// goes through the Executor, which is where read-only mode is enforced.
package action

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
)

// Refusals that are not failures of the action itself.
var (
	ErrReadOnly = errors.New("read-only mode")
	ErrNoTarget = errors.New("nothing selected")
)

// perRowTimeout bounds one action on one row, so a hung daemon call cannot
// block the rest of a bulk action forever.
const perRowTimeout = 60 * time.Second

// Executor applies actions through a Docker client.
type Executor struct {
	client   docker.Client
	readOnly bool
}

// New returns an Executor. With readOnly set it refuses mutating actions.
func New(client docker.Client, readOnly bool) *Executor {
	return &Executor{client: client, readOnly: readOnly}
}

// Run applies act to every row. It keeps going after a failure and returns
// all failures joined, each prefixed with the row's name.
func (e *Executor) Run(ctx context.Context, act resource.Action, rows []resource.Row) error {
	if act.Mutates && e.readOnly {
		return ErrReadOnly
	}
	if len(rows) == 0 {
		return ErrNoTarget
	}
	var errs []error
	for _, row := range rows {
		rowCtx, cancel := context.WithTimeout(ctx, perRowTimeout)
		err := act.Run(rowCtx, e.client, row)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", row.Name(), err))
		}
	}
	return errors.Join(errs...)
}
