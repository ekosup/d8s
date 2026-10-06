package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// State is what d8s remembers between sessions without being told to: how
// each view was last sorted. It is separate from the configuration, which
// only the user writes.
type State struct {
	mu   sync.Mutex
	path string
	data stateData
}

type stateData struct {
	Sort map[string]sortState `yaml:"sort"`
}

type sortState struct {
	Column string `yaml:"column"`
	Desc   bool   `yaml:"desc"`
}

// OpenState reads the state file at path. A missing or unreadable file
// simply means nothing is remembered yet.
func OpenState(path string) *State {
	s := &State{path: path}
	if raw, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(raw, &s.data) // a corrupt file is replaced on the next save
	}
	if s.data.Sort == nil {
		s.data.Sort = map[string]sortState{}
	}
	return s
}

// Sort returns the remembered sort of a view.
func (s *State) Sort(view string) (column string, desc, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data.Sort[view]
	return v.Column, v.Desc, ok
}

// SetSort remembers the sort of a view and writes the file.
func (s *State) SetSort(view, column string, desc bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Sort[view] = sortState{Column: column, Desc: desc}
	raw, err := yaml.Marshal(s.data)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	// Write beside the file and rename, so a crash never leaves half a file.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}
