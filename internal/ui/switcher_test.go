package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
)

// switcherHarness is a container view with the known contexts wired in,
// the way the real application has them.
func switcherHarness(t *testing.T, names ...string) *contextFixture {
	t.Helper()
	fx := contextHarness(t)
	eps := make([]docker.Endpoint, len(names))
	for i, n := range names {
		eps[i] = docker.Endpoint{Context: n, Host: "ssh://" + n}
	}
	fx.app.contexts = func() ([]docker.Endpoint, error) { return eps, nil }
	fx.show("containers")
	fx.step()
	return fx
}

func TestContextCommandSwitchesByName(t *testing.T) {
	for _, typed := range []string{"ctx staging", "ctx sta", "context STAGING", "ctx  staging "} {
		t.Run(typed, func(t *testing.T) {
			fx := switcherHarness(t, "default", "staging")
			fx.command(typed)
			fx.until("new context", func() bool { return strings.Contains(fx.screen(), "staging-api") })
			if len(fx.dialled) != 1 || fx.dialled[0] != "staging" {
				t.Fatalf("dialled %v", fx.dialled)
			}
		})
	}
}

func TestContextCommandErrors(t *testing.T) {
	tests := []struct{ typed, want string }{
		{"ctx nope", `unknown context "nope"`},
		{"ctx sta", `ambiguous context "sta"`},
		{"ctx default", "already on default"},
		{"containers staging", `unknown command "containers staging"`},
	}
	for _, tt := range tests {
		t.Run(tt.typed, func(t *testing.T) {
			fx := switcherHarness(t, "default", "stage2", "staging")
			fx.command(tt.typed)
			if s := fx.screen(); !strings.Contains(s, tt.want) {
				t.Fatalf("missing %q:\n%s", tt.want, s)
			}
			if len(fx.dialled) != 0 {
				t.Fatalf("dialled %v", fx.dialled)
			}
		})
	}
}

// An exact name wins even when it is also the start of another one.
func TestContextCommandPrefersTheExactName(t *testing.T) {
	fx := switcherHarness(t, "default", "staging", "staging-eu")
	fx.command("ctx staging")
	fx.until("switch", func() bool { return strings.Contains(fx.screen(), "connected to staging") })
	if len(fx.dialled) != 1 || fx.dialled[0] != "staging" {
		t.Fatalf("dialled %v", fx.dialled)
	}
}

func TestContextCommandCompletesNames(t *testing.T) {
	tests := []struct{ typed, want string }{
		{"ctx st", "ctx stag"}, // stage2 + staging: longest common prefix
		{"ctx stagi", "ctx staging"},
		{"ctx d", "ctx default"},
		{"ctx zz", "ctx zz"},
		{"containers st", "containers st"}, // only a context view takes a name
	}
	for _, tt := range tests {
		t.Run(tt.typed, func(t *testing.T) {
			fx := switcherHarness(t, "default", "stage2", "staging")
			fx.app.key(tcell.KeyRune, ':')
			fx.app.typeText(tt.typed)
			fx.app.key(tcell.KeyTab, 0)
			if got := fx.app.prompt.GetText(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}

	// While typing, the names that still fit are suggested.
	fx := switcherHarness(t, "default", "stage2", "staging")
	fx.app.key(tcell.KeyRune, ':')
	fx.app.typeText("ctx st")
	if s := fx.screen(); !strings.Contains(s, "stage2  staging") || strings.Contains(s, "default  stage2") {
		t.Fatalf("suggestions:\n%s", s)
	}
}

func TestNumberKeySwitchesContext(t *testing.T) {
	fx := switcherHarness(t, "default", "staging")
	fx.app.key(tcell.KeyRune, '1') // the one in use
	fx.app.key(tcell.KeyRune, '3') // no such context
	if len(fx.dialled) != 0 {
		t.Fatalf("dialled %v", fx.dialled)
	}
	fx.app.key(tcell.KeyRune, '2')
	fx.until("new context", func() bool { return strings.Contains(fx.screen(), "staging-api") })
	if len(fx.dialled) != 1 || fx.dialled[0] != "staging" {
		t.Fatalf("dialled %v", fx.dialled)
	}
}

// Number keys belong to tables. Elsewhere they mean other things (the log
// range, text in a dialog) or nothing.
func TestNumberKeyDoesNothingOutsideTables(t *testing.T) {
	fx := switcherHarness(t, "default", "staging")
	fx.app.Push(textPage("notes", "body"))
	fx.app.key(tcell.KeyRune, '2')
	fx.app.key(tcell.KeyEscape, 0)
	fx.app.key(tcell.KeyRune, ':')
	fx.app.typeText("2")
	fx.app.key(tcell.KeyEscape, 0)
	if len(fx.dialled) != 0 {
		t.Fatalf("dialled %v", fx.dialled)
	}
}

func TestOnlyNineContextsGetAKey(t *testing.T) {
	fx := switcherHarness(t, "default", "c2", "c3", "c4", "c5", "c6", "c7", "c8", "c9", "c10")
	fx.app.key(tcell.KeyRune, '9')
	fx.until("switch", func() bool { return strings.Contains(fx.screen(), "connected to c9") })
	if len(fx.dialled) != 1 || fx.dialled[0] != "c9" {
		t.Fatalf("dialled %v", fx.dialled)
	}
	fx.app.key(tcell.KeyRune, '0')
	if len(fx.dialled) != 1 {
		t.Fatalf("dialled %v", fx.dialled)
	}
}

func TestHeaderListsContextsWhenThereIsRoom(t *testing.T) {
	fx := switcherHarness(t, "default", "portalprod", "scdev", "scprod", "c5", "c6", "c7")
	wide := headerLines(t, fx.app, 160)
	for i, name := range []string{"default", "portalprod", "scdev", "scprod", "c5", "c6"} {
		row, col := hintAt(wide, string(rune('1'+i)), name)
		if row != i {
			t.Fatalf("<%d> %s on row %d, want %d:\n%s", i+1, name, row, i, strings.Join(wide, "\n"))
		}
		if _, first := hintAt(wide, "1", "default"); col != first {
			t.Fatalf("context keys are not one column:\n%s", strings.Join(wide, "\n"))
		}
	}
	// The seventh starts a second column, beside the first.
	if row, col := hintAt(wide, "7", "c7"); row != 0 {
		t.Fatalf("<7> c7 at row %d col %d:\n%s", row, col, strings.Join(wide, "\n"))
	}
	// Contexts sit between the connection and the keys.
	_, ctxCol := hintAt(wide, "1", "default")
	_, keyCol := hintAt(wide, ":", "Command mode")
	if ctxCol < len(" Context: default") || ctxCol > keyCol {
		t.Fatalf("contexts at column %d, general keys at %d:\n%s", ctxCol, keyCol, strings.Join(wide, "\n"))
	}
	// Every key of the view is still there.
	for _, b := range append(fx.app.globalBindings(), fx.app.top().hints()...) {
		if b.noHint {
			continue
		}
		if row, _ := hintAt(wide, b.label, b.desc); row < 0 {
			t.Fatalf("<%s> %s pushed out by the contexts:\n%s", b.label, b.desc, strings.Join(wide, "\n"))
		}
	}

	// On a narrow terminal the keys of the view matter more.
	narrow := headerLines(t, fx.app, 80)
	if row, _ := hintAt(narrow, "2", "portalprod"); row >= 0 {
		t.Fatalf("contexts shown where they do not fit:\n%s", strings.Join(narrow, "\n"))
	}
	for _, b := range append(fx.app.globalBindings(), fx.app.top().hints()...) {
		if !b.noHint && !strings.Contains(strings.Join(narrow, "\n"), "<"+b.label+">") {
			t.Fatalf("key <%s> missing at 80 columns:\n%s", b.label, strings.Join(narrow, "\n"))
		}
	}

	// And they follow the page: no context keys where they do not work.
	fx.app.Push(textPage("notes", "body"))
	if row, _ := hintAt(headerLines(t, fx.app, 160), "2", "portalprod"); row >= 0 {
		t.Fatal("context keys advertised on a page that ignores them")
	}
}

func TestHelpListsContextKeys(t *testing.T) {
	fx := switcherHarness(t, "default", "staging")
	fx.app.key(tcell.KeyRune, '?')
	s := screenOf(t, fx.app, 160, 50)
	for _, want := range []string{"SWITCH CONTEXT", "<1>  default", "<2>  staging"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
}

func TestAliasAndHotkeyCanNameAContext(t *testing.T) {
	fx := switcherHarness(t, "default", "staging")
	warnings := fx.app.SetCustom(map[string]string{"stg": "ctx staging"}, map[string]string{"f6": "ctx staging", "7": "containers"})
	if len(warnings) != 1 || !strings.Contains(warnings[0], `hotkey "7"`) {
		t.Fatalf("warnings: %v", warnings)
	}
	fx.app.key(tcell.KeyF6, 0)
	fx.until("switch by hotkey", func() bool { return strings.Contains(fx.screen(), "connected to staging") })

	fx = switcherHarness(t, "default", "staging")
	fx.app.SetCustom(map[string]string{"stg": "ctx staging"}, nil)
	fx.command("stg")
	fx.until("switch by alias", func() bool { return strings.Contains(fx.screen(), "connected to staging") })
}
