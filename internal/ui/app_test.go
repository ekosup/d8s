package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/docker"
)

var testInfo = docker.Info{Context: "default", Host: "unix:///var/run/docker.sock", ServerVersion: "27.3.1", APIVersion: "1.47"}

func textPage(name, text string) *page {
	return &page{name: name, prim: tview.NewTextView().SetText(text)}
}

// key feeds one key through the application the way the event loop does:
// global handling first, then the focused primitive.
func (a *App) key(k tcell.Key, r rune) {
	ev := a.handleKey(tcell.NewEventKey(k, r, tcell.ModNone))
	if ev == nil {
		return
	}
	if p := a.tv.GetFocus(); p != nil {
		if h := p.InputHandler(); h != nil {
			h(ev, func(p tview.Primitive) { a.tv.SetFocus(p) })
		}
	}
}

func screenOf(t *testing.T, a *App, w, h int) string {
	t.Helper()
	return strings.Join(render(t, a.root, w, h), "\n")
}

func TestAppHeaderShowsConnection(t *testing.T) {
	a := NewApp(testInfo)
	a.Push(textPage("home", "hello"))
	lines := render(t, a.root, 100, 24)
	for _, want := range []string{"Context: default", "Engine: 27.3.1", "API: 1.47"} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("header %q does not contain %q", lines[0], want)
		}
	}
	if !strings.Contains(lines[1], "<ctrl-c>") {
		t.Fatalf("no key hints in %q", lines[1])
	}
}

func TestAppPageStack(t *testing.T) {
	a := NewApp(testInfo)
	a.Push(textPage("home", "first page"))
	a.Push(textPage("detail", "second page"))

	s := screenOf(t, a, 80, 24)
	if !strings.Contains(s, "second page") || strings.Contains(s, "first page") {
		t.Fatalf("top page is not the visible one:\n%s", s)
	}
	if !strings.Contains(s, "<home> <detail>") {
		t.Fatalf("breadcrumbs missing:\n%s", s)
	}

	a.key(tcell.KeyEscape, 0)
	s = screenOf(t, a, 80, 24)
	if !strings.Contains(s, "first page") || strings.Contains(s, "second page") {
		t.Fatalf("esc did not go back:\n%s", s)
	}

	a.key(tcell.KeyEscape, 0) // the last page stays
	if len(a.stack) != 1 || !strings.Contains(screenOf(t, a, 80, 24), "first page") {
		t.Fatal("esc removed the root page")
	}
}

func TestAppLayoutSurvivesResize(t *testing.T) {
	a := NewApp(testInfo)
	a.Push(textPage("home", "body"))
	a.Flash(flashInfo, "ready")
	for _, size := range [][2]int{{80, 24}, {200, 60}, {40, 10}, {20, 5}, {1, 1}} {
		lines := render(t, a.root, size[0], size[1])
		if len(lines) != size[1] {
			t.Fatalf("%v: got %d lines", size, len(lines))
		}
		if size[0] >= 40 && size[1] >= 10 {
			if !strings.Contains(lines[0], "Context") {
				t.Fatalf("%v: header not on the first line: %q", size, lines[0])
			}
			if last := lines[len(lines)-1]; !strings.Contains(last, "<home>") || !strings.Contains(last, "ready") {
				t.Fatalf("%v: footer not on the last line: %q", size, last)
			}
		}
	}
}

func TestAppFlashClearsOnNextKey(t *testing.T) {
	a := NewApp(testInfo)
	a.Push(textPage("home", "body"))
	a.Flash(flashError, "something failed")
	if !strings.Contains(screenOf(t, a, 80, 24), "something failed") {
		t.Fatal("flash not shown")
	}
	a.key(tcell.KeyRune, 'x')
	if strings.Contains(screenOf(t, a, 80, 24), "something failed") {
		t.Fatal("flash still shown after a key press")
	}
}

func TestAppCtrlCQuits(t *testing.T) {
	a := NewApp(testInfo)
	stopped := false
	a.stop = func() { stopped = true }
	a.Push(textPage("home", "body"))
	a.key(tcell.KeyCtrlC, 0)
	if !stopped {
		t.Fatal("ctrl-c did not stop the application")
	}
}

func TestAppPageBindingsWinOverGlobal(t *testing.T) {
	a := NewApp(testInfo)
	hit := ""
	p := textPage("home", "body")
	p.bindings = func() []binding {
		return []binding{runeBinding('x', "x", "Test", func() { hit = "page" })}
	}
	a.Push(p)
	a.key(tcell.KeyRune, 'x')
	if hit != "page" {
		t.Fatalf("page binding not called, hit=%q", hit)
	}
}
