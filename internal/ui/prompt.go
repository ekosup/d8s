package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/resource"
)

type promptMode int

const (
	promptNone promptMode = iota
	promptCommand
	promptFilter
)

const (
	footerStatus = "status"
	footerPrompt = "prompt"
)

// command is a built-in `:` command that is not a resource.
type command struct {
	names []string // first one is canonical
	desc  string
	do    func()
}

func (a *App) commands() []command {
	return []command{
		{names: []string{"quit", "q", "q!", "exit"}, desc: "Quit", do: func() { a.stop() }},
		{names: []string{"help", "h", "?"}, desc: "Help", do: a.showHelp},
	}
}

func (a *App) buildPrompt() {
	a.prompt = tview.NewInputField().
		SetFieldStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite)).
		SetLabelStyle(tcell.StyleDefault.Foreground(colorTitle).Bold(true))
	a.hint = tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight)

	a.prompt.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Key() {
		case tcell.KeyEnter:
			a.submitPrompt()
			return nil
		case tcell.KeyEscape:
			a.cancelPrompt()
			return nil
		case tcell.KeyTab:
			if a.prompting == promptCommand {
				a.completeCommand()
			}
			return nil
		}
		return ev
	})
	a.prompt.SetChangedFunc(a.promptChanged)
}

func (a *App) openPrompt(mode promptMode) {
	initial := ""
	switch mode {
	case promptFilter:
		p := a.top()
		if p == nil || p.filter == nil {
			return // nothing to filter on this page
		}
		initial = p.filter.Filter()
		a.prompt.SetLabel(" /")
	case promptCommand:
		a.prompt.SetLabel(" :")
	default:
		return
	}
	a.prompting = mode
	a.prompt.SetText(initial)
	a.promptChanged(initial)
	a.footer.SwitchToPage(footerPrompt)
	a.tv.SetFocus(a.prompt)
}

func (a *App) closePrompt() {
	a.prompting = promptNone
	a.prompt.SetText("")
	a.hint.SetText("")
	a.footer.SwitchToPage(footerStatus)
	if p := a.top(); p != nil {
		a.tv.SetFocus(p.prim)
	}
}

func (a *App) submitPrompt() {
	mode, text := a.prompting, a.prompt.GetText()
	a.closePrompt()
	if mode == promptCommand {
		a.runCommand(text)
	}
	// A filter is already applied; Enter only keeps it.
}

func (a *App) cancelPrompt() {
	if a.prompting == promptFilter {
		if p := a.top(); p != nil && p.filter != nil {
			p.filter.SetFilter("")
		}
	}
	a.closePrompt()
}

// promptChanged runs on every edit: filters apply live, commands show what
// the text could complete to.
func (a *App) promptChanged(text string) {
	switch a.prompting {
	case promptFilter:
		if p := a.top(); p != nil && p.filter != nil {
			p.filter.SetFilter(text)
		}
	case promptCommand:
		a.hint.SetText("[gray]" + tview.Escape(strings.Join(a.candidates(text), "  ")) + " ")
	}
}

// candidates lists the canonical commands starting with prefix.
func (a *App) candidates(prefix string) []string {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	var out []string
	if a.registry != nil {
		out = a.registry.Complete(prefix)
	}
	for _, c := range a.commands() {
		if strings.HasPrefix(c.names[0], prefix) {
			out = append(out, c.names[0])
		}
	}
	slices.Sort(out)
	return out
}

// completeCommand extends the typed text as far as the candidates agree.
func (a *App) completeCommand() {
	cands := a.candidates(a.prompt.GetText())
	if len(cands) == 0 {
		return
	}
	common := cands[0]
	for _, c := range cands[1:] {
		for !strings.HasPrefix(c, common) {
			common = common[:len(common)-1]
		}
	}
	if len(common) > len(strings.TrimSpace(a.prompt.GetText())) {
		a.prompt.SetText(common)
	}
}

func (a *App) runCommand(text string) {
	cmd := strings.ToLower(strings.TrimSpace(text))
	if cmd == "" {
		return
	}
	for _, c := range a.commands() {
		if slices.Contains(c.names, cmd) {
			c.do()
			return
		}
	}
	if a.registry == nil {
		a.Flash(flashError, fmt.Sprintf("unknown command %q", cmd))
		return
	}
	res, err := a.registry.Lookup(cmd)
	switch {
	case errors.Is(err, resource.ErrAmbiguous):
		a.Flash(flashError, fmt.Sprintf("ambiguous command %q", cmd))
	case err != nil:
		a.Flash(flashError, fmt.Sprintf("unknown command %q", cmd))
	default:
		a.ShowResource(res)
	}
}
