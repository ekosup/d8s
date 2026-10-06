package ui

import "github.com/gdamore/tcell/v2"

// binding ties a key to an action. The same list drives key dispatch and
// the help screen, so the two cannot drift apart.
type binding struct {
	key   tcell.Key // tcell.KeyRune for printable keys
	r     rune
	label string // as shown to the user, e.g. "shift-n"
	desc  string
	do    func()
}

func runeBinding(r rune, label, desc string, do func()) binding {
	return binding{key: tcell.KeyRune, r: r, label: label, desc: desc, do: do}
}

func (b binding) matches(ev *tcell.EventKey) bool {
	if ev.Key() != b.key {
		return false
	}
	return b.key != tcell.KeyRune || ev.Rune() == b.r
}
