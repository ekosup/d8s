package dockertest

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/ekosup/d8s/internal/docker"
)

// swarmState is the swarm side of a Fake.
type swarmState struct {
	nodes      []docker.Node
	services   []docker.Service
	tasks      []docker.Task
	secrets    []docker.Secret
	configs    []docker.Config
	configData map[string][]byte
}

// SetNodes replaces the node list.
func (f *Fake) SetNodes(ns ...docker.Node) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.swarm.nodes = ns
}

// SetServices replaces the service list.
func (f *Fake) SetServices(ss ...docker.Service) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.swarm.services = ss
}

// SetTasks replaces the task list.
func (f *Fake) SetTasks(ts ...docker.Task) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.swarm.tasks = ts
}

// SetSecrets replaces the secret list.
func (f *Fake) SetSecrets(ss ...docker.Secret) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.swarm.secrets = ss
}

// SetConfigs replaces the config list.
func (f *Fake) SetConfigs(cs ...docker.Config) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.swarm.configs = cs
}

// SetConfigData sets the content of one config.
func (f *Fake) SetConfigData(id string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.swarm.configData == nil {
		f.swarm.configData = map[string][]byte{}
	}
	f.swarm.configData[id] = data
}

// SetSwarm sets how the fake daemon takes part in a swarm.
func (f *Fake) SetSwarm(info docker.SwarmInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.info.Swarm = info
}

// Nodes implements docker.Swarm.
func (f *Fake) Nodes(context.Context) ([]docker.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return slices.Clone(f.swarm.nodes), f.listErr
}

// Services implements docker.Swarm.
func (f *Fake) Services(context.Context) ([]docker.Service, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return slices.Clone(f.swarm.services), f.listErr
}

// Tasks implements docker.Swarm.
func (f *Fake) Tasks(context.Context) ([]docker.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return slices.Clone(f.swarm.tasks), f.listErr
}

// Secrets implements docker.Swarm.
func (f *Fake) Secrets(context.Context) ([]docker.Secret, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return slices.Clone(f.swarm.secrets), f.listErr
}

// Configs implements docker.Swarm.
func (f *Fake) Configs(context.Context) ([]docker.Config, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return slices.Clone(f.swarm.configs), f.listErr
}

// ConfigData implements docker.Swarm.
func (f *Fake) ConfigData(_ context.Context, id string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.swarm.configData[id]
	if !ok {
		return nil, fmt.Errorf("config %s not found", id)
	}
	return data, nil
}

// ServiceLogs implements docker.Swarm; set the lines with SetLogs("service/<id>").
func (f *Fake) ServiceLogs(ctx context.Context, id string, opts docker.LogOptions) (io.ReadCloser, error) {
	return f.ContainerLogs(ctx, "service/"+id, opts)
}

// TaskLogs implements docker.Swarm; set the lines with SetLogs("task/<id>").
func (f *Fake) TaskLogs(ctx context.Context, id string, opts docker.LogOptions) (io.ReadCloser, error) {
	return f.ContainerLogs(ctx, "task/"+id, opts)
}

// ServiceUpdate implements docker.Swarm. It logs the change and applies it
// the way a manager would report it afterwards.
func (f *Fake) ServiceUpdate(_ context.Context, id string, ch docker.ServiceChange) error {
	f.mu.Lock()
	if f.actionErr != nil {
		err := f.actionErr
		f.mu.Unlock()
		return err
	}
	found := false
	for i, s := range f.swarm.services {
		if s.ID != id {
			continue
		}
		found = true
		switch {
		case ch.Rollback:
			f.log = append(f.log, "service "+id+" rollback")
			s.Image, s.PreviousImage = s.PreviousImage, s.Image
			s.UpdateState = "rollback_completed"
		case ch.Replicas != nil:
			f.log = append(f.log, fmt.Sprintf("service %s replicas=%d", id, *ch.Replicas))
			s.Desired = *ch.Replicas
		case ch.Image != "":
			f.log = append(f.log, "service "+id+" image="+ch.Image)
			s.PreviousImage, s.Image = s.Image, ch.Image
			s.UpdateState = "updating"
		case ch.Force:
			f.log = append(f.log, "service "+id+" force")
			s.UpdateState = "updating"
		}
		f.swarm.services[i] = s
	}
	f.mu.Unlock()
	if !found {
		return fmt.Errorf("service %s not found", id)
	}
	f.Emit(docker.Event{Type: "service", Action: "update", ID: id})
	return nil
}

// NodeUpdate implements docker.Swarm.
func (f *Fake) NodeUpdate(_ context.Context, id string, ch docker.NodeChange) error {
	f.mu.Lock()
	if f.actionErr != nil {
		err := f.actionErr
		f.mu.Unlock()
		return err
	}
	for i, n := range f.swarm.nodes {
		if n.ID != id {
			continue
		}
		if ch.Availability != "" {
			f.log = append(f.log, "node "+id+" availability="+ch.Availability)
			n.Availability = ch.Availability
		}
		if ch.Role != "" {
			f.log = append(f.log, "node "+id+" role="+ch.Role)
			n.Role = ch.Role
		}
		labels := map[string]string{}
		for k, v := range n.Labels {
			labels[k] = v
		}
		keys := make([]string, 0, len(ch.SetLabels))
		for k := range ch.SetLabels {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			f.log = append(f.log, "node "+id+" label "+k+"="+ch.SetLabels[k])
			labels[k] = ch.SetLabels[k]
		}
		for _, k := range ch.RemoveLabels {
			f.log = append(f.log, "node "+id+" unlabel "+k)
			delete(labels, k)
		}
		n.Labels = labels
		f.swarm.nodes[i] = n
	}
	f.mu.Unlock()
	f.Emit(docker.Event{Type: "node", Action: "update", ID: id})
	return nil
}

// StackRemove implements docker.Swarm.
func (f *Fake) StackRemove(_ context.Context, name string) error {
	f.mu.Lock()
	if f.actionErr != nil {
		err := f.actionErr
		f.mu.Unlock()
		return err
	}
	f.log = append(f.log, "stack rm "+name)
	f.swarm.services = slices.DeleteFunc(f.swarm.services, func(s docker.Service) bool { return s.Stack == name })
	f.swarm.secrets = slices.DeleteFunc(f.swarm.secrets, func(s docker.Secret) bool { return s.Stack == name })
	f.swarm.configs = slices.DeleteFunc(f.swarm.configs, func(c docker.Config) bool { return c.Stack == name })
	f.mu.Unlock()
	f.Emit(docker.Event{Type: "service", Action: "remove"})
	return nil
}

// removeSwarmObject handles Remove for swarm kinds. The caller holds f.mu.
func (f *Fake) removeSwarmObject(kind docker.Kind, id string) {
	switch kind {
	case docker.KindService:
		f.swarm.services = slices.DeleteFunc(f.swarm.services, func(s docker.Service) bool { return s.ID == id })
	case docker.KindSecret:
		f.swarm.secrets = slices.DeleteFunc(f.swarm.secrets, func(s docker.Secret) bool { return s.ID == id })
	case docker.KindConfig:
		f.swarm.configs = slices.DeleteFunc(f.swarm.configs, func(c docker.Config) bool { return c.ID == id })
	}
}
