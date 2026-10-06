package resource

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Lookup failures.
var (
	ErrUnknown   = errors.New("unknown command")
	ErrAmbiguous = errors.New("ambiguous command")
)

// Registry maps commands to resources.
type Registry struct {
	byName map[string]Resource
	byCmd  map[string]string // name or alias -> canonical name
	order  []string          // canonical names, as registered
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byName: map[string]Resource{}, byCmd: map[string]string{}}
}

// Register adds a resource. Names and aliases must be unique across the registry.
func (r *Registry) Register(res Resource) error {
	if res.Name == "" {
		return errors.New("resource has no name")
	}
	if res.List == nil {
		return fmt.Errorf("resource %q has no List function", res.Name)
	}
	if len(res.Columns) == 0 {
		return fmt.Errorf("resource %q has no columns", res.Name)
	}
	cmds := append([]string{res.Name}, res.Aliases...)
	for _, c := range cmds {
		if owner, taken := r.byCmd[strings.ToLower(c)]; taken {
			return fmt.Errorf("command %q of resource %q is already used by %q", c, res.Name, owner)
		}
	}
	r.byName[res.Name] = res
	r.order = append(r.order, res.Name)
	for _, c := range cmds {
		r.byCmd[strings.ToLower(c)] = res.Name
	}
	return nil
}

// Lookup resolves a command: an exact name or alias, or a prefix that points
// at exactly one resource.
func (r *Registry) Lookup(cmd string) (Resource, error) {
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	if cmd == "" {
		return Resource{}, ErrUnknown
	}
	if name, ok := r.byCmd[cmd]; ok {
		return r.byName[name], nil
	}
	matches := map[string]struct{}{}
	for c, name := range r.byCmd {
		if strings.HasPrefix(c, cmd) {
			matches[name] = struct{}{}
		}
	}
	switch len(matches) {
	case 0:
		return Resource{}, fmt.Errorf("%w: %s", ErrUnknown, cmd)
	case 1:
		for name := range matches {
			return r.byName[name], nil
		}
	}
	return Resource{}, fmt.Errorf("%w: %s", ErrAmbiguous, cmd)
}

// Complete returns the canonical names starting with prefix, sorted.
func (r *Registry) Complete(prefix string) []string {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	var out []string
	for name := range r.byName {
		if strings.HasPrefix(name, prefix) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Has reports whether cmd is exactly the name or an alias of a resource.
func (r *Registry) Has(cmd string) bool {
	_, ok := r.byCmd[strings.ToLower(strings.TrimSpace(cmd))]
	return ok
}

// All returns every resource in the order it was registered.
func (r *Registry) All() []Resource {
	out := make([]Resource, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.byName[name])
	}
	return out
}
