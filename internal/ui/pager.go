package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// pager shows text that can be scrolled, searched, wrapped, copied and
// saved. Inspect output and logs are both pagers.
type pager struct {
	*tview.TextView
	title string
	// saveName names saved files; empty means derive it from the title.
	saveName string
	max      int // lines kept; 0 = unlimited
	lines    []string

	wrap    bool
	follow  bool // stay at the end as lines arrive
	query   string
	re      *regexp.Regexp
	matches int
	current int
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func newPager(title string, maxLines int) *pager {
	p := &pager{TextView: tview.NewTextView(), title: title, max: maxLines}
	p.SetDynamicColors(true).SetRegions(true).SetWrap(false)
	p.SetBorder(true)
	if maxLines > 0 {
		p.SetMaxLines(maxLines)
	}
	p.drawTitle()
	return p
}

// SetLines replaces the content.
func (p *pager) SetLines(lines []string) {
	p.lines = p.trim(append([]string(nil), lines...))
	p.render()
}

// Append adds lines at the end, dropping the oldest beyond the limit.
func (p *pager) Append(lines []string) {
	p.lines = p.trim(append(p.lines, lines...))
	if p.query != "" {
		p.render() // match numbering changes
		return
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(plainLine(l))
		sb.WriteByte('\n')
	}
	_, _ = p.Write([]byte(sb.String()))
	if p.follow {
		p.ScrollToEnd()
	}
}

func (p *pager) trim(lines []string) []string {
	if p.max > 0 && len(lines) > p.max {
		return lines[len(lines)-p.max:]
	}
	return lines
}

// Content is the text as it would be saved or copied.
func (p *pager) Content() string {
	if len(p.lines) == 0 {
		return ""
	}
	return strings.Join(p.lines, "\n") + "\n"
}

// Filter implements filterer; for a pager the filter is a search.
func (p *pager) Filter() string { return p.query }

// SetFilter searches for text and jumps to the first match.
func (p *pager) SetFilter(text string) {
	p.query, p.re, p.current = text, nil, 0
	if text != "" {
		re, err := regexp.Compile("(?i)" + text)
		if err != nil {
			re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(text))
		}
		p.re = re
	}
	p.render()
}

// step moves to the next (+1) or previous (-1) match, wrapping around.
func (p *pager) step(delta int) {
	if p.matches == 0 {
		return
	}
	p.current = (p.current + delta + p.matches) % p.matches
	p.showCurrent()
}

func (p *pager) toggleWrap() {
	p.wrap = !p.wrap
	p.SetWrap(p.wrap)
}

func (p *pager) render() {
	var sb strings.Builder
	p.matches = 0
	for _, l := range p.lines {
		if p.re == nil {
			sb.WriteString(plainLine(l))
		} else {
			p.writeHighlighted(&sb, ansiRe.ReplaceAllString(l, ""))
		}
		sb.WriteByte('\n')
	}
	p.SetText(sb.String())
	if p.current >= p.matches {
		p.current = 0
	}
	p.showCurrent()
	if p.re == nil && p.follow {
		p.ScrollToEnd()
	}
}

// writeHighlighted writes line with every match wrapped in its own region,
// so the view can scroll to match number n.
func (p *pager) writeHighlighted(sb *strings.Builder, line string) {
	last := 0
	for _, m := range p.re.FindAllStringIndex(line, -1) {
		if m[0] == m[1] {
			continue // an empty match would highlight nothing
		}
		sb.WriteString(tview.Escape(line[last:m[0]]))
		fmt.Fprintf(sb, `["m%d"]%s%s[-:-:-][""]`, p.matches, theme.tagMatch, tview.Escape(line[m[0]:m[1]]))
		p.matches++
		last = m[1]
	}
	sb.WriteString(tview.Escape(line[last:]))
}

func (p *pager) showCurrent() {
	if p.matches > 0 {
		p.Highlight(fmt.Sprintf("m%d", p.current)).ScrollToHighlight()
	} else {
		p.Highlight()
	}
	p.drawTitle()
}

func (p *pager) drawTitle() {
	t := " " + tview.Escape(p.title) + " "
	if p.query != "" {
		n := 0
		if p.matches > 0 {
			n = p.current + 1
		}
		t = fmt.Sprintf(" %s [%d/%d[] </%s> ", tview.Escape(p.title), n, p.matches, tview.Escape(p.query))
	}
	p.SetTitle(t)
}

// plainLine prepares a line for display without search: markup is shown
// literally, terminal colours are kept.
func plainLine(l string) string {
	return tview.TranslateANSI(tview.Escape(l))
}

// pushPager shows p as a page with the standard pager keys plus extra.
func (a *App) pushPager(name string, p *pager, onClose func(), extra ...binding) {
	keys := append([]binding{
		runeBinding('n', "n", "Next match", func() { p.step(1) }),
		runeBinding('N', "shift-n", "Previous match", func() { p.step(-1) }),
		runeBinding('w', "w", "Wrap", p.toggleWrap),
		runeBinding('c', "c", "Copy", func() { a.copyText(p.Content()) }),
		keyBinding(tcell.KeyCtrlS, "ctrl-s", "Save", func() {
			name := p.saveName
			if name == "" {
				name = p.title
			}
			a.saveText(name, p.Content())
		}),
	}, extra...)
	a.Push(&page{
		name:     name,
		prim:     p,
		bindings: func() []binding { return keys },
		hints:    func() []binding { return keys },
		filter:   p,
		onClose:  onClose,
		back: func() bool {
			if p.Filter() == "" {
				return false
			}
			p.SetFilter("")
			return true
		},
	})
}

func (a *App) copyText(text string) {
	a.copy([]byte(text))
	a.Flash(flashInfo, fmt.Sprintf("copied %d bytes to the clipboard", len(text)))
}

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// saveText writes text to a timestamped file in the dump directory.
func (a *App) saveText(title, text string) {
	name := strings.Trim(unsafeFileChars.ReplaceAllString(strings.ToLower(title), "-"), "-")
	path := filepath.Join(a.dumpDir, fmt.Sprintf("%s-%s.txt", name, a.now().Format("20060102-150405")))
	if err := os.MkdirAll(a.dumpDir, 0o750); err != nil {
		a.Flash(flashError, "save failed: "+err.Error())
		return
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		a.Flash(flashError, "save failed: "+err.Error())
		return
	}
	a.Flash(flashInfo, "saved "+path)
}
