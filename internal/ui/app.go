package ui

import (
	"context"
	"fmt"
	"log/slog"
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

// headerHeight is the number of rows of the header, and so the number of
// keys one header column holds.
const headerHeight = 6

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
	// pause and resume run when another page covers this one and when it
	// is uncovered again, so only the visible view is kept fresh.
	pause, resume func()
	// hints are the bindings worth advertising in the header.
	hints func() []binding
	// modal pages take every key: nothing reaches the pages below or the
	// global bindings, except Esc (close) and Ctrl-C (quit).
	modal bool
	// typing marks a modal page with a text field: keys it does not bind
	// go to the field instead of being dropped.
	typing bool
}

// filterer is anything the `/` prompt can narrow down or search.
type filterer interface {
	Filter() string
	SetFilter(text string)
}

// App is the application shell: header, a stack of pages, and a footer with
// breadcrumbs and status messages.
type App struct {
	tv        *tview.Application
	root      *tview.Flex
	header    *tview.Flex // columns: connection info, general keys, view keys
	pages     *tview.Pages
	crumbs    *tview.TextView
	status    *tview.TextView
	statusBar *tview.Flex
	footer    *tview.Pages // "status" or "prompt"
	prompt    *tview.InputField
	hint      *tview.TextView

	prompting promptMode

	info       docker.Info
	stack      []*page
	nextPageID int
	stop       func()

	events       *store.EventLog // recent daemon events of the current connection
	cancelEvents context.CancelFunc

	logs         *logView // the log page currently open, if any
	logBuffer    int      // lines a log page keeps
	logTail      int      // lines fetched when a log page opens
	shellCommand []string // what `s` runs in a container
	log          *slog.Logger

	// Seams for the shell: how to leave the TUI and reach the terminal.
	suspend      func(f func()) bool
	openTerminal func() (terminal, error)
	resizePoll   time.Duration

	// Seams for the pager's side effects.
	dumpDir string            // where ctrl-s saves
	now     func() time.Time  // names saved files
	copy    func(data []byte) // puts text on the clipboard
	screen  tcell.Screen      // the live screen, once drawing has started

	views     map[string][]string // user-chosen columns per view
	sorts     SortStore           // remembered sort per view; may be nil
	aliases   map[string]string   // user-defined commands: name -> "view" or "view /filter"
	hotkeys   []binding           // user-defined keys that run a command
	client    docker.Client
	policyFor func(context string) Policy // what the user decided per context
	policy    Policy                      // the one in force for the current connection
	connect   Connector
	contexts  func() ([]docker.Endpoint, error) // known Docker contexts, for offering a switch
	input     *tview.InputField                 // the field of the dialog that is open, if any
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
		header: tview.NewFlex(),
		pages:  tview.NewPages(),
		crumbs: tview.NewTextView().SetDynamicColors(true),
		status: tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight),
		info:   info,
	}
	a.stop = a.tv.Stop
	a.queue = func(f func()) { a.tv.QueueUpdateDraw(f) }
	a.now = time.Now
	a.logBuffer = defaultLogBuffer
	a.logTail = defaultLogTail
	a.shellCommand = shellCommand
	a.log = slog.New(slog.DiscardHandler)
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
	a.applyPolicy()
	a.startEventLog()

	a.buildPrompt()
	// Breadcrumbs take the width they need; messages get everything else.
	a.statusBar = tview.NewFlex().
		AddItem(a.crumbs, 1, 0, false).
		AddItem(a.status, 0, 1, false)
	a.footer = tview.NewPages().
		AddPage(footerPrompt, tview.NewFlex().
			AddItem(a.prompt, 0, 1, true).
			AddItem(a.hint, 0, 2, false), true, false).
		AddPage(footerStatus, a.statusBar, true, true)
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
	if cur := a.top(); cur != nil && cur.pause != nil && !p.modal {
		cur.pause()
	}
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
	if top := a.stack[last-1]; top.resume != nil && !closing.modal {
		top.resume()
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
	tag := theme.tagInfo
	switch level {
	case flashWarn:
		tag = theme.tagWarn
	case flashError:
		tag = theme.tagError
	}
	a.status.SetText(tag + tview.Escape(msg) + "[-:-:-] ")
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
			return nil
		case tcell.KeyCtrlC:
			a.stop()
			return nil
		}
		if p.typing {
			return ev
		}
		return nil
	}
	for _, b := range append(a.globalBindings(), a.hotkeys...) {
		if b.matches(ev) {
			b.do()
			return nil
		}
	}
	return ev
}

// drawHeader rebuilds the header: connection info in the first column, the
// keys that work everywhere in the second, and the keys of the page on top
// in as many further columns as they need.
func (a *App) drawHeader() {
	a.header.Clear()

	contextName := a.info.Context
	if a.policy.Production {
		contextName += " (production)"
	}
	info := [][2]string{
		{"Context", contextName},
		{"Engine", a.info.ServerVersion},
		{"API", a.info.APIVersion},
	}
	switch sw := a.info.Swarm; {
	case sw.Manager && sw.Leader:
		info = append(info, [2]string{"Swarm", "manager (leader)"})
	case sw.Manager:
		info = append(info, [2]string{"Swarm", "manager"})
	case sw.Active:
		info = append(info, [2]string{"Swarm", "worker"})
	}
	if a.policy.ReadOnly {
		info = append(info, [2]string{"Mode", "READ-ONLY"})
	}
	if a.stale {
		info = append(info, [2]string{"Daemon", "DISCONNECTED"})
	}
	labelWidth, infoWidth := 0, 0
	for _, f := range info {
		labelWidth = max(labelWidth, len(f[0])+1)
	}
	var sb strings.Builder
	for _, f := range info {
		label := f[0] + ":"
		fmt.Fprintf(&sb, " %s%s[-:-:-]%s %s%s[-:-:-]\n", theme.tagLabel, label, strings.Repeat(" ", labelWidth-len(label)), theme.tagValue, tview.Escape(f[1]))
		infoWidth = max(infoWidth, labelWidth+1+len([]rune(f[1])))
	}
	// The info column takes what it needs, within reason; keys share the rest.
	a.header.AddItem(headerColumn(sb.String()), min(infoWidth+4, maxInfoWidth), 0, false)

	columns := [][]binding{a.globalBindings()}
	if p := a.top(); p != nil && p.hints != nil {
		hints := make([]binding, 0, 2*headerHeight)
		for _, b := range p.hints() {
			if !b.noHint {
				hints = append(hints, b)
			}
		}
		for len(hints) > 0 {
			n := min(len(hints), headerHeight)
			columns = append(columns, hints[:n])
			hints = hints[n:]
		}
	}
	for _, col := range columns {
		text, width := hintColumn(col)
		// Width is shared in proportion to what each column has to show.
		a.header.AddItem(headerColumn(text), 0, width, false)
	}
}

const maxInfoWidth = 44

func headerColumn(text string) *tview.TextView {
	return tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetText(text)
}

// hintColumn renders bindings one per row with the keys padded to equal
// width, and returns the text together with its widest row.
func hintColumn(bs []binding) (text string, width int) {
	keyWidth := 0
	for _, b := range bs {
		keyWidth = max(keyWidth, len([]rune(b.label))+2)
	}
	var sb strings.Builder
	for _, b := range bs {
		key := "<" + b.label + ">"
		pad := strings.Repeat(" ", keyWidth-len([]rune(key)))
		fmt.Fprintf(&sb, "%s%s[-:-:-]%s %s\n", theme.tagKey, tview.Escape(key), pad, tview.Escape(b.desc))
		width = max(width, keyWidth+1+len([]rune(b.desc)))
	}
	return sb.String(), width + 2
}

func (a *App) drawCrumbs() {
	parts := make([]string, len(a.stack))
	for i, p := range a.stack {
		tag := theme.tagMuted
		if i == len(a.stack)-1 {
			tag = theme.tagCurrent
		}
		parts[i] = fmt.Sprintf("%s<%s>[-:-:-]", tag, tview.Escape(p.name))
	}
	text := " " + strings.Join(parts, " ")
	a.crumbs.SetText(text)
	a.statusBar.ResizeItem(a.crumbs, tview.TaggedStringWidth(text)+1, 0)
}

// StateDir is where d8s keeps what it writes on its own: saved logs and
// remembered view settings. It follows the XDG state directory convention.
func StateDir(getenv func(string) string) string {
	base := getenv("XDG_STATE_HOME")
	if base == "" {
		home := getenv("HOME")
		if home == "" {
			return filepath.Join(os.TempDir(), "d8s")
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "d8s")
}

func defaultDumpDir() string {
	return filepath.Join(StateDir(os.Getenv), "dumps")
}
