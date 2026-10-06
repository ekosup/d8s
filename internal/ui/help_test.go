package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestHelpListsEveryBinding(t *testing.T) {
	h := filterHarness(t)
	view := h.app.top()
	h.app.key(tcell.KeyRune, '?')
	s := h.screen()

	if !strings.Contains(s, "<containers> <help>") {
		t.Fatalf("help page not on the stack:\n%s", s)
	}
	all := append(view.bindings(), h.app.globalBindings()...)
	if len(all) < 8 {
		t.Fatalf("only %d bindings; the test would prove nothing", len(all))
	}
	for _, b := range all {
		if !strings.Contains(s, "<"+b.label+">") || !strings.Contains(s, b.desc) {
			t.Fatalf("binding <%s> %q missing from help:\n%s", b.label, b.desc, s)
		}
	}
	for _, want := range []string{"Sort by NAME", "Sort by AGE", "Command mode", "Filter", "Quit"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in help:\n%s", want, s)
		}
	}
}

func TestHelpFollowsTheActiveView(t *testing.T) {
	h := promptHarness(t) // shows "images": columns THING and SIZE
	h.app.key(tcell.KeyRune, '?')
	s := h.screen()
	if !strings.Contains(s, "Sort by THING") || !strings.Contains(s, "Sort by SIZE") || strings.Contains(s, "Sort by NAME") {
		t.Fatalf("help does not match the active view:\n%s", s)
	}
}

func TestHelpClosesWithEscAndDoesNotStack(t *testing.T) {
	h := filterHarness(t)
	h.app.key(tcell.KeyRune, '?')
	h.app.key(tcell.KeyRune, '?')
	if len(h.app.stack) != 2 {
		t.Fatalf("stack depth %d after pressing ? twice", len(h.app.stack))
	}
	h.app.key(tcell.KeyEscape, 0)
	if s := h.screen(); len(h.app.stack) != 1 || !strings.Contains(s, "Containers[3]") {
		t.Fatalf("esc did not close help:\n%s", s)
	}
}

func TestHelpCommand(t *testing.T) {
	h := filterHarness(t)
	h.command("help")
	if !strings.Contains(h.screen(), "<containers> <help>") {
		t.Fatal(":help did not open help")
	}
}

func TestHeaderHintsComeFromBindings(t *testing.T) {
	a := NewApp(testInfo)
	a.Push(textPage("home", "body"))
	line := render(t, a.root, 120, 24)[1]
	for _, b := range a.globalBindings() {
		if !strings.Contains(line, "<"+b.label+">") {
			t.Fatalf("hint for <%s> missing from %q", b.label, line)
		}
	}
}
