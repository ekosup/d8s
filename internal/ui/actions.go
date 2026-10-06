package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/resource"
)

const confirmPage = "confirm"

// actionBindings turns a resource's actions into key bindings on view.
func (a *App) actionBindings(res resource.Resource, view *tableView) []binding {
	var out []binding
	for _, act := range res.Actions {
		b, ok := parseKey(act.Key)
		if !ok {
			continue
		}
		b.desc = act.Name
		b.do = func() { a.startAction(act, view) }
		out = append(out, b)
	}
	return out
}

// startAction runs act on the selected row, asking first when it must.
func (a *App) startAction(act resource.Action, view *tableView) {
	row, ok := view.SelectedRow()
	if !ok {
		return
	}
	rows := []resource.Row{row}
	if !act.Confirm {
		a.runAction(act, rows)
		return
	}
	a.confirm(fmt.Sprintf("%s %s?", act.Name, rowNames(rows)), func() { a.runAction(act, rows) })
}

// runAction executes in the background so a slow daemon cannot freeze the
// screen; the outcome comes back as a status message.
func (a *App) runAction(act resource.Action, rows []resource.Row) {
	if a.executor == nil {
		return
	}
	a.Flash(flashInfo, fmt.Sprintf("%s %s…", act.Name, rowNames(rows)))
	go func() {
		err := a.executor.Run(context.Background(), act, rows)
		a.queue(func() {
			if err != nil {
				a.Flash(flashError, fmt.Sprintf("%s %s", act.Name, oneLine(err.Error())))
				return
			}
			a.Flash(flashInfo, fmt.Sprintf("%s %s: done", act.Name, rowNames(rows)))
		})
	}()
}

func rowNames(rows []resource.Row) string {
	if len(rows) > 3 {
		return fmt.Sprintf("%d items", len(rows))
	}
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Name()
	}
	return strings.Join(names, ", ")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// confirm shows a yes/no dialog over the current page.
func (a *App) confirm(question string, yes func()) {
	text := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
	text.SetText(fmt.Sprintf("\n%s\n\n[steelblue]<y>[-] yes   [steelblue]<n>[-] no", tview.Escape(question)))
	text.SetBorder(true).SetTitle(" Confirm ").SetBorderColor(toneColors[resource.ToneWarn])

	width := min(max(len(question)+8, 40), 100)
	dialog := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(text, width, 0, true).
			AddItem(nil, 0, 1, false), 6, 0, true).
		AddItem(nil, 0, 1, false)

	a.Push(&page{
		name:  confirmPage,
		prim:  dialog,
		modal: true,
		bindings: func() []binding {
			return []binding{
				runeBinding('y', "y", "Yes", func() {
					a.Pop()
					yes()
				}),
				runeBinding('n', "n", "No", func() { a.Pop() }),
			}
		},
	})
}
