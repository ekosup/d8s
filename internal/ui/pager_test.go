package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func numbered(n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %03d", i)
	}
	lines[40] = "line 040 Mounts here"
	lines[80] = "line 080 mounts again"
	return lines
}

func pagerApp(t *testing.T, lines []string) (*App, *pager) {
	t.Helper()
	a := NewApp(testInfo)
	a.Push(textPage("home", "body"))
	p := newPager("Inspect: web", 0)
	p.SetLines(lines)
	a.pushPager("inspect", p, nil)
	return a, p
}

func TestPagerShowsTextAndTitle(t *testing.T) {
	a, _ := pagerApp(t, numbered(100))
	s := screenOf(t, a, 80, 24)
	for _, want := range []string{"Inspect: web", "line 000", "line 010", "<home> <inspect>"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
}

func TestPagerSearchJumpsToMatches(t *testing.T) {
	a, p := pagerApp(t, numbered(100))
	a.key(tcell.KeyRune, '/')
	a.typeText("mounts")
	a.key(tcell.KeyEnter, 0)
	s := screenOf(t, a, 80, 24)
	if !strings.Contains(s, "line 040 Mounts here") || !strings.Contains(s, "[1/2]") || !strings.Contains(s, "</mounts>") {
		t.Fatalf("first match not shown:\n%s", s)
	}

	a.key(tcell.KeyRune, 'n')
	if s = screenOf(t, a, 80, 24); !strings.Contains(s, "line 080 mounts again") || !strings.Contains(s, "[2/2]") {
		t.Fatalf("n did not jump to the second match:\n%s", s)
	}
	a.key(tcell.KeyRune, 'n') // wraps around
	if s = screenOf(t, a, 80, 24); !strings.Contains(s, "[1/2]") {
		t.Fatalf("n did not wrap:\n%s", s)
	}
	a.key(tcell.KeyRune, 'N')
	if s = screenOf(t, a, 80, 24); !strings.Contains(s, "[2/2]") {
		t.Fatalf("N did not go back:\n%s", s)
	}

	// Esc clears the search first, then closes the pager.
	a.key(tcell.KeyEscape, 0)
	if p.Filter() != "" || len(a.stack) != 2 {
		t.Fatalf("first esc: filter %q, stack %d", p.Filter(), len(a.stack))
	}
	a.key(tcell.KeyEscape, 0)
	if len(a.stack) != 1 {
		t.Fatal("second esc did not close the pager")
	}
}

func TestPagerSearchWithNoMatch(t *testing.T) {
	a, _ := pagerApp(t, numbered(100))
	a.key(tcell.KeyRune, '/')
	a.typeText("zzz")
	a.key(tcell.KeyEnter, 0)
	if s := screenOf(t, a, 80, 24); !strings.Contains(s, "[0/0]") {
		t.Fatalf("no-match state not shown:\n%s", s)
	}
	a.key(tcell.KeyRune, 'n') // must not panic
}

func TestPagerEscapesMarkup(t *testing.T) {
	a, _ := pagerApp(t, []string{`"Cmd": ["nginx", "-g", "daemon off;"]`, "[red]not a colour[-]"})
	s := screenOf(t, a, 80, 24)
	if !strings.Contains(s, `["nginx", "-g", "daemon off;"]`) || !strings.Contains(s, "[red]not a colour[-]") {
		t.Fatalf("brackets were swallowed as markup:\n%s", s)
	}
}

func TestPagerWrapToggle(t *testing.T) {
	long := strings.Repeat("x", 70) + " TAIL"
	a, _ := pagerApp(t, []string{long})
	if strings.Contains(screenOf(t, a, 40, 12), "TAIL") {
		t.Fatal("long line wrapped by default")
	}
	a.key(tcell.KeyRune, 'w')
	if !strings.Contains(screenOf(t, a, 40, 12), "TAIL") {
		t.Fatal("w did not wrap the line")
	}
}

func TestPagerSaveAndCopy(t *testing.T) {
	a, _ := pagerApp(t, []string{"alpha", "beta"})
	dir := t.TempDir()
	a.dumpDir = dir
	a.now = func() time.Time { return time.Date(2026, 10, 6, 12, 30, 0, 0, time.UTC) }
	var copied string
	a.copy = func(b []byte) { copied = string(b) }

	a.key(tcell.KeyCtrlS, 0)
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(files) != 1 {
		t.Fatalf("saved files: %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil || string(data) != "alpha\nbeta\n" {
		t.Fatalf("file content %q, err %v", data, err)
	}
	if s := screenOf(t, a, 120, 24); !strings.Contains(s, filepath.Base(files[0])) {
		t.Fatalf("saved path not reported:\n%s", s)
	}

	a.key(tcell.KeyRune, 'c')
	if copied != "alpha\nbeta\n" {
		t.Fatalf("copied %q", copied)
	}
}

func TestPagerKeepsOnlyMaxLines(t *testing.T) {
	p := newPager("Logs", 50)
	p.SetLines(numbered(100))
	p.Append([]string{"newest"})
	if got := len(p.lines); got != 50 {
		t.Fatalf("kept %d lines", got)
	}
	if p.lines[len(p.lines)-1] != "newest" || p.lines[0] != "line 051" {
		t.Fatalf("wrong lines kept: first %q last %q", p.lines[0], p.lines[len(p.lines)-1])
	}
}
