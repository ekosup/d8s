package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

type sdkClient struct {
	cli      *client.Client
	endpoint Endpoint
}

// Connect opens a client for the endpoint, negotiates the API version and
// verifies the daemon answers.
func Connect(ctx context.Context, ep Endpoint) (Client, error) {
	cli, err := client.New(client.WithHost(ep.Host))
	if err != nil {
		return nil, fmt.Errorf("configure docker client for %s: %w", ep.Host, err)
	}
	if _, err := cli.Ping(ctx, client.PingOptions{NegotiateAPIVersion: true}); err != nil {
		_ = cli.Close()
		return nil, explainConnectError(ep.Host, err)
	}
	return &sdkClient{cli: cli, endpoint: ep}, nil
}

func (c *sdkClient) Info(ctx context.Context) (Info, error) {
	v, err := c.cli.ServerVersion(ctx, client.ServerVersionOptions{})
	if err != nil {
		return Info{}, fmt.Errorf("get server version: %w", err)
	}
	return Info{
		Context:       c.endpoint.Context,
		Host:          c.endpoint.Host,
		ServerVersion: v.Version,
		APIVersion:    c.cli.ClientVersion(),
	}, nil
}

func (c *sdkClient) Containers(ctx context.Context) ([]Container, error) {
	res, err := c.cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	out := make([]Container, 0, len(res.Items))
	for _, s := range res.Items {
		out = append(out, toContainer(s))
	}
	return out, nil
}

func toContainer(s container.Summary) Container {
	name := s.ID
	if len(s.Names) > 0 {
		name = strings.TrimPrefix(s.Names[0], "/")
	}
	ports := make([]Port, 0, len(s.Ports))
	for _, p := range s.Ports {
		ip := ""
		if p.IP.IsValid() {
			ip = p.IP.String()
		}
		ports = append(ports, Port{IP: ip, Public: p.PublicPort, Private: p.PrivatePort, Proto: p.Type})
	}
	return Container{
		ID:      s.ID,
		Name:    name,
		Image:   s.Image,
		State:   string(s.State),
		Status:  s.Status,
		Ports:   ports,
		Created: time.Unix(s.Created, 0),
	}
}

func (c *sdkClient) Events(ctx context.Context) (<-chan Event, <-chan error) {
	res := c.cli.Events(ctx, client.EventsListOptions{})
	out := make(chan Event)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case m, ok := <-res.Messages:
				if !ok {
					return
				}
				select {
				case out <- toEvent(m):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, res.Err
}

func toEvent(m events.Message) Event {
	return Event{
		Type:   string(m.Type),
		Action: string(m.Action),
		ID:     m.Actor.ID,
		Time:   time.Unix(0, m.TimeNano),
	}
}

func (c *sdkClient) Close() error {
	return c.cli.Close()
}
