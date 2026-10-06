package ui

import (
	"context"
	"time"

	"github.com/ekosup/d8s/internal/action"
	"github.com/ekosup/d8s/internal/docker"
)

// connectTimeout is generous because an ssh endpoint has to log in first.
const connectTimeout = 40 * time.Second

// Connector opens a client for an endpoint and identifies the daemon.
type Connector func(ctx context.Context, ep docker.Endpoint) (docker.Client, docker.Info, error)

// WithConnector lets the application switch daemons at run time.
func WithConnector(c Connector) Option { return func(a *App) { a.connect = c } }

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
	a.executor = action.New(client, false)
	a.info = info
	a.drawHeader()
	if a.registry != nil {
		if home, err := a.registry.Lookup(homeResource); err == nil {
			a.ShowResource(home) // closes every page, and with them their streams
		}
	}
	if old != nil {
		go func() { _ = old.Close() }()
	}
}

// homeResource is the view a fresh connection starts on.
const homeResource = "containers"

// Close releases the current connection. Call it after Run returns.
func (a *App) Close() {
	a.stopWatch()
	if a.client != nil {
		_ = a.client.Close()
	}
}
