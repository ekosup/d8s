package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/rivo/tview"
)

const helpPage = "help"

// helpEntry is one line of the help screen: a key or command, and what it does.
type helpEntry struct{ label, desc string }

// helpSection is a titled list of entries, laid out as one column.
type helpSection struct {
	title   string
	entries []helpEntry
}

// tableNavigation documents the keys the table handles itself.
var tableNavigation = []helpEntry{
	{"j / ↓", "Down"},
	{"k / ↑", "Up"},
	{"g", "First row"},
	{"shift-g", "Last row"},
	{"ctrl-f", "Page down"},
	{"ctrl-b", "Page up"},
}

// commandColumns is how many columns the command list is spread over.
const commandColumns = 4

// showHelp opens the help screen for the page it is called from: that
// page's keys, built from the same bindings that handle them, and below
// them every `:` command. The screen scrolls when it does not fit.
func (a *App) showHelp() {
	top := a.top()
	if top == nil || top.name == helpPage {
		return
	}
	var keys []helpSection
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
			keys = append(keys, helpSection{g, entriesOf(groups[g])})
		}
	}
	if top.table != nil {
		keys = append(keys, helpSection{"NAVIGATION", tableNavigation})
	}
	keys = append(keys, helpSection{"GENERAL", entriesOf(a.globalBindings())})

	// The commands flow into equal columns under one heading.
	cmds := a.commandEntries()
	perColumn := (len(cmds) + commandColumns - 1) / commandColumns
	var commands []helpSection
	for i := 0; i < len(cmds); i += perColumn {
		title := ""
		if i == 0 {
			title = "COMMANDS"
		}
		commands = append(commands, helpSection{title, cmds[i:min(i+perColumn, len(cmds))]})
	}

	text := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	text.SetText(layoutSections(keys) + "\n" + layoutSections(commands))
	text.SetBorder(true).SetTitle(" Help (j/k to scroll, esc to close) ")
	a.Push(&page{name: helpPage, prim: text})
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

// commandEntries lists every `:` command by its shortest spelling, with the
// view it opens: resources in registration order, then the built-ins.
func (a *App) commandEntries() []helpEntry {
	var out []helpEntry
	if a.registry != nil {
		for _, res := range a.registry.All() {
			out = append(out, helpEntry{label: ":" + shortest(append([]string{res.Name}, res.Aliases...)), desc: res.Name})
		}
	}
	builtins := a.commands()
	slices.SortFunc(builtins, func(x, y command) int { return strings.Compare(x.names[0], y.names[0]) })
	for _, c := range builtins {
		out = append(out, helpEntry{label: ":" + shortest(c.names), desc: c.names[0]})
	}
	return out
}

// shortest returns the shortest name; among equals, the earliest.
func shortest(names []string) string {
	best := names[0]
	for _, n := range names[1:] {
		if len(n) < len(best) {
			best = n
		}
	}
	return best
}

// layoutSections sets sections side by side as text: each a column with
// its title on top and its labels padded to one width.
func layoutSections(sections []helpSection) string {
	const gap = 3
	type column struct {
		lines []string // styled
		plain []int    // visible width of each line
		width int
	}
	cols := make([]column, len(sections))
	rows := 0
	for i, sec := range sections {
		labelWidth := 0
		for _, e := range sec.entries {
			labelWidth = max(labelWidth, len([]rune(e.label))+2)
		}
		c := column{
			lines: []string{fmt.Sprintf("[aqua::b]%s[-:-:-]", tview.Escape(sec.title))},
			plain: []int{len([]rune(sec.title))},
		}
		for _, e := range sec.entries {
			label := "<" + e.label + ">"
			pad := strings.Repeat(" ", labelWidth-len([]rune(label)))
			c.lines = append(c.lines, fmt.Sprintf("[dodgerblue::b]%s[-:-:-]%s  %s", tview.Escape(label), pad, tview.Escape(e.desc)))
			c.plain = append(c.plain, labelWidth+2+len([]rune(e.desc)))
		}
		for _, w := range c.plain {
			c.width = max(c.width, w)
		}
		cols[i] = c
		rows = max(rows, len(c.lines))
	}
	var sb strings.Builder
	for r := range rows {
		sb.WriteByte(' ')
		for i, c := range cols {
			line, width := "", 0
			if r < len(c.lines) {
				line, width = c.lines[r], c.plain[r]
			}
			sb.WriteString(line)
			if i < len(cols)-1 {
				sb.WriteString(strings.Repeat(" ", c.width-width+gap))
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}
