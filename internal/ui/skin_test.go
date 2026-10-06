package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
)

// styledCell is one screen cell with the style it was drawn in.
type styledCell struct {
	r     rune
	style tcell.Style
}

// renderStyled draws p and returns the cells, row by row.
func renderStyled(t *testing.T, p tview.Primitive, w, h int) [][]styledCell {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	p.SetRect(0, 0, w, h)
	p.Draw(screen)
	screen.Show()
	cells, cw, ch := screen.GetContents()
	out := make([][]styledCell, ch)
	for y := range ch {
		out[y] = make([]styledCell, cw)
		for x := range cw {
			c := cells[y*cw+x]
			r := ' '
			if len(c.Runes) > 0 {
				r = c.Runes[0]
			}
			out[y][x] = styledCell{r: r, style: c.Style}
		}
	}
	return out
}

// styleOf returns the style of the first cell of text on screen.
func styleOf(t *testing.T, rows [][]styledCell, text string) tcell.Style {
	t.Helper()
	for _, row := range rows {
		var sb strings.Builder
		for _, c := range row {
			sb.WriteRune(c.r)
		}
		line := sb.String()
		if i := strings.Index(line, text); i >= 0 {
			return row[len([]rune(line[:i]))].style
		}
	}
	t.Fatalf("%q is not on screen", text)
	return tcell.StyleDefault
}

func useSkin(t *testing.T, name string) {
	t.Helper()
	if err := SetSkin(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = SetSkin("dark") })
}

func toneRows() []resource.Row {
	return []resource.Row{
		{ID: "1", Cells: []string{"aaa-normal"}, Tone: resource.ToneNormal},
		{ID: "2", Cells: []string{"bbb-good"}, Tone: resource.ToneGood},
		{ID: "3", Cells: []string{"ccc-warn"}, Tone: resource.ToneWarn},
		{ID: "4", Cells: []string{"ddd-bad"}, Tone: resource.ToneBad},
		{ID: "5", Cells: []string{"eee-muted"}, Tone: resource.ToneMuted},
		{ID: "6", Cells: []string{"fff-marked"}, Tone: resource.ToneNormal},
	}
}

func toneTable() *tableView {
	v := newTableView("Tones", []resource.Column{{Name: "NAME"}})
	v.SetRows(toneRows())
	v.marks["6"] = true
	v.refresh()
	v.Select(6, 0) // keep the highlight off the rows whose own style is under test
	return v
}

func TestSetSkinRejectsUnknownNames(t *testing.T) {
	if err := SetSkin("neon"); err == nil {
		t.Fatal("accepted an unknown skin")
	}
	for _, name := range []string{"dark", "light", "mono"} {
		if err := SetSkin(name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	_ = SetSkin("dark")
}

func TestMonoSkinUsesNoColour(t *testing.T) {
	useSkin(t, "mono")
	h := newHarness(t, fastWatch,
		docker.Container{ID: "1", Name: "web", State: "running", Status: "Up", Created: created},
		docker.Container{ID: "2", Name: "crashed", State: "exited", Status: "Exited (1)", Created: created},
	)
	h.show("containers")
	h.step()
	h.app.Flash(flashError, "something failed")
	for y, row := range renderStyled(t, h.app.root, 120, 24) {
		for x, c := range row {
			fg, bg, _ := c.style.Decompose()
			if fg != tcell.ColorDefault || bg != tcell.ColorDefault {
				t.Fatalf("cell (%d,%d) %q has colour fg=%v bg=%v in the mono skin", x, y, c.r, fg, bg)
			}
		}
	}
}

func TestMonoSkinKeepsStatusesApart(t *testing.T) {
	useSkin(t, "mono")
	rows := renderStyled(t, toneTable(), 60, 12)
	attrs := func(text string) tcell.AttrMask {
		_, _, a := styleOf(t, rows, text).Decompose()
		return a
	}
	normal, warn, bad, muted, marked := attrs("aaa-normal"), attrs("ccc-warn"), attrs("ddd-bad"), attrs("eee-muted"), attrs("fff-marked")
	seen := map[tcell.AttrMask]string{}
	for name, a := range map[string]tcell.AttrMask{"normal": normal, "warn": warn, "bad": bad, "muted": muted} {
		if other, dup := seen[a]; dup {
			t.Fatalf("%s and %s look the same without colour (attributes %v)", name, other, a)
		}
		seen[a] = name
	}
	if marked == normal {
		t.Fatal("a marked row looks like an unmarked one without colour")
	}
}

func TestMonoSkinHighlightsTheSelectedRow(t *testing.T) {
	useSkin(t, "mono")
	v := toneTable()
	v.Select(1, 0)
	_, _, a := styleOf(t, renderStyled(t, v, 60, 12), "aaa-normal").Decompose()
	if a&tcell.AttrReverse == 0 {
		t.Fatal("the selected row is not visibly selected without colour")
	}
}

func TestLightSkinAvoidsColoursThatVanishOnWhite(t *testing.T) {
	useSkin(t, "light")
	h := newHarness(t, fastWatch,
		docker.Container{ID: "1", Name: "web", State: "running", Status: "Up", Created: created},
		docker.Container{ID: "2", Name: "frozen", State: "paused", Status: "Up (Paused)", Created: created},
		docker.Container{ID: "3", Name: "crashed", State: "exited", Status: "Exited (1)", Created: created},
	)
	h.show("containers")
	h.step()
	h.app.Flash(flashWarn, "careful")
	pale := map[tcell.Color]string{tcell.ColorWhite: "white", tcell.ColorYellow: "yellow", tcell.ColorAqua: "aqua",
		tcell.ColorLightSkyBlue: "light sky blue", tcell.ColorLightGreen: "light green"}
	for y, row := range renderStyled(t, h.app.root, 120, 24) {
		for x, c := range row {
			fg, bg, _ := c.style.Decompose()
			if name, bad := pale[fg]; bad && bg == tcell.ColorDefault && c.r != ' ' {
				t.Fatalf("cell (%d,%d) %q is %s on the terminal background, unreadable on a light terminal", x, y, c.r, name)
			}
		}
	}
}

func TestSkinsDifferAndDarkIsTheDefault(t *testing.T) {
	bad := func() tcell.Color {
		fg, _, _ := styleOf(t, renderStyled(t, toneTable(), 60, 12), "ddd-bad").Decompose()
		return fg
	}
	dark := bad()
	if dark == tcell.ColorDefault {
		t.Fatal("the default skin should colour a failing row")
	}
	useSkin(t, "light")
	if light := bad(); light == tcell.ColorDefault {
		t.Fatal("the light skin should colour a failing row too")
	}
}

func TestSkinFor(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		configured string
		env        map[string]string
		want       string
	}{
		{"dark", nil, "dark"},
		{"light", nil, "light"},
		{"light", map[string]string{"NO_COLOR": "1"}, "mono"}, // the convention wins over the file
		{"dark", map[string]string{"NO_COLOR": ""}, "dark"},   // set but empty does not count
		{"dark", map[string]string{"TERM": "dumb"}, "mono"},
	}
	for _, tt := range tests {
		if got := SkinFor(tt.configured, env(tt.env)); got != tt.want {
			t.Errorf("SkinFor(%q, %v) = %q, want %q", tt.configured, tt.env, got, tt.want)
		}
	}
}
