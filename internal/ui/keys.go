package ui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

// binding ties a key to an action. The same list drives key dispatch and
// the help screen, so the two cannot drift apart.
type binding struct {
	key   tcell.Key // tcell.KeyRune for printable keys
	r     rune
	label string // as shown to the user, e.g. "shift-n"
	desc  string
	do    func()
	group string // help column; empty means the page's own column
}

// Help columns besides a page's own.
const groupSort = "SORT"

func runeBinding(r rune, label, desc string, do func()) binding {
	return binding{key: tcell.KeyRune, r: r, label: label, desc: desc, do: do}
}

func keyBinding(key tcell.Key, label, desc string, do func()) binding {
	return binding{key: key, label: label, desc: desc, do: do}
}

func (b binding) matches(ev *tcell.EventKey) bool {
	if ev.Key() != b.key {
		return false
	}
	return b.key != tcell.KeyRune || ev.Rune() == b.r
}

// parseKey turns a key name from a resource definition ("r", "ctrl-d",
// "enter", "space") into a binding without an action.
func parseKey(name string) (binding, bool) {
	switch {
	case name == "enter":
		return binding{key: tcell.KeyEnter, label: name}, true
	case name == "space":
		return binding{key: tcell.KeyRune, r: ' ', label: name}, true
	case strings.HasPrefix(name, "ctrl-") && len(name) == len("ctrl-")+1:
		c := name[len(name)-1]
		if c < 'a' || c > 'z' {
			return binding{}, false
		}
		return binding{key: tcell.KeyCtrlA + tcell.Key(c-'a'), label: name}, true
	case len([]rune(name)) == 1:
		return binding{key: tcell.KeyRune, r: []rune(name)[0], label: name}, true
	}
	return binding{}, false
}
