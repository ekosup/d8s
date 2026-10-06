package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
)

func (a *App) typeText(s string) {
	for _, r := range s {
		a.key(tcell.KeyRune, r)
	}
}

func stubResource(name string, aliases ...string) resource.Resource {
	return resource.Resource{
		Name:    name,
		Aliases: aliases,
		Title:   strings.ToUpper(name[:1]) + name[1:],
		Columns: []resource.Column{{Name: "THING"}, {Name: "SIZE", Right: true}},
		List: func(context.Context, docker.Client) ([]resource.Row, error) {
			return []resource.Row{{ID: "t1", Cells: []string{"alpha", "42"}}}, nil
		},
	}
}

// promptHarness has three resources so prefixes can be unique or ambiguous.
func promptHarness(t *testing.T, cs ...docker.Container) *harness {
	t.Helper()
	h := newHarness(t, fastWatch, cs...)
	for _, r := range []resource.Resource{stubResource("configs", "cfg"), stubResource("images", "i")} {
		if err := h.app.registry.Register(r); err != nil {
			t.Fatal(err)
		}
	}
	h.show("images")
	h.step()
	return h
}

func (h *harness) lastLine() string {
	h.t.Helper()
	lines := render(h.t, h.app.root, 120, 24)
	return lines[len(lines)-1]
}

func (h *harness) command(cmd string) {
	h.t.Helper()
	h.app.key(tcell.KeyRune, ':')
	h.app.typeText(cmd)
	h.app.key(tcell.KeyEnter, 0)
}

func TestCommandModeOpensViews(t *testing.T) {
	for _, cmd := range []string{"c", "containers", "container", "cont", "CONTAINERS", " c "} {
		t.Run(cmd, func(t *testing.T) {
			h := promptHarness(t)
			h.command(cmd)
			h.step()
			if s := h.screen(); !strings.Contains(s, "<containers>") || !strings.Contains(s, "Containers[0]") {
				t.Fatalf("%q did not open the container view:\n%s", cmd, s)
			}
			if h.app.prompting != promptNone {
				t.Fatal("prompt still open")
			}
		})
	}
}

func TestCommandPromptIsVisibleWhileTyping(t *testing.T) {
	h := promptHarness(t)
	h.app.key(tcell.KeyRune, ':')
	h.app.typeText("co")
	line := h.lastLine()
	if !strings.HasPrefix(strings.TrimSpace(line), ":co") {
		t.Fatalf("prompt not shown: %q", line)
	}
	if !strings.Contains(line, "configs") || !strings.Contains(line, "containers") || strings.Contains(line, "images") {
		t.Fatalf("suggestions wrong: %q", line)
	}
}

func TestCommandAutocomplete(t *testing.T) {
	tests := []struct {
		typed string
		want  string
	}{
		{"cont", "containers"}, // unique
		{"c", "con"},           // configs + containers: longest common prefix
		{"i", "images"},
		{"q", "quit"}, // built-in commands complete too
		{"zz", "zz"},  // no candidate: unchanged
	}
	for _, tt := range tests {
		t.Run(tt.typed, func(t *testing.T) {
			h := promptHarness(t)
			h.app.key(tcell.KeyRune, ':')
			h.app.typeText(tt.typed)
			h.app.key(tcell.KeyTab, 0)
			if got := h.app.prompt.GetText(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandErrors(t *testing.T) {
	tests := []struct {
		cmd  string
		want string
	}{
		{"xyz", `unknown command "xyz"`},
		{"con", `ambiguous command "con"`},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			h := promptHarness(t)
			h.command(tt.cmd)
			if s := h.screen(); !strings.Contains(s, tt.want) || !strings.Contains(s, "<images>") {
				t.Fatalf("want %q and the old view kept:\n%s", tt.want, s)
			}
		})
	}
}

func TestCommandQuit(t *testing.T) {
	for _, cmd := range []string{"q", "quit", "q!"} {
		t.Run(cmd, func(t *testing.T) {
			h := promptHarness(t)
			stopped := false
			h.app.stop = func() { stopped = true }
			h.command(cmd)
			if !stopped {
				t.Fatalf(":%s did not quit", cmd)
			}
		})
	}
}

func TestCommandEscCancels(t *testing.T) {
	h := promptHarness(t)
	h.app.key(tcell.KeyRune, ':')
	h.app.typeText("containers")
	h.app.key(tcell.KeyEscape, 0)
	if s := h.screen(); !strings.Contains(s, "<images>") || h.app.prompting != promptNone {
		t.Fatalf("esc did not cancel the command:\n%s", s)
	}
	// Empty command is a no-op too.
	h.command("")
	if !strings.Contains(h.screen(), "<images>") {
		t.Fatal("empty command changed the view")
	}
}

func TestTypingInPromptDoesNotTriggerViewKeys(t *testing.T) {
	h := promptHarness(t)
	col, desc := h.app.view.model.Sort()
	h.app.key(tcell.KeyRune, ':')
	h.app.typeText("TS?/:") // sort keys, help, filter, command: all just text here
	if got := h.app.prompt.GetText(); got != "TS?/:" {
		t.Fatalf("prompt text %q", got)
	}
	if c, d := h.app.view.model.Sort(); c != col || d != desc || len(h.app.stack) != 1 {
		t.Fatal("keys typed into the prompt reached the view")
	}
}

func TestPromptWorksAgainAfterSwitchingViews(t *testing.T) {
	h := promptHarness(t)
	h.command("containers")
	h.step()

	h.app.key(tcell.KeyRune, ':')
	h.app.typeText("xyz")
	if got := h.app.prompt.GetText(); got != "xyz" {
		t.Fatalf("prompt text %q after switching views; keys are not reaching the prompt", got)
	}
	h.app.key(tcell.KeyEnter, 0)
	if s := h.screen(); !strings.Contains(s, `unknown command "xyz"`) {
		t.Fatalf("command not run:\n%s", s)
	}

	// The table still gets its keys afterwards.
	h.app.key(tcell.KeyRune, '?')
	if !strings.Contains(h.screen(), "<containers> <help>") {
		t.Fatal("keys no longer reach the page")
	}
}
