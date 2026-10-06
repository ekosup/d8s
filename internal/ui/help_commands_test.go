package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/resource"
)

func commandHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, fastWatch)
	h.register(resource.Images(swarmClock), resource.Volumes(swarmClock), resource.Networks(swarmClock), resource.Compose(swarmClock),
		resource.Services(swarmClock), resource.Tasks(swarmClock), resource.Nodes(swarmClock), resource.Stacks(swarmClock),
		resource.Secrets(swarmClock), resource.Configs(swarmClock), resource.DiskUsage())
	h.show("containers")
	h.step()
	return h
}

func TestHelpListsEveryCommand(t *testing.T) {
	h := commandHarness(t)
	h.app.key(tcell.KeyRune, '?')
	s := screenOf(t, h.app, 120, 50)
	if !strings.Contains(s, "COMMANDS") {
		t.Fatalf("no commands section:\n%s", s)
	}
	want := map[string]string{
		"c": "containers", "i": "images", "v": "volumes", "n": "networks", "cp": "compose", "df": "df",
		"svc": "services", "ts": "tasks", "no": "nodes", "stk": "stacks", "sec": "secrets", "cfg": "configs",
		"q": "quit", "h": "help",
	}
	for short, name := range want {
		re := regexp.MustCompile(regexp.QuoteMeta("<:"+short+">") + `\s+` + regexp.QuoteMeta(name) + `(\s|│|$)`)
		if !re.MatchString(s) {
			t.Fatalf("command <:%s> %s missing from help:\n%s", short, name, s)
		}
	}
}

func TestHelpCommandsAreTheOnesThatWork(t *testing.T) {
	h := commandHarness(t)
	entries := h.app.commandEntries()
	if len(entries) < 14 {
		t.Fatalf("only %d commands listed", len(entries))
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.label] {
			t.Fatalf("command %q listed twice", e.label)
		}
		seen[e.label] = true
		short := strings.TrimPrefix(e.label, ":")
		if e.desc == "quit" || e.desc == "help" {
			continue
		}
		// What help advertises must open exactly the view it names.
		res, err := h.app.registry.Lookup(short)
		if err != nil || res.Name != e.desc {
			t.Fatalf("help says :%s opens %s, but it resolves to %q (%v)", short, e.desc, res.Name, err)
		}
		// And it is the shortest way to say it.
		for _, alt := range append([]string{res.Name}, res.Aliases...) {
			if len(alt) < len(short) {
				t.Fatalf(":%s is advertised for %s although :%s is shorter", short, res.Name, alt)
			}
		}
	}
}

func TestHelpCommandsFollowRegistrationOrder(t *testing.T) {
	h := commandHarness(t)
	var names []string
	for _, e := range h.app.commandEntries() {
		names = append(names, e.desc)
	}
	got := strings.Join(names, " ")
	// Engine views first, then swarm views, as registered; built-ins last.
	if !strings.HasPrefix(got, "containers images volumes networks compose services") || !strings.HasSuffix(got, "help quit") {
		t.Fatalf("order: %s", got)
	}
}

func TestHelpScrollsOnAShortTerminal(t *testing.T) {
	h := commandHarness(t)
	h.app.key(tcell.KeyRune, '?')
	if !strings.Contains(screenOf(t, h.app, 120, 24), "Sort by NAME") {
		t.Fatal("keys should be visible at the top of help")
	}
	h.app.key(tcell.KeyRune, 'G') // jump to the end
	s := screenOf(t, h.app, 120, 24)
	if !strings.Contains(s, "COMMANDS") || !strings.Contains(s, "<:svc>") {
		t.Fatalf("commands not reachable by scrolling on a 24-row terminal:\n%s", s)
	}
	h.app.key(tcell.KeyRune, 'g')
	if !strings.Contains(screenOf(t, h.app, 120, 24), "Sort by NAME") {
		t.Fatal("g did not return to the top")
	}
	h.app.key(tcell.KeyEscape, 0)
	if len(h.app.stack) != 1 {
		t.Fatal("esc did not close help")
	}
}

func TestHelpWithoutARegistryStillShowsBuiltins(t *testing.T) {
	a := NewApp(testInfo)
	a.Push(textPage("home", "body"))
	a.key(tcell.KeyRune, '?')
	if s := screenOf(t, a, 120, 40); !strings.Contains(s, "<:q>") || !strings.Contains(s, "<:h>") {
		t.Fatalf("built-in commands missing:\n%s", s)
	}
}
