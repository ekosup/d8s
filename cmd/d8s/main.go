// Command d8s is a terminal UI for Docker Engine and Docker Swarm.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/resource"
	"github.com/ekosup/d8s/internal/ui"
	"github.com/ekosup/d8s/internal/version"
)

const connectTimeout = 5 * time.Second

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "d8s:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "version" {
		fmt.Println(version.String())
		return nil
	}

	ep, err := docker.ResolveEndpoint(os.Getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	client, info, err := connect(ctx, ep)
	if err != nil {
		return err
	}

	registry, err := resource.Default(time.Now)
	if err != nil {
		return err
	}
	var app *ui.App
	contexts := resource.Contexts(
		func() ([]docker.Endpoint, error) { return docker.ListContexts(os.Getenv) },
		func() string { return app.Context() },
	)
	events := resource.Events(func() []docker.Event { return app.Events() }, time.Local)
	for _, res := range []resource.Resource{contexts, events} {
		if err := registry.Register(res); err != nil {
			return err
		}
	}
	home, err := registry.Lookup("containers")
	if err != nil {
		return err
	}

	listContexts := func() ([]docker.Endpoint, error) { return docker.ListContexts(os.Getenv) }
	app = ui.NewApp(info, ui.WithClient(client), ui.WithRegistry(registry), ui.WithConnector(connect), ui.WithContexts(listContexts))
	defer app.Close()
	app.ShowResource(home)
	return app.Run()
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
