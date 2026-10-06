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
	{"ctrl-f", "Page down"},
	{"ctrl-b", "Page up"},
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
		// One column per group, in order of first appearance; the page's
		// own (ungrouped) bindings come under its name.
		var order []string
		groups := map[string][]binding{}
		for _, b := range top.bindings() {
			g := b.group
			if g == "" {
				g = strings.ToUpper(top.name)
			}
			if _, seen := groups[g]; !seen {
				order = append(order, g)
			}
			groups[g] = append(groups[g], b)
		}
		for _, g := range order {
			add(g, entriesOf(groups[g]))
		}
	}
	if top.table != nil {
		add("NAVIGATION", tableNavigation)
	}
	add("GENERAL", entriesOf(a.globalBindings()))
	columns.SetBorder(true).SetTitle(" Help ")

	a.Push(&page{name: helpPage, prim: columns})
}

func entriesOf(bs []binding) []helpEntry {
	out := make([]helpEntry, 0, len(bs))
	for _, b := range bs {
		if b.do == nil {
			continue // a header-only summary of other keys
		}
		out = append(out, helpEntry{label: b.label, desc: b.desc})
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
