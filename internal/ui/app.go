package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/action"
	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
	"github.com/ekosup/d8s/internal/store"
)

// headerHeight is one line each for connection, global keys and view keys.
const headerHeight = 3

type flashLevel int

const (
	flashInfo flashLevel = iota
	flashWarn
	flashError
)

// page is one entry on the navigation stack.
type page struct {
	id   string // unique within the app, assigned by Push
	name string
	prim tview.Primitive
	// bindings returns the keys this page handles; they take precedence
	// over the global ones.
	bindings func() []binding
	// back runs on Esc before the page is popped. Returning true means the
	// key was used (for example to clear a filter) and the page stays.
	back func() bool
	// table is set when the page is a table; help then lists its navigation keys.
	table *tableView
	// filter receives what is typed at the `/` prompt, live.
	filter filterer
	// onClose runs when the page leaves the stack, to stop what feeds it.
	onClose func()
	// hints are the bindings worth advertising in the header.
	hints func() []binding
	// modal pages take every key: nothing reaches the pages below or the
	// global bindings, except Esc (close) and Ctrl-C (quit).
	modal bool
}

// filterer is anything the `/` prompt can narrow down or search.
type filterer interface {
	Filter() string
	SetFilter(text string)
}

// App is the application shell: header, a stack of pages, and a footer with
// breadcrumbs and status messages.
type App struct {
	tv     *tview.Application
	root   *tview.Flex
	header *tview.TextView
	pages  *tview.Pages
	crumbs *tview.TextView
	status *tview.TextView
	footer *tview.Pages // "status" or "prompt"
	prompt *tview.InputField
	hint   *tview.TextView

	prompting promptMode

	info       docker.Info
	stack      []*page
	nextPageID int
	stop       func()

	logs      *logView // the log page currently open, if any
	logBuffer int      // lines a log page keeps

	// Seams for the shell: how to leave the TUI and reach the terminal.
	suspend      func(f func()) bool
	openTerminal func() (terminal, error)
	resizePoll   time.Duration

	// Seams for the pager's side effects.
	dumpDir string            // where ctrl-s saves
	now     func() time.Time  // names saved files
	copy    func(data []byte) // puts text on the clipboard
	screen  tcell.Screen      // the live screen, once drawing has started

	client    docker.Client
	executor  *action.Executor
	registry  *resource.Registry
	watchOpts store.Options
	// queue runs f on the UI goroutine. Watchers use it to hand over results.
	queue func(f func())

	// The resource view currently shown, and the watcher feeding it. gen
	// identifies the view so that late results of a closed one are dropped.
	view        *tableView
	gen         int
	cancelWatch context.CancelFunc
	watchDone   <-chan struct{}
	stale       bool
}

// Option configures an App.
type Option func(*App)

// WithClient sets the Docker client resource views read from.
func WithClient(c docker.Client) Option {
	return func(a *App) {
		a.client = c
		a.executor = action.New(c, false)
	}
}

// WithRegistry sets the resources reachable from the application.
func WithRegistry(r *resource.Registry) Option { return func(a *App) { a.registry = r } }

// WithWatchOptions tunes how views are refreshed.
func WithWatchOptions(o store.Options) Option { return func(a *App) { a.watchOpts = o } }

// NewApp builds the shell for the daemon described by info.
func NewApp(info docker.Info, opts ...Option) *App {
	a := &App{
		tv:     tview.NewApplication(),
		header: tview.NewTextView().SetDynamicColors(true),
		pages:  tview.NewPages(),
		crumbs: tview.NewTextView().SetDynamicColors(true),
		status: tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight),
		info:   info,
	}
	a.stop = a.tv.Stop
	a.queue = func(f func()) { a.tv.QueueUpdateDraw(f) }
	a.now = time.Now
	a.logBuffer = defaultLogBuffer
	a.suspend = a.tv.Suspend
	a.openTerminal = openTTY
	a.resizePoll = 250 * time.Millisecond
	a.dumpDir = defaultDumpDir()
	a.tv.SetBeforeDrawFunc(func(s tcell.Screen) bool {
		a.screen = s
		return false
	})
	a.copy = func(data []byte) {
		if a.screen != nil {
			a.screen.SetClipboard(data) // OSC 52; the terminal decides whether to honour it
		}
	}
	for _, o := range opts {
		o(a)
	}

	a.buildPrompt()
	a.footer = tview.NewPages().
		AddPage(footerPrompt, tview.NewFlex().
			AddItem(a.prompt, 0, 1, true).
			AddItem(a.hint, 0, 2, false), true, false).
		AddPage(footerStatus, tview.NewFlex().
			AddItem(a.crumbs, 0, 1, false).
			AddItem(a.status, 0, 2, false), true, true)
	a.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header, headerHeight, 0, false).
		AddItem(a.pages, 0, 1, true).
		AddItem(a.footer, 1, 0, false)

	a.tv.SetRoot(a.root, true)
	a.tv.SetInputCapture(a.handleKey)
	a.drawHeader()
	return a
}

// Run starts the event loop and blocks until the application stops. The
// terminal is restored on return, including after a panic.
func (a *App) Run() error {
	defer a.stopWatch()
	return a.tv.Run()
}

// Push shows p on top of the current page.
func (a *App) Push(p *page) {
	a.nextPageID++
	p.id = fmt.Sprintf("page-%d", a.nextPageID)
	a.stack = append(a.stack, p)
	a.pages.AddPage(p.id, p.prim, true, true)
	a.tv.SetFocus(p.prim)
	a.drawCrumbs()
	a.drawHeader()
}

// Pop removes the top page. The root page is never removed.
func (a *App) Pop() bool {
	if len(a.stack) <= 1 {
		return false
	}
	last := len(a.stack) - 1
	closing := a.stack[last]
	a.pages.RemovePage(closing.id)
	a.stack = a.stack[:last]
	if closing.onClose != nil {
		closing.onClose()
	}
	a.tv.SetFocus(a.stack[last-1].prim)
	a.drawCrumbs()
	a.drawHeader()
	return true
}

func (a *App) top() *page {
	if len(a.stack) == 0 {
		return nil
	}
	return a.stack[len(a.stack)-1]
}

// Flash shows a message on the status line until the next key press.
func (a *App) Flash(level flashLevel, msg string) {
	color := "white"
	switch level {
	case flashWarn:
		color = "yellow"
	case flashError:
		color = "orangered"
	}
	a.status.SetText(fmt.Sprintf("[%s]%s ", color, tview.Escape(msg)))
}

func (a *App) clearFlash() { a.status.SetText("") }

// globalBindings are available on every page.
func (a *App) globalBindings() []binding {
	return []binding{
		runeBinding(':', ":", "Command mode", func() { a.openPrompt(promptCommand) }),
		runeBinding('/', "/", "Filter", func() { a.openPrompt(promptFilter) }),
		runeBinding('?', "?", "Help", a.showHelp),
		keyBinding(tcell.KeyEscape, "esc", "Back", a.back),
		keyBinding(tcell.KeyCtrlC, "ctrl-c", "Quit", func() { a.stop() }),
	}
}

func (a *App) back() {
	if p := a.top(); p != nil && p.back != nil && p.back() {
		return
	}
	a.Pop()
}

// handleKey is the application-wide input capture. It returns nil when the
// key was consumed, or the event to pass on to the focused primitive.
func (a *App) handleKey(ev *tcell.EventKey) *tcell.EventKey {
	if a.prompting != promptNone {
		// Everything typed belongs to the prompt, except the way out.
		if ev.Key() == tcell.KeyCtrlC {
			a.stop()
			return nil
		}
		return ev
	}
	a.clearFlash()
	p := a.top()
	if p != nil && p.bindings != nil {
		for _, b := range p.bindings() {
			if b.matches(ev) {
				b.do()
				return nil
			}
		}
	}
	if p != nil && p.modal {
		switch ev.Key() {
		case tcell.KeyEscape:
			a.Pop()
		case tcell.KeyCtrlC:
			a.stop()
		}
		return nil
	}
	for _, b := range a.globalBindings() {
		if b.matches(ev) {
			b.do()
			return nil
		}
	}
	return ev
}

func (a *App) drawHeader() {
	field := func(k, v string) string {
		return fmt.Sprintf("[aqua]%s:[white] %s", k, tview.Escape(v))
	}
	line1 := " " + strings.Join([]string{
		field("Context", a.info.Context),
		field("Engine", a.info.ServerVersion),
		field("API", a.info.APIVersion),
	}, "   ")

	hintLine := func(bs []binding) string {
		parts := make([]string, 0, len(bs))
		for _, b := range bs {
			parts = append(parts, fmt.Sprintf("[steelblue]<%s>[gray] %s", b.label, strings.ToLower(b.desc)))
		}
		return " " + strings.Join(parts, "  ")
	}
	view := ""
	if p := a.top(); p != nil && p.hints != nil {
		view = hintLine(p.hints())
	}
	a.header.SetText(line1 + "\n" + hintLine(a.globalBindings()) + "\n" + view)
}

func (a *App) drawCrumbs() {
	parts := make([]string, len(a.stack))
	for i, p := range a.stack {
		color := "gray"
		if i == len(a.stack)-1 {
			color = "aqua"
		}
		parts[i] = fmt.Sprintf("[%s]<%s>", color, tview.Escape(p.name))
	}
	a.crumbs.SetText(" " + strings.Join(parts, " "))
}

// defaultDumpDir follows the XDG state directory convention.
func defaultDumpDir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "d8s", "dumps")
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "d8s", "dumps")
}
