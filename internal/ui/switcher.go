package ui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/docker"
)

// maxContextKeys is how many contexts get a number key: 1 to 9, in the
// order of the context list, so a context keeps its key between sessions.
const maxContextKeys = 9

const groupContexts = "SWITCH CONTEXT"

// contextBindings are the number keys that switch context. There are none
// when switching is not possible or there is nothing to switch to.
func (a *App) contextBindings() []binding {
	if a.connect == nil || a.contexts == nil {
		return nil
	}
	eps, err := a.contexts()
	if err != nil || len(eps) < 2 {
		return nil
	}
	eps = eps[:min(len(eps), maxContextKeys)]
	out := make([]binding, len(eps))
	for i, ep := range eps {
		digit := rune('1' + i)
		out[i] = runeBinding(digit, string(digit), ep.Context, func() { a.switchTo(ep) })
		out[i].group = groupContexts
	}
	return out
}

// contextKeysApply reports whether the page on top answers to the number
// keys. Only tables do: logs use digits for their range, dialogs for text.
func (a *App) contextKeysApply() bool {
	p := a.top()
	return p != nil && p.table != nil && !p.modal
}

// contextColumns renders the number keys for the header, headerHeight to a
// column, with the context in use set apart. It returns each column's text
// and width.
func (a *App) contextColumns() (texts []string, widths []int) {
	if !a.contextKeysApply() {
		return nil, nil
	}
	bs := a.contextBindings()
	for len(bs) > 0 {
		n := min(len(bs), headerHeight)
		var sb strings.Builder
		width := 0
		for _, b := range bs[:n] {
			tag := ""
			if b.desc == a.info.Context {
				tag = theme.tagValue
			}
			fmt.Fprintf(&sb, "%s<%s>[-:-:-] %s%s[-:-:-]\n", theme.tagKey, b.label, tag, tview.Escape(b.desc))
			width = max(width, len("<1> ")+len([]rune(b.desc)))
		}
		texts = append(texts, sb.String())
		widths = append(widths, width+2)
		bs = bs[n:]
	}
	return texts, widths
}

// contextArgument splits "ctx name" into the command and the name. ok is
// false unless the command opens a view that connects to what it lists.
func (a *App) contextArgument(cmd string) (head, arg string, ok bool) {
	head, arg, found := strings.Cut(cmd, " ")
	if !found || a.registry == nil {
		return "", "", false
	}
	res, err := a.registry.Lookup(head)
	if err != nil || res.Connect == nil {
		return "", "", false
	}
	return head, strings.TrimSpace(arg), true
}

// contextCandidates lists the contexts whose name starts with what is typed
// after a context command. ok is false when text is not such a command.
func (a *App) contextCandidates(text string) (head string, names []string, ok bool) {
	head, arg, ok := a.contextArgument(strings.ToLower(strings.TrimLeft(text, " ")))
	if !ok || a.contexts == nil {
		return "", nil, false
	}
	eps, err := a.contexts()
	if err != nil {
		return "", nil, false
	}
	for _, ep := range eps {
		if strings.HasPrefix(strings.ToLower(ep.Context), arg) {
			names = append(names, ep.Context)
		}
	}
	return head, names, true
}

// switchToNamed switches to the context called name, or to the only one
// whose name starts with it. Case does not matter.
func (a *App) switchToNamed(name string) {
	var eps []docker.Endpoint
	if a.contexts != nil {
		eps, _ = a.contexts()
	}
	var matches []docker.Endpoint
	for _, ep := range eps {
		if strings.EqualFold(ep.Context, name) {
			matches = []docker.Endpoint{ep}
			break
		}
		if strings.HasPrefix(strings.ToLower(ep.Context), strings.ToLower(name)) {
			matches = append(matches, ep)
		}
	}
	switch {
	case len(matches) == 0:
		a.Flash(flashError, fmt.Sprintf("unknown context %q", name))
	case len(matches) > 1:
		a.Flash(flashError, fmt.Sprintf("ambiguous context %q", name))
	case matches[0].Context == a.info.Context:
		a.Flash(flashInfo, "already on "+a.info.Context)
	default:
		a.switchTo(matches[0])
	}
}
