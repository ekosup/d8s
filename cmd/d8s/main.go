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

	client, info, err := connect()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	registry, err := resource.Default(time.Now)
	if err != nil {
		return err
	}
	home, err := registry.Lookup("containers")
	if err != nil {
		return err
	}

	app := ui.NewApp(info, ui.WithClient(client), ui.WithRegistry(registry))
	app.ShowResource(home)
	return app.Run()
}

func connect() (docker.Client, docker.Info, error) {
	ep, err := docker.ResolveEndpoint(os.Getenv)
	if err != nil {
		return nil, docker.Info{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
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
