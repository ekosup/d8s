package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
)

func customHarness(t *testing.T, aliases, hotkeys map[string]string) (*harness, []string) {
	t.Helper()
	h := newHarness(t, fastWatch,
		docker.Container{ID: "1", Name: "shop_web", Image: "nginx", State: "running", Created: created},
		docker.Container{ID: "2", Name: "shop_api", Image: "nginx", State: "running", Created: created},
		docker.Container{ID: "3", Name: "blog", Image: "ghost", State: "running", Created: created},
	)
	h.register(resource.Images(swarmClock), resource.Volumes(swarmClock))
	warnings := h.app.SetCustom(aliases, hotkeys)
	h.show("containers")
	h.step()
	return h, warnings
}

func TestAliasOpensAView(t *testing.T) {
	h, warnings := customHarness(t, map[string]string{"im": "images", "shop": "containers /shop_"}, nil)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	h.command("im")
	h.until("images view", func() bool { return strings.Contains(h.screen(), "<images>") })

	// An alias can carry a filter.
	h.command("shop")
	h.until("filtered containers", func() bool { return strings.Contains(h.screen(), "Containers[2/3] </shop_>") })
	if strings.Contains(h.screen(), "blog") {
		t.Fatalf("filter from the alias not applied:\n%s", h.screen())
	}
}

func TestAliasesAppearInHelpAndCompletion(t *testing.T) {
	h, _ := customHarness(t, map[string]string{"shop": "containers /shop_"}, nil)
	h.app.key(tcell.KeyRune, ':')
	h.app.typeText("sh")
	h.app.key(tcell.KeyTab, 0)
	if got := h.app.prompt.GetText(); got != "shop" {
		t.Fatalf("alias not completed: %q", got)
	}
	h.app.key(tcell.KeyEscape, 0)
	h.app.key(tcell.KeyRune, '?')
	if s := screenOf(t, h.app, 120, 50); !strings.Contains(s, "<:shop>") || !strings.Contains(s, "containers /shop_") {
		t.Fatalf("alias missing from help:\n%s", s)
	}
}

func TestHotkeyRunsACommand(t *testing.T) {
	h, warnings := customHarness(t, map[string]string{"shop": "containers /shop_"}, map[string]string{"f2": "images", "ctrl-w": "shop", "shift-z": "volumes"})
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	h.app.key(tcell.KeyF2, 0)
	h.until("images", func() bool { return strings.Contains(h.screen(), "<images>") })
	h.app.key(tcell.KeyCtrlW, 0)
	h.until("alias through hotkey", func() bool { return strings.Contains(h.screen(), "</shop_>") })
	h.app.key(tcell.KeyRune, 'Z')
	h.until("volumes", func() bool { return strings.Contains(h.screen(), "<volumes>") })

	h.app.key(tcell.KeyRune, '?')
	if s := screenOf(t, h.app, 120, 50); !strings.Contains(s, "<f2>") || !strings.Contains(s, "HOTKEYS") {
		t.Fatalf("hotkeys missing from help:\n%s", s)
	}
}

func TestCustomConflictsAreReportedAndIgnored(t *testing.T) {
	h, warnings := customHarness(t,
		map[string]string{
			"c":     "images",     // shadows a built-in command
			"quit":  "images",     // shadows a built-in command
			"ghost": "nosuchview", // points nowhere
			"ok":    "volumes",
		},
		map[string]string{
			"r":       "images",  // restart on the container view
			"ctrl-d":  "images",  // delete
			"?":       "images",  // help
			"shift-n": "images",  // sort by NAME
			"j":       "images",  // navigation
			"ctrl-c":  "volumes", // quit
			"hyper-x": "images",  // not a key d8s knows
			"f5":      "nowhere", // command that does not exist
			"f3":      "ok",      // fine: an alias that works
		})
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{`alias "c"`, `alias "quit"`, `alias "ghost"`, `hotkey "r"`, `hotkey "ctrl-d"`, `hotkey "?"`,
		`hotkey "shift-n"`, `hotkey "j"`, `hotkey "ctrl-c"`, `hotkey "hyper-x"`, `hotkey "f5"`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("no warning about %s in:\n%s", want, joined)
		}
	}
	if len(warnings) != 11 {
		t.Fatalf("%d warnings, want 11:\n%s", len(warnings), joined)
	}
	if !slices.IsSorted(warnings) {
		t.Fatalf("warnings should come in a stable order:\n%s", joined)
	}

	// Built-in behaviour is untouched...
	h.command("c")
	h.step()
	if !strings.Contains(h.screen(), "<containers>") {
		t.Fatal(":c no longer opens containers")
	}
	h.app.key(tcell.KeyRune, 'r')
	h.until("restart still works", func() bool { return len(h.fake.Calls()) == 1 })
	// ...and what was valid still works.
	h.app.key(tcell.KeyF3, 0)
	h.until("valid hotkey", func() bool { return strings.Contains(h.screen(), "<volumes>") })
}

func TestStartupWarningIsShown(t *testing.T) {
	h, warnings := customHarness(t, map[string]string{"c": "images"}, nil)
	h.app.ShowWarnings(warnings)
	if s := h.screen(); !strings.Contains(s, `alias "c"`) {
		t.Fatalf("warning not shown:\n%s", s)
	}
}
