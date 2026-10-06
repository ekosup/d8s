// Package docker is the only place that talks to the Docker API. The rest of
// the application depends on the Client interface, never on the SDK.
package docker

import (
	"context"
	"io"
	"time"
)

// Info identifies the daemon d8s is connected to.
type Info struct {
	Context       string
	Host          string
	ServerVersion string
	APIVersion    string
	Swarm         SwarmInfo
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

	ImageID  string
	Labels   map[string]string
	Networks []Attachment // networks the container is attached to
	Volumes  []string     // names of the volumes it mounts
}

// Attachment is a container's connection to one network.
type Attachment struct {
	Network   string
	NetworkID string
	IP        string
}

// Image is one image; it may carry several tags or none.
type Image struct {
	ID      string
	Tags    []string // "repo:tag"; empty for a dangling image
	Size    int64
	Created time.Time
}

// ImageLayer is one step of an image's build history.
type ImageLayer struct {
	ID        string
	CreatedBy string
	Size      int64
	Created   time.Time
}

// Volume is a named volume.
type Volume struct {
	Name       string
	Driver     string
	Mountpoint string
	Created    time.Time
}

// Network is a Docker network.
type Network struct {
	ID      string
	Name    string
	Driver  string
	Scope   string
	Subnets []string
}

// DiskUsage summarises the space one kind of object takes.
type DiskUsage struct {
	Kind        Kind
	Total       int64
	Active      int64
	Size        int64
	Reclaimable int64
}

// Stats is one resource usage sample of a container. CPU counters are
// cumulative: usage over an interval is the difference of two samples.
type Stats struct {
	CPUTotal   uint64 // nanoseconds of CPU the container has used
	CPUSystem  uint64 // nanoseconds of CPU the whole host has used
	OnlineCPUs uint32
	MemUsage   uint64 // bytes, without reclaimable page cache
	MemLimit   uint64
	NetRx      uint64
	NetTx      uint64
	BlkRead    uint64
	BlkWrite   uint64
	PIDs       uint64
}

// PruneReport says what a prune removed.
type PruneReport struct {
	Count     int
	Reclaimed uint64
}

// Event is a daemon event, reduced to what triggers a refresh.
type Event struct {
	Type   string
	Action string
	ID     string
	Name   string // the object's name, when the daemon reports one
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
	KindContainer  Kind = "container"
	KindImage      Kind = "image"
	KindVolume     Kind = "volume"
	KindNetwork    Kind = "network"
	KindBuildCache Kind = "build cache"
)

// LogOptions selects which log lines to read.
type LogOptions struct {
	Follow     bool          // keep the stream open for new lines
	Tail       int           // only the last n lines; 0 = all
	Since      time.Duration // only lines newer than this; 0 = no limit
	Timestamps bool          // prefix each line with its RFC 3339 time
}

// ExecSession is an interactive command running in a container with a TTY.
// Reading gives its output, writing sends input, closing ends it.
type ExecSession interface {
	io.ReadWriteCloser
	Resize(ctx context.Context, rows, cols uint) error
	// ExitCode is valid once reading has reached the end.
	ExitCode(ctx context.Context) (int, error)
}

// ExecOptions describes the command of an ExecSession.
type ExecOptions struct {
	Cmd        []string
	Env        []string
	Rows, Cols uint
}

// Client is everything d8s needs from a Docker daemon.
type Client interface {
	Info(ctx context.Context) (Info, error)
	Containers(ctx context.Context) ([]Container, error)
	ContainerAction(ctx context.Context, id string, op ContainerOp) error
	// ContainerLogs returns the container's output as plain text, stdout
	// and stderr interleaved. Closing the reader ends the stream.
	ContainerLogs(ctx context.Context, id string, opts LogOptions) (io.ReadCloser, error)
	// Exec starts an interactive command in a running container.
	Exec(ctx context.Context, id string, opts ExecOptions) (ExecSession, error)
	// ContainerStats takes one usage sample of a running container.
	ContainerStats(ctx context.Context, id string) (Stats, error)
	Images(ctx context.Context) ([]Image, error)
	ImageHistory(ctx context.Context, id string) ([]ImageLayer, error)
	Volumes(ctx context.Context) ([]Volume, error)
	Networks(ctx context.Context) ([]Network, error)
	// Remove deletes one image, volume or network. It is not forced: an
	// object still in use is refused by the daemon.
	Remove(ctx context.Context, kind Kind, id string) error
	// Prune deletes what is unused: dangling images, stopped containers,
	// unused anonymous volumes, or the build cache.
	Prune(ctx context.Context, kind Kind) (PruneReport, error)
	DiskUsage(ctx context.Context) ([]DiskUsage, error)
	// Inspect returns the daemon's full description of an object as JSON.
	Inspect(ctx context.Context, kind Kind, id string) ([]byte, error)
	// Events streams daemon events until ctx is cancelled. An error on the
	// second channel ends the stream; the caller subscribes again.
	Events(ctx context.Context) (<-chan Event, <-chan error)
	Close() error

	Swarm
}
