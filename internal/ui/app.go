package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
	"github.com/ekosup/d8s/internal/store"
)

type flashLevel int

const (
	flashInfo flashLevel = iota
	flashWarn
	flashError
)

// page is one entry on the navigation stack.
type page struct {
	name string
	prim tview.Primitive
	// bindings returns the keys this page handles; they take precedence
	// over the global ones.
	bindings func() []binding
	// back runs on Esc before the page is popped. Returning true means the
	// key was used (for example to clear a filter) and the page stays.
	back func() bool
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

	info  docker.Info
	stack []*page
	stop  func()

	client    docker.Client
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
func WithClient(c docker.Client) Option { return func(a *App) { a.client = c } }

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
	for _, o := range opts {
		o(a)
	}

	footer := tview.NewFlex().
		AddItem(a.crumbs, 0, 1, false).
		AddItem(a.status, 0, 2, false)
	a.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header, 2, 0, false).
		AddItem(a.pages, 0, 1, true).
		AddItem(footer, 1, 0, false)

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
	a.stack = append(a.stack, p)
	a.pages.AddPage(a.pageID(len(a.stack)-1), p.prim, true, true)
	a.tv.SetFocus(p.prim)
	a.drawCrumbs()
}

// Pop removes the top page. The root page is never removed.
func (a *App) Pop() bool {
	if len(a.stack) <= 1 {
		return false
	}
	last := len(a.stack) - 1
	a.pages.RemovePage(a.pageID(last))
	a.stack = a.stack[:last]
	top := a.stack[last-1]
	a.pages.SwitchToPage(a.pageID(last - 1))
	a.tv.SetFocus(top.prim)
	a.drawCrumbs()
	return true
}

func (a *App) pageID(i int) string { return fmt.Sprintf("page-%d", i) }

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
	a.clearFlash()
	if p := a.top(); p != nil && p.bindings != nil {
		for _, b := range p.bindings() {
			if b.matches(ev) {
				b.do()
				return nil
			}
		}
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

	hints := make([]string, 0, 4)
	for _, b := range a.globalBindings() {
		hints = append(hints, fmt.Sprintf("[steelblue]<%s>[gray] %s", b.label, strings.ToLower(b.desc)))
	}
	a.header.SetText(line1 + "\n " + strings.Join(hints, "  "))
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
