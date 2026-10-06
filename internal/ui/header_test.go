package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
)

// headerLines renders the app and returns only the header's rows.
func headerLines(t *testing.T, a *App, width int) []string {
	t.Helper()
	return render(t, a.root, width, 30)[:headerHeight]
}

// hintAt finds a key hint such as "<ctrl-d> Delete" in the header and
// returns its row and column, or -1, -1.
func hintAt(lines []string, key, desc string) (row, col int) {
	re := regexp.MustCompile(regexp.QuoteMeta("<"+key+">") + `\s+` + regexp.QuoteMeta(desc) + `(\s|$)`)
	for i, l := range lines {
		if loc := re.FindStringIndex(l); loc != nil {
			return i, len([]rune(l[:loc[0]]))
		}
	}
	return -1, -1
}

func TestHeaderStacksConnectionInfoInTheFirstColumn(t *testing.T) {
	a := NewApp(docker.Info{Context: "prod-swarm", ServerVersion: "27.5.1", APIVersion: "1.47",
		Swarm: docker.SwarmInfo{Active: true, Manager: true, Leader: true}})
	a.Push(textPage("home", "body"))
	lines := headerLines(t, a, 120)
	for i, want := range []string{"Context: prod-swarm", "Engine:  27.5.1", "API:     1.47", "Swarm:   manager (leader)"} {
		if !strings.HasPrefix(strings.TrimLeft(lines[i], " "), want) {
			t.Fatalf("header row %d = %q, want it to start with %q", i, lines[i], want)
		}
	}
}

func TestHeaderOmitsSwarmOnAStandaloneEngine(t *testing.T) {
	a := NewApp(testInfo)
	a.Push(textPage("home", "body"))
	if joined := strings.Join(headerLines(t, a, 120), "\n"); strings.Contains(joined, "Swarm") {
		t.Fatalf("header mentions swarm:\n%s", joined)
	}
}

func TestHeaderGeneralKeysFormOneAlignedColumn(t *testing.T) {
	a := NewApp(testInfo)
	a.Push(textPage("home", "body"))
	lines := headerLines(t, a, 120)

	globals := a.globalBindings()
	if len(globals) > headerHeight {
		t.Fatalf("%d general keys do not fit %d header rows", len(globals), headerHeight)
	}
	col := -1
	for i, b := range globals {
		row, c := hintAt(lines, b.label, b.desc)
		if row != i {
			t.Fatalf("<%s> %s is on row %d, want %d:\n%s", b.label, b.desc, row, i, strings.Join(lines, "\n"))
		}
		if col >= 0 && c != col {
			t.Fatalf("<%s> starts at column %d, the others at %d", b.label, c, col)
		}
		col = c
	}
	// It sits to the right of the connection info.
	if info := strings.Index(lines[0], "Context"); col <= info {
		t.Fatalf("general keys at column %d are not right of the info at %d", col, info)
	}
	// Descriptions line up too: the keys are padded to one width.
	descCol := -1
	for i, b := range globals {
		c := strings.Index(lines[i], b.desc)
		if descCol >= 0 && c != descCol {
			t.Fatalf("description of <%s> starts at %d, the others at %d:\n%s", b.label, c, descCol, strings.Join(lines, "\n"))
		}
		descCol = c
	}
}

func TestHeaderViewKeysFillFurtherColumns(t *testing.T) {
	h := actionHarness(t)
	lines := headerLines(t, h.app, 120)
	hints := h.app.top().hints()
	if len(hints) <= headerHeight {
		t.Fatalf("only %d view keys; the test needs more than one column's worth", len(hints))
	}

	_, generalCol := hintAt(lines, ":", "Command mode")
	cols := map[int]int{}
	for i, b := range hints {
		row, col := hintAt(lines, b.label, b.desc)
		if row < 0 {
			t.Fatalf("<%s> %s missing from the header:\n%s", b.label, b.desc, strings.Join(lines, "\n"))
		}
		if row != i%headerHeight {
			t.Fatalf("<%s> is on row %d, want %d (keys fill a column top to bottom)", b.label, row, i%headerHeight)
		}
		if col <= generalCol {
			t.Fatalf("<%s> at column %d overlaps the general keys at %d", b.label, col, generalCol)
		}
		cols[col]++
	}
	if want := (len(hints) + headerHeight - 1) / headerHeight; len(cols) != want {
		t.Fatalf("view keys are in %d columns, want %d:\n%s", len(cols), want, strings.Join(lines, "\n"))
	}
}

func TestHeaderFollowsTheTopPage(t *testing.T) {
	h := actionHarness(t)
	if row, _ := hintAt(headerLines(t, h.app, 120), "ctrl-d", "Delete"); row < 0 {
		t.Fatal("container keys not shown on the container view")
	}
	h.app.key(tcell.KeyRune, '?')
	if row, _ := hintAt(headerLines(t, h.app, 120), "ctrl-d", "Delete"); row >= 0 {
		t.Fatal("container keys still shown while help is on top")
	}
	h.app.key(tcell.KeyEscape, 0)
	if row, _ := hintAt(headerLines(t, h.app, 120), "ctrl-d", "Delete"); row < 0 {
		t.Fatal("container keys not restored after closing help")
	}
}

func TestHeaderShowsEveryKeyOfABusyView(t *testing.T) {
	a := NewApp(testInfo)
	var many []binding
	for i := range 3 * headerHeight {
		many = append(many, runeBinding(rune('a'+i), string(rune('a'+i)), fmt.Sprintf("Action%02d", i), func() {}))
	}
	p := textPage("busy", "body")
	p.hints = func() []binding { return many }
	a.Push(p)
	lines := headerLines(t, a, 160)
	for _, b := range many {
		if row, _ := hintAt(lines, b.label, b.desc); row < 0 {
			t.Fatalf("<%s> %s missing:\n%s", b.label, b.desc, strings.Join(lines, "\n"))
		}
	}
}

func TestHeaderFitsAnEightyColumnTerminal(t *testing.T) {
	h := actionHarness(t)
	lines := headerLines(t, h.app, 80)
	for _, b := range append(h.app.globalBindings(), h.app.top().hints()...) {
		if !strings.Contains(strings.Join(lines, "\n"), "<"+b.label+">") {
			t.Fatalf("key <%s> pushed off an 80-column header:\n%s", b.label, strings.Join(lines, "\n"))
		}
	}
}

func TestLogHeaderSummarisesRangeKeys(t *testing.T) {
	h := logHarness(t, "hello")
	h.openLogs()
	lines := headerLines(t, h.app, 120)
	if row, _ := hintAt(lines, "0-5", "Range"); row < 0 {
		t.Fatalf("no summary for the range keys:\n%s", strings.Join(lines, "\n"))
	}
	if row, _ := hintAt(lines, "3", "Last 15m"); row >= 0 {
		t.Fatal("each range key still has its own header entry")
	}
	// The individual keys still work and are still listed in help.
	h.app.key(tcell.KeyRune, '?')
	if s := h.screen(); !strings.Contains(s, "Last 15m") {
		t.Fatalf("range keys missing from help:\n%s", s)
	}
}
