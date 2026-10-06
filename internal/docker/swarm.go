package docker

import (
	"context"
	"fmt"
	"io"
	"time"
)

// More object kinds, for a daemon that is a Swarm manager.
const (
	KindService Kind = "service"
	KindTask    Kind = "task"
	KindNode    Kind = "node"
	KindSecret  Kind = "secret"
	KindConfig  Kind = "config"
)

// StackLabel is the label `docker stack deploy` puts on everything that
// belongs to a stack. A stack is not an API object; it is this label.
const StackLabel = "com.docker.stack.namespace"

// SwarmInfo says how the connected daemon takes part in a swarm.
type SwarmInfo struct {
	Active  bool // the daemon is a swarm node
	Manager bool // it can answer Swarm API calls
	Leader  bool
	NodeID  string
}

// Node is a swarm node.
type Node struct {
	ID            string
	Hostname      string
	State         string // ready, down, unknown, disconnected
	Availability  string // active, pause, drain
	Role          string // manager, worker
	Leader        bool
	Reachability  string // managers only
	EngineVersion string
	Addr          string
	Labels        map[string]string
}

// Service is a swarm service.
type Service struct {
	ID            string
	Name          string
	Stack         string // empty when not deployed as part of a stack
	Mode          string // replicated, global, replicated-job, global-job
	Image         string
	Desired       uint64
	Running       uint64
	Ports         []Port
	UpdateState   string // empty when the service was never updated
	UpdateMessage string
	UpdateStarted time.Time
	Secrets       []string
	Configs       []string
	PreviousImage string // image of the spec a rollback would restore; empty if none
	Created       time.Time
	Updated       time.Time
}

// Task is one scheduled instance of a service.
type Task struct {
	ID           string
	ServiceID    string
	Slot         int
	NodeID       string
	DesiredState string
	State        string
	Message      string
	Err          string // the full error, never shortened
	Image        string
	ContainerID  string
	Timestamp    time.Time // when it entered State
	Created      time.Time
}

// Secret is the metadata of a swarm secret. Its value is never available.
type Secret struct {
	ID      string
	Name    string
	Stack   string
	Created time.Time
	Updated time.Time
}

// Config is the metadata of a swarm config.
type Config struct {
	ID      string
	Name    string
	Stack   string
	Created time.Time
	Updated time.Time
}

// ServiceChange is an update to a service. Set exactly what should change.
type ServiceChange struct {
	Replicas *uint64 // new replica count; replicated services only
	Image    string  // new image reference
	Force    bool    // restart every task without changing the spec
	Rollback bool    // return to the previous spec
}

// NodeChange is an update to a node. Empty fields are left alone.
type NodeChange struct {
	Availability string
	Role         string
	SetLabels    map[string]string
	RemoveLabels []string
}

// Swarm is the part of Client that needs a swarm manager.
type Swarm interface {
	Nodes(ctx context.Context) ([]Node, error)
	Services(ctx context.Context) ([]Service, error)
	Tasks(ctx context.Context) ([]Task, error)
	Secrets(ctx context.Context) ([]Secret, error)
	Configs(ctx context.Context) ([]Config, error)
	// ConfigData returns the content of a config.
	ConfigData(ctx context.Context, id string) ([]byte, error)
	// ServiceLogs and TaskLogs read through the manager, so they work for
	// tasks on any node. Each line is prefixed with its task and node.
	ServiceLogs(ctx context.Context, id string, opts LogOptions) (io.ReadCloser, error)
	TaskLogs(ctx context.Context, id string, opts LogOptions) (io.ReadCloser, error)
	// ServiceUpdate applies change on top of the service's current spec.
	ServiceUpdate(ctx context.Context, id string, change ServiceChange) error
	NodeUpdate(ctx context.Context, id string, change NodeChange) error
	// StackRemove deletes the services, secrets, configs and networks of a
	// stack, as `docker stack rm` does.
	StackRemove(ctx context.Context, name string) error
}

// ErrOtherNode reports that a task's container runs on a node other than
// the one d8s is connected to, where exec and stats cannot reach it.
type ErrOtherNode struct {
	Node string // hostname of the node that has the container
}

func (e *ErrOtherNode) Error() string {
	return fmt.Sprintf("the container runs on node %s; a shell needs a connection to that node's daemon", e.Node)
}

// TaskName is the name docker shows for a task: service.slot, or
// service.node for a global service, which has no slots.
func TaskName(service string, slot int, nodeID string) string {
	suffix := fmt.Sprintf("%d", slot)
	if slot == 0 {
		suffix = nodeID
		if len(suffix) > 12 {
			suffix = suffix[:12]
		}
	}
	if service == "" {
		return suffix
	}
	return service + "." + suffix
}
