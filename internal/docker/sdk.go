package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
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
	var endpoints []Attachment
	if s.NetworkSettings != nil {
		for netName, ep := range s.NetworkSettings.Networks {
			if ep == nil {
				continue
			}
			ip := ""
			if ep.IPAddress.IsValid() {
				ip = ep.IPAddress.String()
			}
			endpoints = append(endpoints, Attachment{Network: netName, NetworkID: ep.NetworkID, IP: ip})
		}
	}
	var volumes []string
	for _, m := range s.Mounts {
		if m.Type == "volume" && m.Name != "" {
			volumes = append(volumes, m.Name)
		}
	}
	return Container{
		ID:       s.ID,
		Name:     name,
		Image:    s.Image,
		State:    string(s.State),
		Status:   s.Status,
		Ports:    ports,
		Created:  time.Unix(s.Created, 0),
		ImageID:  s.ImageID,
		Labels:   s.Labels,
		Networks: endpoints,
		Volumes:  volumes,
	}
}

func (c *sdkClient) Images(ctx context.Context) ([]Image, error) {
	res, err := c.cli.ImageList(ctx, client.ImageListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	out := make([]Image, 0, len(res.Items))
	for _, s := range res.Items {
		out = append(out, Image{ID: s.ID, Tags: s.RepoTags, Size: s.Size, Created: time.Unix(s.Created, 0)})
	}
	return out, nil
}

func (c *sdkClient) ImageHistory(ctx context.Context, id string) ([]ImageLayer, error) {
	res, err := c.cli.ImageHistory(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("image history: %w", err)
	}
	out := make([]ImageLayer, 0, len(res.Items))
	for _, h := range res.Items {
		out = append(out, ImageLayer{ID: h.ID, CreatedBy: h.CreatedBy, Size: h.Size, Created: time.Unix(h.Created, 0)})
	}
	return out, nil
}

func (c *sdkClient) Volumes(ctx context.Context) ([]Volume, error) {
	res, err := c.cli.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}
	out := make([]Volume, 0, len(res.Items))
	for _, v := range res.Items {
		created, _ := time.Parse(time.RFC3339, v.CreatedAt)
		out = append(out, Volume{Name: v.Name, Driver: v.Driver, Mountpoint: v.Mountpoint, Created: created})
	}
	return out, nil
}

func (c *sdkClient) Networks(ctx context.Context) ([]Network, error) {
	res, err := c.cli.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	out := make([]Network, 0, len(res.Items))
	for _, n := range res.Items {
		var subnets []string
		for _, cfg := range n.IPAM.Config {
			if cfg.Subnet.IsValid() {
				subnets = append(subnets, cfg.Subnet.String())
			}
		}
		out = append(out, Network{ID: n.ID, Name: n.Name, Driver: n.Driver, Scope: n.Scope, Subnets: subnets})
	}
	return out, nil
}

func (c *sdkClient) Remove(ctx context.Context, kind Kind, id string) error {
	var err error
	switch kind {
	case KindImage:
		_, err = c.cli.ImageRemove(ctx, id, client.ImageRemoveOptions{PruneChildren: true})
	case KindVolume:
		_, err = c.cli.VolumeRemove(ctx, id, client.VolumeRemoveOptions{})
	case KindNetwork:
		_, err = c.cli.NetworkRemove(ctx, id, client.NetworkRemoveOptions{})
	default:
		return fmt.Errorf("cannot remove %q", kind)
	}
	if err != nil {
		return fmt.Errorf("remove %s: %w", kind, err)
	}
	return nil
}

func (c *sdkClient) Prune(ctx context.Context, kind Kind) (PruneReport, error) {
	var (
		rep PruneReport
		err error
	)
	switch kind {
	case KindImage:
		var res client.ImagePruneResult
		res, err = c.cli.ImagePrune(ctx, client.ImagePruneOptions{Filters: make(client.Filters).Add("dangling", "true")})
		rep = PruneReport{Count: len(res.Report.ImagesDeleted), Reclaimed: res.Report.SpaceReclaimed}
	case KindContainer:
		var res client.ContainerPruneResult
		res, err = c.cli.ContainerPrune(ctx, client.ContainerPruneOptions{})
		rep = PruneReport{Count: len(res.Report.ContainersDeleted), Reclaimed: res.Report.SpaceReclaimed}
	case KindVolume:
		var res client.VolumePruneResult
		res, err = c.cli.VolumePrune(ctx, client.VolumePruneOptions{})
		rep = PruneReport{Count: len(res.Report.VolumesDeleted), Reclaimed: res.Report.SpaceReclaimed}
	case KindBuildCache:
		var res client.BuildCachePruneResult
		res, err = c.cli.BuildCachePrune(ctx, client.BuildCachePruneOptions{})
		rep = PruneReport{Count: len(res.Report.CachesDeleted), Reclaimed: res.Report.SpaceReclaimed}
	default:
		return PruneReport{}, fmt.Errorf("cannot prune %q", kind)
	}
	if err != nil {
		return PruneReport{}, fmt.Errorf("prune %s: %w", kind, err)
	}
	return rep, nil
}

func (c *sdkClient) DiskUsage(ctx context.Context) ([]DiskUsage, error) {
	res, err := c.cli.DiskUsage(ctx, client.DiskUsageOptions{Containers: true, Images: true, BuildCache: true, Volumes: true})
	if err != nil {
		return nil, fmt.Errorf("disk usage: %w", err)
	}
	return []DiskUsage{
		{Kind: KindImage, Total: res.Images.TotalCount, Active: res.Images.ActiveCount, Size: res.Images.TotalSize, Reclaimable: res.Images.Reclaimable},
		{Kind: KindContainer, Total: res.Containers.TotalCount, Active: res.Containers.ActiveCount, Size: res.Containers.TotalSize, Reclaimable: res.Containers.Reclaimable},
		{Kind: KindVolume, Total: res.Volumes.TotalCount, Active: res.Volumes.ActiveCount, Size: res.Volumes.TotalSize, Reclaimable: res.Volumes.Reclaimable},
		{Kind: KindBuildCache, Total: res.BuildCache.TotalCount, Active: res.BuildCache.ActiveCount, Size: res.BuildCache.TotalSize, Reclaimable: res.BuildCache.Reclaimable},
	}, nil
}

func (c *sdkClient) ContainerAction(ctx context.Context, id string, op ContainerOp) error {
	var err error
	switch op {
	case OpStart:
		_, err = c.cli.ContainerStart(ctx, id, client.ContainerStartOptions{})
	case OpStop:
		_, err = c.cli.ContainerStop(ctx, id, client.ContainerStopOptions{})
	case OpRestart:
		_, err = c.cli.ContainerRestart(ctx, id, client.ContainerRestartOptions{})
	case OpPause:
		_, err = c.cli.ContainerPause(ctx, id, client.ContainerPauseOptions{})
	case OpUnpause:
		_, err = c.cli.ContainerUnpause(ctx, id, client.ContainerUnpauseOptions{})
	case OpKill:
		_, err = c.cli.ContainerKill(ctx, id, client.ContainerKillOptions{})
	case OpRemove:
		_, err = c.cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true})
	default:
		return fmt.Errorf("unknown container operation %q", op)
	}
	if err != nil {
		return fmt.Errorf("%s container: %w", op, err)
	}
	return nil
}

func (c *sdkClient) ContainerLogs(ctx context.Context, id string, opts LogOptions) (io.ReadCloser, error) {
	// A container with a TTY sends raw output; without one the daemon
	// multiplexes stdout and stderr and the stream must be unpacked.
	info, err := c.cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}
	tty := info.Container.Config != nil && info.Container.Config.Tty

	o := client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: opts.Follow, Timestamps: opts.Timestamps}
	if opts.Tail > 0 {
		o.Tail = strconv.Itoa(opts.Tail)
	}
	if opts.Since > 0 {
		o.Since = strconv.FormatInt(time.Now().Add(-opts.Since).Unix(), 10)
	}
	body, err := c.cli.ContainerLogs(ctx, id, o)
	if err != nil {
		return nil, fmt.Errorf("read container logs: %w", err)
	}
	if tty {
		return body, nil
	}
	pr, pw := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(pw, pw, body)
		_ = body.Close()
		_ = pw.CloseWithError(err)
	}()
	return &pipeCloser{PipeReader: pr, closeSource: body.Close}, nil
}

// pipeCloser closes the stream it unpacks from when the reader is closed,
// which is what ends the copying goroutine.
type pipeCloser struct {
	*io.PipeReader
	closeSource func() error
}

func (p *pipeCloser) Close() error {
	_ = p.closeSource()
	return p.PipeReader.Close()
}

func (c *sdkClient) Exec(ctx context.Context, id string, opts ExecOptions) (ExecSession, error) {
	size := client.ConsoleSize{Height: opts.Rows, Width: opts.Cols}
	created, err := c.cli.ExecCreate(ctx, id, client.ExecCreateOptions{
		TTY:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		ConsoleSize:  size,
		Env:          opts.Env,
		Cmd:          opts.Cmd,
	})
	if err != nil {
		return nil, fmt.Errorf("create exec: %w", err)
	}
	att, err := c.cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{TTY: true, ConsoleSize: size})
	if err != nil {
		return nil, fmt.Errorf("attach exec: %w", err)
	}
	return &execSession{cli: c.cli, id: created.ID, att: att}, nil
}

type execSession struct {
	cli *client.Client
	id  string
	att client.ExecAttachResult
}

func (e *execSession) Read(p []byte) (int, error)  { return e.att.Reader.Read(p) }
func (e *execSession) Write(p []byte) (int, error) { return e.att.Conn.Write(p) }

func (e *execSession) Close() error {
	e.att.Close()
	return nil
}

func (e *execSession) Resize(ctx context.Context, rows, cols uint) error {
	if _, err := e.cli.ExecResize(ctx, e.id, client.ExecResizeOptions{Height: rows, Width: cols}); err != nil {
		return fmt.Errorf("resize exec: %w", err)
	}
	return nil
}

func (e *execSession) ExitCode(ctx context.Context) (int, error) {
	res, err := e.cli.ExecInspect(ctx, e.id, client.ExecInspectOptions{})
	if err != nil {
		return 0, fmt.Errorf("inspect exec: %w", err)
	}
	return res.ExitCode, nil
}

func (c *sdkClient) Inspect(ctx context.Context, kind Kind, id string) ([]byte, error) {
	var (
		raw []byte
		err error
	)
	switch kind {
	case KindContainer:
		var res client.ContainerInspectResult
		res, err = c.cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
		raw = res.Raw
	case KindImage:
		var res client.ImageInspectResult
		if res, err = c.cli.ImageInspect(ctx, id); err == nil {
			raw, err = json.Marshal(res.InspectResponse)
		}
	case KindVolume:
		var res client.VolumeInspectResult
		res, err = c.cli.VolumeInspect(ctx, id, client.VolumeInspectOptions{})
		raw = res.Raw
	case KindNetwork:
		var res client.NetworkInspectResult
		res, err = c.cli.NetworkInspect(ctx, id, client.NetworkInspectOptions{})
		raw = res.Raw
	default:
		return nil, fmt.Errorf("cannot inspect %q", kind)
	}
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", kind, err)
	}
	return raw, nil
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
