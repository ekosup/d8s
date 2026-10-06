package ui

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ekosup/d8s/internal/action"
	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/store"
)

// connectTimeout is generous because an ssh endpoint has to log in first.
const connectTimeout = 40 * time.Second

// Connector opens a client for an endpoint and identifies the daemon.
type Connector func(ctx context.Context, ep docker.Endpoint) (docker.Client, docker.Info, error)

// WithConnector lets the application switch daemons at run time.
func WithConnector(c Connector) Option { return func(a *App) { a.connect = c } }

// WithContexts tells the application which Docker contexts exist, so it can
// offer to switch to the one that reaches a particular node.
func WithContexts(list func() ([]docker.Endpoint, error)) Option {
	return func(a *App) { a.contexts = list }
}

// contextNamed finds a known context by name.
func (a *App) contextNamed(name string) (docker.Endpoint, bool) {
	if a.contexts == nil || name == "" {
		return docker.Endpoint{}, false
	}
	eps, err := a.contexts()
	if err != nil {
		return docker.Endpoint{}, false
	}
	for _, ep := range eps {
		if ep.Context == name {
			return ep, true
		}
	}
	return docker.Endpoint{}, false
}

// WithLogSettings sets how many lines a log page keeps and how many it
// fetches when it opens. Values below one keep the defaults.
func WithLogSettings(buffer, tail int) Option {
	return func(a *App) {
		if buffer > 0 {
			a.logBuffer = buffer
		}
		if tail > 0 {
			a.logTail = tail
		}
	}
}

// WithShell sets the command `s` runs in a container. Empty keeps the
// default, which prefers bash and falls back to sh.
func WithShell(command string) Option {
	return func(a *App) {
		if command != "" {
			a.shellCommand = []string{command}
		}
	}
}

// WithLogger sends a record of what the application does to l.
func WithLogger(l *slog.Logger) Option {
	return func(a *App) {
		if l != nil {
			a.log = l
		}
	}
}

// Policy is what applies to the connection in use.
type Policy struct {
	ReadOnly   bool // every change is refused
	Production bool // the context is marked as one to be careful with
}

// WithPolicy sets how the policy of a context is decided. It is asked again
// whenever the connection changes.
func WithPolicy(f func(context string) Policy) Option {
	return func(a *App) { a.policyFor = f }
}

// setPolicy replaces the policy source and applies it to the current connection.
func (a *App) setPolicy(f func(context string) Policy) {
	a.policyFor = f
	a.applyPolicy()
	a.drawHeader()
}

// applyPolicy works out the policy for the current context and rebuilds
// the executor with it, which is where read-only is enforced.
func (a *App) applyPolicy() {
	a.policy = Policy{}
	if a.policyFor != nil {
		a.policy = a.policyFor(a.info.Context)
	}
	if a.client != nil {
		a.executor = action.New(a.client, a.policy.ReadOnly)
	}
}

// refuseIfReadOnly says why and returns true when changes are not allowed.
func (a *App) refuseIfReadOnly(what string) bool {
	if !a.policy.ReadOnly {
		return false
	}
	a.Flash(flashWarn, "read-only mode: "+what+" is disabled")
	return true
}

// Context returns the name of the Docker context in use.
func (a *App) Context() string { return a.info.Context }

// switchTo connects to ep and, once that works, moves the whole
// application over. A failure leaves the current connection untouched.
func (a *App) switchTo(ep docker.Endpoint) {
	if a.connect == nil || ep.Context == a.info.Context {
		return
	}
	a.Flash(flashInfo, "connecting to "+ep.Context+"…")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
		defer cancel()
		client, info, err := a.connect(ctx, ep)
		a.log.Info("switch context", "to", ep.Context, "host", ep.Host, "error", errText(err))
		a.queue(func() {
			if err != nil {
				a.Flash(flashError, "connect to "+ep.Context+": "+oneLine(err.Error()))
				return
			}
			a.adopt(client, info)
			a.Flash(flashInfo, "connected to "+ep.Context)
		})
	}()
}

// adopt replaces the connection: everything fed by the old client stops
// before it is closed, and the home view opens on the new one.
func (a *App) adopt(client docker.Client, info docker.Info) {
	a.stopWatch()
	old := a.client
	a.client = client
	a.info = info
	a.applyPolicy()
	a.startEventLog()
	a.drawHeader()
	if err := a.ShowHome(); err != nil { // closes every page, and with them their streams
		a.Flash(flashError, err.Error())
	}
	if old != nil {
		go func() { _ = old.Close() }()
	}
}

// HomeAuto as the home view means: pick by what the daemon is.
const HomeAuto = "auto"

// WithHomeView sets the view a connection opens on: a view's command, or
// HomeAuto (also the meaning of "") to follow the cluster.
func WithHomeView(command string) Option { return func(a *App) { a.home = command } }

// ShowHome opens the home view of the current connection. On HomeAuto that
// is the service list on a swarm manager, where the cluster is, and the
// container list everywhere else.
func (a *App) ShowHome() error {
	if a.registry == nil {
		return nil
	}
	command := a.home
	if command == "" || command == HomeAuto {
		command = "containers"
		if a.info.Swarm.Manager && a.registry.Has("services") {
			command = "services"
		}
	}
	res, err := a.registry.Lookup(command)
	if err != nil {
		return fmt.Errorf("home view %q is not a view: %w", command, err)
	}
	a.ShowResource(res)
	return nil
}

// Close releases the current connection. Call it after Run returns.
func (a *App) Close() {
	a.stopWatch()
	if a.cancelEvents != nil {
		a.cancelEvents()
	}
	if a.client != nil {
		_ = a.client.Close()
	}
}

// eventLogSize is how many daemon events the events view can look back on.
const eventLogSize = 500

// startEventLog begins recording the current daemon's events, replacing
// the log of a previous connection.
func (a *App) startEventLog() {
	if a.cancelEvents != nil {
		a.cancelEvents()
		a.cancelEvents = nil
	}
	a.events = store.NewEventLog(eventLogSize)
	if a.client == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelEvents = cancel
	go a.events.Run(ctx, a.client, store.DefaultRetry)
}

// Events returns the recorded events of the current daemon, oldest first.
func (a *App) Events() []docker.Event {
	if a.events == nil {
		return nil
	}
	return a.events.Recent()
}
