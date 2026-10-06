// Package docker is the only place that talks to the Docker API. The rest of
// the application depends on the Client interface, never on the SDK.
package docker

import (
	"context"
	"time"
)

// Info identifies the daemon d8s is connected to.
type Info struct {
	Context       string
	Host          string
	ServerVersion string
	APIVersion    string
}

// Port is one published or exposed container port.
type Port struct {
	IP      string
	Public  uint16
	Private uint16
	Proto   string
}

// Container is the subset of a container summary the UI needs.
type Container struct {
	ID      string
	Name    string
	Image   string
	State   string
	Status  string
	Ports   []Port
	Created time.Time
}

// Event is a daemon event, reduced to what triggers a refresh.
type Event struct {
	Type   string
	Action string
	ID     string
	Time   time.Time
}

// ContainerOp is a state change applied to one container.
type ContainerOp string

// Container operations.
const (
	OpStart   ContainerOp = "start"
	OpStop    ContainerOp = "stop"
	OpRestart ContainerOp = "restart"
	OpPause   ContainerOp = "pause"
	OpUnpause ContainerOp = "unpause"
	OpKill    ContainerOp = "kill"
	OpRemove  ContainerOp = "remove" // forced: also removes a running container
)

// Kind names a type of Docker object for calls that work on several.
type Kind string

// Object kinds.
const (
	KindContainer Kind = "container"
	KindImage     Kind = "image"
	KindVolume    Kind = "volume"
	KindNetwork   Kind = "network"
)

// Client is everything d8s needs from a Docker daemon.
type Client interface {
	Info(ctx context.Context) (Info, error)
	Containers(ctx context.Context) ([]Container, error)
	ContainerAction(ctx context.Context, id string, op ContainerOp) error
	// Inspect returns the daemon's full description of an object as JSON.
	Inspect(ctx context.Context, kind Kind, id string) ([]byte, error)
	// Events streams daemon events until ctx is cancelled. An error on the
	// second channel ends the stream; the caller subscribes again.
	Events(ctx context.Context) (<-chan Event, <-chan error)
	Close() error
}
