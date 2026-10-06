package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/docker"
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

// startAction runs act on its targets, first asking whatever the action
// requires: a value, the row's name typed out, or a plain yes.
func (a *App) startAction(act resource.Action, view *tableView) {
	if act.Target != "" {
		a.confirmAndRun(act, []resource.Row{{Cells: []string{act.Target}}}, view)
		return
	}
	row, ok := view.SelectedRow()
	if !ok {
		return
	}
	switch {
	case act.Input != nil:
		initial := ""
		if act.Input.Default != nil {
			initial = act.Input.Default(row)
		}
		a.ask(act.Name+" "+row.Name(), act.Input.Label, initial, func(value string) bool {
			withValue := act
			withValue.Run = func(ctx context.Context, c docker.Client, r resource.Row) error {
				return act.RunInput(ctx, c, r, value)
			}
			a.runAction(withValue, []resource.Row{row})
			return true
		})
	case act.ConfirmName:
		// One object at a time: a typed name cannot stand for several.
		question := act.Name + " " + row.Name()
		if act.Warn != nil {
			if w := act.Warn(row); w != "" {
				question += "\n" + w
			}
		}
		a.ask(question, "To confirm, type its name", "", func(value string) bool {
			if strings.TrimSpace(value) != row.Name() {
				a.Flash(flashError, "the name does not match; nothing was deleted")
				return false
			}
			a.runAction(act, []resource.Row{row})
			return true
		})
	default:
		rows := view.MarkedRows()
		if len(rows) == 0 {
			rows = []resource.Row{row}
		}
		a.confirmAndRun(act, rows, view)
	}
}

// confirmAndRun runs act on rows, after a yes/no question when it asks for one.
func (a *App) confirmAndRun(act resource.Action, rows []resource.Row, view *tableView) {
	run := func() {
		a.runAction(act, rows)
		view.ClearMarks()
	}
	if !act.Confirm {
		run()
		return
	}
	question := fmt.Sprintf("%s %s?", act.Name, rowNames(rows))
	if len(rows) > 1 {
		// Before destroying several things, name every one of them.
		question = fmt.Sprintf("%s %d items: %s?", act.Name, len(rows), allNames(rows))
	}
	if act.Warn != nil {
		for _, r := range rows {
			if w := act.Warn(r); w != "" {
				question += "\n" + w
				break
			}
		}
	}
	a.confirm(question, run)
}

// runAction executes in the background so a slow daemon cannot freeze the
// screen; the outcome comes back as a status message, and the view is
// refreshed at once rather than at its next poll.
func (a *App) runAction(act resource.Action, rows []resource.Row) {
	if a.executor == nil {
		return
	}
	if !act.Quiet {
		a.Flash(flashInfo, fmt.Sprintf("%s %s…", act.Name, rowNames(rows)))
	}
	executor := a.executor
	go func() {
		err := executor.Run(context.Background(), act, rows)
		a.queue(func() {
			if top := a.top(); top != nil && top.resume != nil {
				top.resume()
			}
			switch {
			case err != nil:
				a.Flash(flashError, fmt.Sprintf("%s %s", act.Name, oneLine(err.Error())))
			case !act.Quiet:
				a.Flash(flashInfo, fmt.Sprintf("%s %s: done", act.Name, rowNames(rows)))
			}
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

// allNames lists every row, up to a number that still fits a dialog.
func allNames(rows []resource.Row) string {
	const limit = 12
	names := make([]string, 0, min(len(rows), limit)+1)
	for i, r := range rows {
		if i == limit {
			names = append(names, fmt.Sprintf("and %d more", len(rows)-limit))
			break
		}
		names = append(names, r.Name())
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

	lines := strings.Split(question, "\n")
	width := 40
	for _, l := range lines {
		width = max(width, len(l)+8)
	}
	width = min(width, 100)
	dialog := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(text, width, 0, true).
			AddItem(nil, 0, 1, false), len(lines)+5, 0, true).
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

const inputPage = "input"

// ask shows a dialog with one text field. submit gets the text on Enter and
// returns whether the dialog should close; Esc closes it without calling.
func (a *App) ask(question, label, initial string, submit func(value string) bool) {
	lines := strings.Split(question, "\n")
	text := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
	text.SetText(tview.Escape(question))

	field := tview.NewInputField().
		SetLabel(" " + label + ": ").
		SetText(initial).
		SetFieldStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Underline(true)).
		SetLabelStyle(tcell.StyleDefault.Foreground(colorTitle))
	field.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter && submit(field.GetText()) {
			a.input = nil
			a.Pop()
		}
	})
	a.input = field

	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(text, len(lines)+1, 0, false).
		AddItem(field, 1, 0, true).
		AddItem(tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter).
			SetText("[steelblue]<enter>[-] ok   [steelblue]<esc>[-] cancel"), 2, 0, false)
	box.SetBorder(true).SetTitle(" Input ").SetBorderColor(toneColors[resource.ToneWarn])

	width := len(label) + 30
	for _, l := range lines {
		width = max(width, len(l)+8)
	}
	width = min(max(width, 50), 100)
	dialog := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(box, width, 0, true).
			AddItem(nil, 0, 1, false), len(lines)+6, 0, true).
		AddItem(nil, 0, 1, false)

	a.Push(&page{name: inputPage, prim: dialog, modal: true, typing: true, onClose: func() { a.input = nil }})
	a.tv.SetFocus(field)
}
