package docker

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
)

func (c *sdkClient) swarmInfo(ctx context.Context) (SwarmInfo, error) {
	res, err := c.cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return SwarmInfo{}, fmt.Errorf("get daemon info: %w", err)
	}
	sw := res.Info.Swarm
	info := SwarmInfo{
		Active:  sw.LocalNodeState == swarm.LocalNodeStateActive,
		Manager: sw.ControlAvailable,
		NodeID:  sw.NodeID,
	}
	if info.Manager {
		if node, err := c.cli.NodeInspect(ctx, sw.NodeID, client.NodeInspectOptions{}); err == nil && node.Node.ManagerStatus != nil {
			info.Leader = node.Node.ManagerStatus.Leader
		}
	}
	return info, nil
}

func (c *sdkClient) Nodes(ctx context.Context) ([]Node, error) {
	res, err := c.cli.NodeList(ctx, client.NodeListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	out := make([]Node, 0, len(res.Items))
	for _, n := range res.Items {
		node := Node{
			ID:            n.ID,
			Hostname:      n.Description.Hostname,
			State:         string(n.Status.State),
			Availability:  string(n.Spec.Availability),
			Role:          string(n.Spec.Role),
			EngineVersion: n.Description.Engine.EngineVersion,
			Addr:          n.Status.Addr,
			Labels:        n.Spec.Labels,
		}
		if n.ManagerStatus != nil {
			node.Leader = n.ManagerStatus.Leader
			node.Reachability = string(n.ManagerStatus.Reachability)
		}
		out = append(out, node)
	}
	return out, nil
}

func (c *sdkClient) Services(ctx context.Context) ([]Service, error) {
	res, err := c.cli.ServiceList(ctx, client.ServiceListOptions{Status: true})
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	out := make([]Service, 0, len(res.Items))
	for _, s := range res.Items {
		out = append(out, toService(s))
	}
	return out, nil
}

func toService(s swarm.Service) Service {
	svc := Service{
		ID:      s.ID,
		Name:    s.Spec.Name,
		Stack:   s.Spec.Labels[StackLabel],
		Created: s.CreatedAt,
		Updated: s.UpdatedAt,
	}
	switch {
	case s.Spec.Mode.Replicated != nil:
		svc.Mode = "replicated"
	case s.Spec.Mode.Global != nil:
		svc.Mode = "global"
	case s.Spec.Mode.ReplicatedJob != nil:
		svc.Mode = "replicated-job"
	case s.Spec.Mode.GlobalJob != nil:
		svc.Mode = "global-job"
	}
	if cs := s.Spec.TaskTemplate.ContainerSpec; cs != nil {
		svc.Image = cs.Image
		for _, r := range cs.Secrets {
			if r != nil {
				svc.Secrets = append(svc.Secrets, r.SecretName)
			}
		}
		for _, r := range cs.Configs {
			if r != nil {
				svc.Configs = append(svc.Configs, r.ConfigName)
			}
		}
	}
	if s.PreviousSpec != nil && s.PreviousSpec.TaskTemplate.ContainerSpec != nil {
		svc.PreviousImage = s.PreviousSpec.TaskTemplate.ContainerSpec.Image
	}
	if s.ServiceStatus != nil {
		svc.Desired, svc.Running = s.ServiceStatus.DesiredTasks, s.ServiceStatus.RunningTasks
	}
	if s.UpdateStatus != nil {
		svc.UpdateState, svc.UpdateMessage = string(s.UpdateStatus.State), s.UpdateStatus.Message
		if s.UpdateStatus.StartedAt != nil {
			svc.UpdateStarted = *s.UpdateStatus.StartedAt
		}
	}
	for _, p := range s.Endpoint.Ports {
		svc.Ports = append(svc.Ports, Port{Public: uint16(p.PublishedPort), Private: uint16(p.TargetPort), Proto: string(p.Protocol)})
	}
	return svc
}

func (c *sdkClient) Tasks(ctx context.Context) ([]Task, error) {
	res, err := c.cli.TaskList(ctx, client.TaskListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	out := make([]Task, 0, len(res.Items))
	for _, t := range res.Items {
		task := Task{
			ID:           t.ID,
			ServiceID:    t.ServiceID,
			Slot:         t.Slot,
			NodeID:       t.NodeID,
			DesiredState: string(t.DesiredState),
			State:        string(t.Status.State),
			Message:      t.Status.Message,
			Err:          t.Status.Err,
			Timestamp:    t.Status.Timestamp,
			Created:      t.CreatedAt,
		}
		if t.Spec.ContainerSpec != nil {
			task.Image = t.Spec.ContainerSpec.Image
		}
		if t.Status.ContainerStatus != nil {
			task.ContainerID = t.Status.ContainerStatus.ContainerID
		}
		out = append(out, task)
	}
	return out, nil
}

func (c *sdkClient) Secrets(ctx context.Context) ([]Secret, error) {
	res, err := c.cli.SecretList(ctx, client.SecretListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	out := make([]Secret, 0, len(res.Items))
	for _, s := range res.Items {
		out = append(out, Secret{ID: s.ID, Name: s.Spec.Name, Stack: s.Spec.Labels[StackLabel], Created: s.CreatedAt, Updated: s.UpdatedAt})
	}
	return out, nil
}

func (c *sdkClient) Configs(ctx context.Context) ([]Config, error) {
	res, err := c.cli.ConfigList(ctx, client.ConfigListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list configs: %w", err)
	}
	out := make([]Config, 0, len(res.Items))
	for _, s := range res.Items {
		out = append(out, Config{ID: s.ID, Name: s.Spec.Name, Stack: s.Spec.Labels[StackLabel], Created: s.CreatedAt, Updated: s.UpdatedAt})
	}
	return out, nil
}

func (c *sdkClient) ConfigData(ctx context.Context, id string) ([]byte, error) {
	res, err := c.cli.ConfigInspect(ctx, id, client.ConfigInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("inspect config: %w", err)
	}
	return res.Config.Spec.Data, nil
}

func (c *sdkClient) ServiceUpdate(ctx context.Context, id string, change ServiceChange) error {
	res, err := c.cli.ServiceInspect(ctx, id, client.ServiceInspectOptions{})
	if err != nil {
		return fmt.Errorf("inspect service: %w", err)
	}
	// Start from the spec as it is now, so that nothing someone else
	// changed in the meantime is undone. The version makes the daemon
	// refuse the update if the service changed again after this read.
	spec := res.Service.Spec
	opts := client.ServiceUpdateOptions{Version: res.Service.Version, Spec: spec}
	if change.Rollback {
		opts.Rollback = "previous"
	}
	if change.Replicas != nil {
		if spec.Mode.Replicated == nil {
			return errors.New("only a replicated service can be scaled")
		}
		opts.Spec.Mode.Replicated = &swarm.ReplicatedService{Replicas: change.Replicas}
	}
	if change.Image != "" || change.Force {
		tmpl := spec.TaskTemplate
		if change.Image != "" {
			if tmpl.ContainerSpec == nil {
				return errors.New("the service has no container to change the image of")
			}
			cs := *tmpl.ContainerSpec
			cs.Image = change.Image
			tmpl.ContainerSpec = &cs
		}
		if change.Force {
			tmpl.ForceUpdate++
		}
		opts.Spec.TaskTemplate = tmpl
	}
	if _, err := c.cli.ServiceUpdate(ctx, id, opts); err != nil {
		return fmt.Errorf("update service: %w", err)
	}
	return nil
}

func (c *sdkClient) NodeUpdate(ctx context.Context, id string, change NodeChange) error {
	res, err := c.cli.NodeInspect(ctx, id, client.NodeInspectOptions{})
	if err != nil {
		return fmt.Errorf("inspect node: %w", err)
	}
	spec := res.Node.Spec
	if change.Availability != "" {
		spec.Availability = swarm.NodeAvailability(change.Availability)
	}
	if change.Role != "" {
		spec.Role = swarm.NodeRole(change.Role)
	}
	if len(change.SetLabels) > 0 || len(change.RemoveLabels) > 0 {
		labels := map[string]string{}
		for k, v := range spec.Labels {
			labels[k] = v
		}
		for k, v := range change.SetLabels {
			labels[k] = v
		}
		for _, k := range change.RemoveLabels {
			delete(labels, k)
		}
		spec.Labels = labels
	}
	if _, err := c.cli.NodeUpdate(ctx, id, client.NodeUpdateOptions{Version: res.Node.Version, Spec: spec}); err != nil {
		return fmt.Errorf("update node: %w", err)
	}
	return nil
}

func (c *sdkClient) StackRemove(ctx context.Context, name string) error {
	byStack := make(client.Filters).Add("label", StackLabel+"="+name)
	var errs []error

	services, err := c.cli.ServiceList(ctx, client.ServiceListOptions{Filters: byStack})
	if err != nil {
		return fmt.Errorf("list services of stack: %w", err)
	}
	for _, s := range services.Items {
		if _, err := c.cli.ServiceRemove(ctx, s.ID, client.ServiceRemoveOptions{}); err != nil {
			errs = append(errs, fmt.Errorf("remove service %s: %w", s.Spec.Name, err))
		}
	}
	secrets, err := c.cli.SecretList(ctx, client.SecretListOptions{Filters: byStack})
	if err == nil {
		for _, s := range secrets.Items {
			if _, err := c.cli.SecretRemove(ctx, s.ID, client.SecretRemoveOptions{}); err != nil {
				errs = append(errs, fmt.Errorf("remove secret %s: %w", s.Spec.Name, err))
			}
		}
	}
	configs, err := c.cli.ConfigList(ctx, client.ConfigListOptions{Filters: byStack})
	if err == nil {
		for _, s := range configs.Items {
			if _, err := c.cli.ConfigRemove(ctx, s.ID, client.ConfigRemoveOptions{}); err != nil {
				errs = append(errs, fmt.Errorf("remove config %s: %w", s.Spec.Name, err))
			}
		}
	}
	// A network stays in use until its tasks have stopped, so removal is
	// retried for a short while.
	networks, err := c.cli.NetworkList(ctx, client.NetworkListOptions{Filters: byStack})
	if err == nil {
		for _, n := range networks.Items {
			var rmErr error
			for range 20 {
				if _, rmErr = c.cli.NetworkRemove(ctx, n.ID, client.NetworkRemoveOptions{}); rmErr == nil {
					break
				}
				select {
				case <-ctx.Done():
					rmErr = ctx.Err()
				case <-time.After(500 * time.Millisecond):
					continue
				}
				break
			}
			if rmErr != nil {
				errs = append(errs, fmt.Errorf("remove network %s: %w", n.Name, rmErr))
			}
		}
	}
	return errors.Join(errs...)
}

func (c *sdkClient) ServiceLogs(ctx context.Context, id string, opts LogOptions) (io.ReadCloser, error) {
	svc, err := c.cli.ServiceInspect(ctx, id, client.ServiceInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("inspect service: %w", err)
	}
	tty := svc.Service.Spec.TaskTemplate.ContainerSpec != nil && svc.Service.Spec.TaskTemplate.ContainerSpec.TTY
	body, err := c.cli.ServiceLogs(ctx, id, client.ServiceLogsOptions(swarmLogOptions(opts)))
	if err != nil {
		return nil, fmt.Errorf("read service logs: %w", err)
	}
	return c.labelled(ctx, body, tty, opts.Timestamps), nil
}

func (c *sdkClient) TaskLogs(ctx context.Context, id string, opts LogOptions) (io.ReadCloser, error) {
	task, err := c.cli.TaskInspect(ctx, id, client.TaskInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("inspect task: %w", err)
	}
	tty := task.Task.Spec.ContainerSpec != nil && task.Task.Spec.ContainerSpec.TTY
	body, err := c.cli.TaskLogs(ctx, id, client.TaskLogsOptions(swarmLogOptions(opts)))
	if err != nil {
		return nil, fmt.Errorf("read task logs: %w", err)
	}
	return c.labelled(ctx, body, tty, opts.Timestamps), nil
}

// swarmLogOptions always asks for timestamps and details: the details carry
// the task and node of each line, and relabel needs both to find them.
func swarmLogOptions(opts LogOptions) client.ContainerLogsOptions {
	o := client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: opts.Follow, Timestamps: true, Details: true}
	if opts.Tail > 0 {
		o.Tail = strconv.Itoa(opts.Tail)
	}
	if opts.Since > 0 {
		o.Since = strconv.FormatInt(time.Now().Add(-opts.Since).Unix(), 10)
	}
	return o
}

// labelled turns a raw swarm log stream into plain lines that start with
// the task and node they came from.
func (c *sdkClient) labelled(ctx context.Context, body io.ReadCloser, tty, timestamps bool) io.ReadCloser {
	names := c.taskNamer(ctx)
	plain, plainW := io.Pipe()
	go func() {
		var err error
		if tty {
			_, err = io.Copy(plainW, body)
		} else {
			_, err = stdcopy.StdCopy(plainW, plainW, body)
		}
		_ = plainW.CloseWithError(err)
	}()
	out, outW := io.Pipe()
	go func() {
		_ = outW.CloseWithError(relabel(outW, plain, names, timestamps))
	}()
	return &pipeCloser{PipeReader: out, closeSource: func() error {
		_ = plain.Close()
		return body.Close()
	}}
}

// taskNamer resolves task and node IDs to "service.slot@hostname". It reads
// the cluster once; tasks created later show their IDs.
func (c *sdkClient) taskNamer(ctx context.Context) func(taskID, nodeID string) string {
	tasks, nodes := map[string]string{}, map[string]string{}
	if services, err := c.Services(ctx); err == nil {
		byID := make(map[string]string, len(services))
		for _, s := range services {
			byID[s.ID] = s.Name
		}
		if ts, err := c.Tasks(ctx); err == nil {
			for _, t := range ts {
				tasks[t.ID] = TaskName(byID[t.ServiceID], t.Slot, t.NodeID)
			}
		}
	}
	if ns, err := c.Nodes(ctx); err == nil {
		for _, n := range ns {
			nodes[n.ID] = n.Hostname
		}
	}
	return func(taskID, nodeID string) string {
		task, node := tasks[taskID], nodes[nodeID]
		if task == "" {
			task = shorten(taskID)
		}
		if node == "" {
			node = shorten(nodeID)
		}
		return task + "@" + node
	}
}

func shorten(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

const (
	detailTask = "com.docker.swarm.task.id="
	detailNode = "com.docker.swarm.node.id="
)

// relabel rewrites swarm log lines of the form
//
//	<timestamp> <key=value,key=value> <message>
//
// into "<timestamp> <task@node> | <message>", or without the timestamp. A
// line that does not have that form is passed through unchanged.
func relabel(w io.Writer, r io.Reader, names func(taskID, nodeID string) string, timestamps bool) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if stamp, rest, ok := strings.Cut(line, " "); ok {
			if details, msg, ok := strings.Cut(rest, " "); ok && strings.Contains(details, detailTask) {
				var taskID, nodeID string
				for _, kv := range strings.Split(details, ",") {
					if v, ok := strings.CutPrefix(kv, detailTask); ok {
						taskID = v
					}
					if v, ok := strings.CutPrefix(kv, detailNode); ok {
						nodeID = v
					}
				}
				line = names(taskID, nodeID) + " | " + msg
				if timestamps {
					line = stamp + " " + line
				}
			}
		}
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return err
		}
	}
	return sc.Err()
}
