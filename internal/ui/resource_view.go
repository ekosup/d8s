package ui

import (
	"context"
	"fmt"
	"slices"

	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/resource"
	"github.com/ekosup/d8s/internal/store"
)

// ShowResource makes res the root view and starts refreshing it. Whatever
// was shown before is closed and its watcher stopped.
func (a *App) ShowResource(res resource.Resource) {
	a.stopWatch()
	p := a.resourcePage(res)
	a.resetStack(p)
	if p.resume != nil {
		p.resume()
	}
}

// PushResource opens res on top of the current page, as a drill-down.
func (a *App) PushResource(res resource.Resource) {
	p := a.resourcePage(res)
	a.Push(p)
	if p.resume != nil {
		p.resume()
	}
}

// resourcePage builds the table page for res. Its watcher runs only while
// the page is the visible one: resume starts it, pause and close stop it.
func (a *App) resourcePage(res resource.Resource) *page {
	if res.Swarm && !a.info.Swarm.Manager {
		return a.notManagerPage(res)
	}
	view := newTableView(res.Title, res.Columns)
	view.note = res.Note
	view.SetSort(res.SortColumn, res.SortDesc)
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
		if a.stale {
			a.stale = false
			a.drawHeader()
		}
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		a.cancelWatch = cancel
		a.watchDone = store.Watch(ctx, a.client, res, a.watchOpts, func(s store.Snapshot) {
			a.queue(func() { a.applySnapshot(gen, view, s) })
		})
	}

	actions := append(a.capabilityBindings(res, view), a.actionBindings(res, view)...)
	// advertised is what the header shows. Marking stays out of it: the
	// header has room for a view's own verbs, and help lists every key.
	advertised := actions
	if len(res.Actions) > 0 {
		actions = append(slices.Clone(actions), runeBinding(' ', "space", "Mark", view.toggleMark))
	}
	return &page{
		name: res.Name,
		prim: view,
		bindings: func() []binding {
			return append(append([]binding(nil), actions...), view.bindings()...)
		},
		hints:   func() []binding { return advertised },
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
		if !a.stale {
			a.log.Warn("refresh failed", "error", errText(s.Err), "context", a.info.Context)
		}
		// Keep the last good rows on screen; they are better than nothing.
		a.Flash(flashError, "refresh failed: "+s.Err.Error())
		a.setStale(view, true)
		return
	}
	view.SetRows(s.Rows)
	if a.stale {
		a.clearFlash()
		a.setStale(view, false)
	}
}

// setStale marks, or unmarks, the view as showing data from before the
// daemon stopped answering. The header says so too; both clear on the
// first refresh that works again, which the watcher keeps attempting.
func (a *App) setStale(view *tableView, stale bool) {
	if a.stale == stale && view.stale == stale {
		return
	}
	a.stale = stale
	view.stale = stale
	view.refresh()
	a.drawHeader()
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

// notManagerPage stands in for a swarm view when the connected daemon
// cannot serve it, and says why instead of showing the daemon's error.
func (a *App) notManagerPage(res resource.Resource) *page {
	reason := "This engine is not part of a swarm."
	if a.info.Swarm.Active {
		reason = "This engine is a swarm worker; only a manager can answer for the cluster."
	}
	text := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetDynamicColors(true)
	text.SetText(fmt.Sprintf("\n\n[yellow]%s[-]\n\n%s needs a connection to a swarm manager.\nPick one with [steelblue]:ctx[-], or go back with [steelblue]:c[-].",
		reason, tview.Escape(res.Title)))
	text.SetBorder(true).SetTitle(" " + tview.Escape(res.Title) + " ")
	return &page{name: res.Name, prim: text}
}
