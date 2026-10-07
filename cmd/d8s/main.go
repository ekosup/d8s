// Command d8s is a terminal UI for Docker Engine and Docker Swarm.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/ekosup/d8s/internal/config"
	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
	"github.com/ekosup/d8s/internal/store"
	"github.com/ekosup/d8s/internal/ui"
	"github.com/ekosup/d8s/internal/version"
)

const connectTimeout = 40 * time.Second

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "d8s:", err)
		os.Exit(1)
	}
}

// options is what the command line asked for.
type options struct {
	command    string // run, version, info, help
	readOnly   bool
	context    string
	configPath string
	logFile    string
}

func parseArgs(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("d8s", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.readOnly, "readonly", false, "")
	fs.StringVar(&o.context, "context", "", "")
	fs.StringVar(&o.configPath, "config", "", "")
	fs.StringVar(&o.logFile, "log-file", "", "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{command: "help"}, nil
		}
		return options{}, err
	}
	switch rest := fs.Args(); {
	case len(rest) == 0:
		o.command = "run"
	case len(rest) == 1 && (rest[0] == "version" || rest[0] == "info" || rest[0] == "help"):
		o.command = rest[0]
	default:
		return options{}, fmt.Errorf("unknown command %q; try `d8s help`", rest[0])
	}
	return o, nil
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `d8s - a terminal UI for Docker Engine and Docker Swarm

Usage:
  d8s [flags]          open the UI
  d8s info [flags]     print version, connection and file locations
  d8s version          print the version
  d8s help             print this text

Flags:
  --readonly           refuse every change, whatever the configuration says
  --context NAME       use this Docker context instead of the current one
  --config PATH        read settings from PATH (default: $XDG_CONFIG_HOME/d8s/config.yaml)
  --log-file PATH      append a record of what d8s does to PATH

Inside the UI, press ? for keys and commands.
`)
}

// envWithContext makes --context win over the environment, the way the
// docker CLI's own flag does.
func envWithContext(getenv func(string) string, name string) func(string) string {
	if name == "" {
		return getenv
	}
	return func(k string) string {
		switch k {
		case "DOCKER_CONTEXT":
			return name
		case "DOCKER_HOST":
			return ""
		}
		return getenv(k)
	}
}

func configPath(o options, getenv func(string) string) string {
	if o.configPath != "" {
		return o.configPath
	}
	return config.Path(getenv)
}

func run(args []string) error {
	o, err := parseArgs(args)
	if err != nil {
		return err
	}
	switch o.command {
	case "help":
		printUsage(os.Stdout)
		return nil
	case "version":
		fmt.Println(version.String())
		return nil
	case "info":
		return printInfo(os.Stdout, o, os.Getenv)
	}
	return runUI(o)
}

// printInfo reports what d8s would use, including what is wrong with it.
func printInfo(w io.Writer, o options, getenv func(string) string) error {
	env := envWithContext(getenv, o.context)
	line := func(key, value string) { _, _ = fmt.Fprintf(w, "%-10s %s\n", key, value) }
	line("version", version.String())

	path := configPath(o, getenv)
	cfg, cfgErr := config.Load(path)
	state := "not found, using defaults"
	if _, err := os.Stat(path); err == nil {
		state = "loaded"
	}
	if cfgErr != nil {
		state = "INVALID: " + cfgErr.Error()
	}
	line("config", path+" ("+state+")")
	if cfgErr == nil {
		for _, w := range contextWarnings(cfg, func() ([]docker.Endpoint, error) { return docker.ListContexts(getenv) }) {
			line("warning", w)
		}
	}
	line("state", ui.StateDir(getenv))

	ep, err := docker.ResolveEndpoint(env)
	if err != nil {
		line("context", err.Error())
		return err
	}
	line("context", ep.Context)
	line("host", ep.Host)

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	client, info, err := connect(ctx, ep)
	if err != nil {
		line("daemon", err.Error())
		return err
	}
	defer func() { _ = client.Close() }()
	line("engine", info.ServerVersion)
	line("api", info.APIVersion)
	swarm := "inactive"
	switch {
	case info.Swarm.Manager && info.Swarm.Leader:
		swarm = "manager (leader)"
	case info.Swarm.Manager:
		swarm = "manager"
	case info.Swarm.Active:
		swarm = "worker"
	}
	line("swarm", swarm)
	return cfgErr
}

func runUI(o options) error {
	cfg, err := config.Load(configPath(o, os.Getenv))
	if err != nil {
		return err
	}
	if err := ui.SetSkin(ui.SkinFor(cfg.Skin, os.Getenv)); err != nil {
		return err
	}
	logger, closeLog, err := openLog(o.logFile)
	if err != nil {
		return err
	}
	defer closeLog()

	env := envWithContext(os.Getenv, o.context)
	ep, err := docker.ResolveEndpoint(env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	client, info, err := connect(ctx, ep)
	if err != nil {
		return err
	}
	logger.Info("started", "version", version.Version, "context", info.Context, "host", info.Host, "engine", info.ServerVersion)

	registry, err := resource.Default(time.Now)
	if err != nil {
		return err
	}
	var app *ui.App
	listContexts := func() ([]docker.Endpoint, error) { return docker.ListContexts(os.Getenv) }
	contexts := resource.Contexts(listContexts, func() string { return app.Context() },
		resource.WithContextPolicy(func(name string) resource.ContextPolicy {
			p := cfg.Policy(name, o.readOnly)
			return resource.ContextPolicy{ReadOnly: p.ReadOnly, Production: p.Production}
		}))
	events := resource.Events(func() []docker.Event { return app.Events() }, time.Local)
	for _, res := range []resource.Resource{contexts, events} {
		if err := registry.Register(res); err != nil {
			return err
		}
	}
	app = ui.NewApp(info,
		ui.WithClient(client),
		ui.WithRegistry(registry),
		ui.WithConnector(connect),
		ui.WithContexts(listContexts),
		ui.WithWatchOptions(store.Options{Poll: cfg.Refresh.Std()}),
		ui.WithLogSettings(cfg.LogBuffer, cfg.LogTail),
		ui.WithShell(cfg.Shell),
		ui.WithHomeView(cfg.DefaultView),
		ui.WithLogger(logger),
		ui.WithSortStore(config.OpenState(filepath.Join(ui.StateDir(os.Getenv), "state.yaml"))),
		ui.WithPolicy(func(context string) ui.Policy {
			p := cfg.Policy(context, o.readOnly)
			return ui.Policy{ReadOnly: p.ReadOnly, Production: p.Production}
		}),
	)
	defer app.Close()

	views := make(map[string][]string, len(cfg.Views))
	for name, v := range cfg.Views {
		views[name] = v.Columns
	}
	warnings := contextWarnings(cfg, listContexts)
	warnings = append(warnings, app.SetCustom(cfg.Aliases, cfg.Hotkeys)...)
	warnings = append(warnings, app.SetViews(views)...)

	if err := app.ShowHome(); err != nil {
		return fmt.Errorf("defaultView in the configuration: %w", err)
	}
	app.ShowWarnings(warnings)
	return app.Run()
}

// contextWarnings names the entries under `contexts` in the settings that
// match no Docker context. When the contexts cannot be listed there is
// nothing to compare with, and nothing is reported.
func contextWarnings(cfg config.Config, list func() ([]docker.Endpoint, error)) []string {
	eps, err := list()
	if err != nil {
		return nil
	}
	known := make([]string, len(eps))
	for i, ep := range eps {
		known[i] = ep.Context
	}
	var out []string
	for _, name := range cfg.UnknownContexts(known) {
		out = append(out, fmt.Sprintf("contexts: %q is not a Docker context; its settings apply to nothing", name))
	}
	return out
}

// openLog opens the --log-file, or returns a logger that discards.
func openLog(path string) (*slog.Logger, func(), error) {
	if path == "" {
		return slog.New(slog.DiscardHandler), func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	return slog.New(slog.NewTextHandler(f, nil)), func() { _ = f.Close() }, nil
}

// connect opens a client for ep and asks the daemon who it is.
func connect(ctx context.Context, ep docker.Endpoint) (docker.Client, docker.Info, error) {
	client, err := docker.Connect(ctx, ep)
	if err != nil {
		return nil, docker.Info{}, err
	}
	info, err := client.Info(ctx)
	if err != nil {
		_ = client.Close()
		return nil, docker.Info{}, err
	}
	return client, info, nil
}
