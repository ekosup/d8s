package ui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

const helpPage = "help"

// helpEntry is one line of the help screen.
type helpEntry struct{ label, desc string }

// tableNavigation documents the keys the table handles itself.
var tableNavigation = []helpEntry{
	{"j / ↓", "Down"},
	{"k / ↑", "Up"},
	{"g", "First row"},
	{"shift-g", "Last row"},
	{"ctrl-f / pgdn", "Page down"},
	{"ctrl-b / pgup", "Page up"},
}

// showHelp opens the help screen for the page it is called from. Its
// content is built from the same bindings that handle the keys.
func (a *App) showHelp() {
	top := a.top()
	if top == nil || top.name == helpPage {
		return
	}
	columns := tview.NewFlex()
	add := func(title string, entries []helpEntry) {
		if len(entries) > 0 {
			columns.AddItem(helpColumn(title, entries), 0, 1, false)
		}
	}
	if top.bindings != nil {
		add(strings.ToUpper(top.name), entriesOf(top.bindings()))
	}
	if top.table != nil {
		add("NAVIGATION", tableNavigation)
	}
	add("GENERAL", entriesOf(a.globalBindings()))
	columns.SetBorder(true).SetTitle(" Help ")

	a.Push(&page{name: helpPage, prim: columns})
}

func entriesOf(bs []binding) []helpEntry {
	out := make([]helpEntry, len(bs))
	for i, b := range bs {
		out[i] = helpEntry{label: b.label, desc: b.desc}
	}
	return out
}

func helpColumn(title string, entries []helpEntry) *tview.TextView {
	width := 0
	for _, e := range entries {
		width = max(width, len([]rune(e.label)))
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, " [aqua::b]%s[-:-:-]\n", tview.Escape(title))
	for _, e := range entries {
		pad := strings.Repeat(" ", width-len([]rune(e.label)))
		fmt.Fprintf(&sb, " [steelblue]<%s>[-]%s  %s\n", tview.Escape(e.label), pad, tview.Escape(e.desc))
	}
	return tview.NewTextView().SetDynamicColors(true).SetText(sb.String())
}
