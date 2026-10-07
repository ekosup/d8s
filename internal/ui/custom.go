package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
)

const groupHotkeys = "HOTKEYS"

// SetCustom installs the user's aliases and hotkeys. Anything that would
// shadow a built-in command or key, or that points at nothing, is left out
// and reported in the returned warnings, sorted. Call it after the
// registry is complete.
func (a *App) SetCustom(aliases, hotkeys map[string]string) []string {
	var warnings []string
	warn := func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }

	a.aliases = map[string]string{}
	for _, name := range sortedKeys(aliases) {
		target := strings.TrimSpace(aliases[name])
		switch {
		case a.isBuiltinCommand(name):
			warn("alias %q: already a built-in command; ignored", name)
		case !a.resolves(firstWord(target)):
			warn("alias %q: %q is not a view or command; ignored", name, firstWord(target))
		default:
			a.aliases[name] = target
		}
	}

	a.hotkeys = nil
	reserved := a.reservedKeys()
	for _, key := range sortedKeys(hotkeys) {
		cmd := strings.TrimSpace(hotkeys[key])
		b, ok := parseKey(key)
		switch {
		case !ok:
			warn("hotkey %q: not a key d8s understands; ignored", key)
		case reserved[b.label]:
			warn("hotkey %q: already used by d8s; ignored", key)
		case !a.resolves(firstWord(cmd)):
			warn("hotkey %q: %q is not a command; ignored", key, firstWord(cmd))
		default:
			b.desc = ":" + cmd
			b.group = groupHotkeys
			b.do = func() { a.runCommand(cmd) }
			a.hotkeys = append(a.hotkeys, b)
		}
	}
	slices.Sort(warnings)
	return warnings
}

// ShowWarnings puts start-up warnings on the status line.
func (a *App) ShowWarnings(warnings []string) {
	for _, w := range warnings {
		a.log.Warn("configuration", "problem", w)
	}
	switch len(warnings) {
	case 0:
	case 1:
		a.Flash(flashWarn, warnings[0])
	default:
		a.Flash(flashWarn, fmt.Sprintf("%s (and %d more)", warnings[0], len(warnings)-1))
	}
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// isBuiltinCommand reports whether name is exactly a command d8s ships
// with: a built-in, or the name or alias of a registered view.
func (a *App) isBuiltinCommand(name string) bool {
	for _, c := range a.commands() {
		if slices.Contains(c.names, name) {
			return true
		}
	}
	return a.registry != nil && a.registry.Has(name)
}

// resolves reports whether cmd would do something when typed after `:`.
func (a *App) resolves(cmd string) bool {
	if cmd == "" {
		return false
	}
	if _, ok := a.aliases[cmd]; ok {
		return true
	}
	if a.isBuiltinCommand(cmd) {
		return true
	}
	if a.registry == nil {
		return false
	}
	_, err := a.registry.Lookup(cmd)
	return err == nil
}

// reservedKeys is every key d8s binds somewhere, by label. A hotkey may
// not take one of them, even on views where it happens to be free: the
// same key doing different things from view to view is how accidents start.
func (a *App) reservedKeys() map[string]bool {
	reserved := map[string]bool{}
	add := func(labels ...string) {
		for _, l := range labels {
			reserved[l] = true
		}
	}
	for _, b := range a.globalBindings() {
		add(b.label)
	}
	// Table and text navigation, capabilities, marking, dialogs, pager and
	// logs, and the number keys that switch context.
	add("j", "k", "g", "shift-g", "h", "l", "ctrl-f", "ctrl-b", "enter", "space", "esc",
		"d", "y", "s", "n", "shift-n", "w", "c", "t", "ctrl-s", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9")
	if a.registry != nil {
		for _, res := range a.registry.All() {
			for _, act := range res.Actions {
				add(act.Key)
			}
			for _, p := range res.Pages {
				add(p.Key)
			}
			for _, k := range assignSortKeys(res.Columns) {
				add("shift-" + strings.ToLower(string(k.r)))
			}
		}
	}
	return reserved
}

// functionKey maps "f1".."f12" to tcell keys.
func functionKey(name string) (tcell.Key, bool) {
	var n int
	if _, err := fmt.Sscanf(name, "f%d", &n); err != nil || n < 1 || n > 12 || fmt.Sprintf("f%d", n) != name {
		return 0, false
	}
	return tcell.KeyF1 + tcell.Key(n-1), true
}
