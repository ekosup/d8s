package resource

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ekosup/d8s/internal/docker"
)

func stub(name string, aliases ...string) Resource {
	return Resource{
		Name:    name,
		Aliases: aliases,
		Title:   name,
		Columns: []Column{{Name: "NAME"}},
		List:    func(context.Context, docker.Client) ([]Row, error) { return nil, nil },
	}
}

func newRegistry(t *testing.T, rs ...Resource) *Registry {
	t.Helper()
	reg := NewRegistry()
	for _, r := range rs {
		if err := reg.Register(r); err != nil {
			t.Fatal(err)
		}
	}
	return reg
}

func TestRegistryLookup(t *testing.T) {
	reg := newRegistry(t,
		stub("containers", "c", "container", "ps"),
		stub("configs", "cfg", "config"),
		stub("images", "i", "image"),
	)
	tests := []struct {
		cmd     string
		want    string
		wantErr error
	}{
		{"containers", "containers", nil},
		{"c", "containers", nil},
		{"C", "containers", nil},
		{"  ps ", "containers", nil},
		{"cont", "containers", nil}, // unique prefix, through name and alias of one resource
		{"im", "images", nil},
		{"con", "", ErrAmbiguous}, // containers and configs
		{"xyz", "", ErrUnknown},
		{"", "", ErrUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got, err := reg.Lookup(tt.cmd)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got err %v, want %v", err, tt.wantErr)
			}
			if got.Name != tt.want {
				t.Fatalf("got %q, want %q", got.Name, tt.want)
			}
		})
	}
}

func TestRegistryComplete(t *testing.T) {
	reg := newRegistry(t, stub("containers", "c"), stub("configs", "cfg"), stub("images", "i"))
	tests := []struct {
		prefix string
		want   []string
	}{
		{"cont", []string{"containers"}},
		{"co", []string{"configs", "containers"}},
		{"", []string{"configs", "containers", "images"}},
		{"z", nil},
	}
	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			if got := reg.Complete(tt.prefix); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegistryRegisterRejects(t *testing.T) {
	tests := []struct {
		name string
		res  Resource
	}{
		{"empty name", stub("")},
		{"duplicate name", stub("containers")},
		{"alias collides with a name", stub("pods", "containers")},
		{"alias collides with an alias", stub("pods", "c")},
		{"no list function", Resource{Name: "pods", Columns: []Column{{Name: "NAME"}}}},
		{"no columns", Resource{Name: "pods", List: stub("x").List}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := newRegistry(t, stub("containers", "c"))
			if err := reg.Register(tt.res); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestRegistryAllKeepsRegistrationOrder(t *testing.T) {
	reg := newRegistry(t, stub("zebra"), stub("apple", "a"), stub("mango"))
	var names []string
	for _, r := range reg.All() {
		names = append(names, r.Name)
	}
	if !slices.Equal(names, []string{"zebra", "apple", "mango"}) {
		t.Fatalf("got %v", names)
	}
	// The result is a copy.
	reg.All()[0].Name = "changed"
	if reg.All()[0].Name != "zebra" {
		t.Fatal("All exposed the registry's own slice")
	}
}
