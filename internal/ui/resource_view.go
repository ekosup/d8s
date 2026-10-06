package ui

import (
	"context"

	"github.com/ekosup/d8s/internal/resource"
	"github.com/ekosup/d8s/internal/store"
)

// ShowResource makes res the root view and starts refreshing it. Whatever
// was shown before is closed and its watcher stopped.
func (a *App) ShowResource(res resource.Resource) {
	a.stopWatch()
	p := a.resourcePage(res)
	a.resetStack(p)
	p.resume()
}

// PushResource opens res on top of the current page, as a drill-down.
func (a *App) PushResource(res resource.Resource) {
	p := a.resourcePage(res)
	a.Push(p)
	p.resume()
}

// resourcePage builds the table page for res. Its watcher runs only while
// the page is the visible one: resume starts it, pause and close stop it.
func (a *App) resourcePage(res resource.Resource) *page {
	view := newTableView(res.Title, res.Columns)
	var cancel context.CancelFunc
	stop := func() {
		if cancel != nil {
			cancel()
			cancel = nil
		}
	}
	start := func() {
		stop()
		a.gen++
		gen := a.gen
		a.view = view
		a.stale = false
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		a.cancelWatch = cancel
		a.watchDone = store.Watch(ctx, a.client, res, a.watchOpts, func(s store.Snapshot) {
			a.queue(func() { a.applySnapshot(gen, view, s) })
		})
	}

	actions := append(a.capabilityBindings(res, view), a.actionBindings(res, view)...)
	if len(res.Actions) > 0 {
		actions = append(actions, runeBinding(' ', "space", "Mark", view.toggleMark))
	}
	return &page{
		name: res.Name,
		prim: view,
		bindings: func() []binding {
			return append(append([]binding(nil), actions...), view.bindings()...)
		},
		hints:   func() []binding { return actions },
		table:   view,
		filter:  view,
		pause:   stop,
		resume:  start,
		onClose: stop,
		back: func() bool {
			if view.Filter() != "" {
				view.SetFilter("")
				return true
			}
			return view.ClearMarks()
		},
	}
}

// applySnapshot draws one refresh result. It runs on the UI goroutine.
func (a *App) applySnapshot(gen int, view *tableView, s store.Snapshot) {
	if gen != a.gen {
		return // the view this belongs to has been closed
	}
	if s.Err != nil {
		// Keep the last good rows on screen; they are better than nothing.
		a.Flash(flashError, "refresh failed: "+s.Err.Error())
		a.stale = true
		return
	}
	view.SetRows(s.Rows)
	if a.stale {
		a.clearFlash()
		a.stale = false
	}
}

func (a *App) stopWatch() {
	if a.cancelWatch != nil {
		a.cancelWatch()
		a.cancelWatch = nil
	}
}

// resetStack replaces every page with root.
func (a *App) resetStack(root *page) {
	old := a.stack
	a.stack = nil
	// Add the new page before removing the old ones: a Pages container that
	// is emptied while focused keeps that focus for itself, and would then
	// swallow every key meant for the prompt.
	a.Push(root)
	for _, p := range old {
		a.pages.RemovePage(p.id)
		if p.onClose != nil {
			p.onClose()
		}
	}
	a.tv.SetFocus(root.prim)
}
