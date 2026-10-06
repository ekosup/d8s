package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateRemembersSortAcrossSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.yaml")
	s := OpenState(path)
	if _, _, ok := s.Sort("containers"); ok {
		t.Fatal("a fresh state remembers something")
	}
	if err := s.SetSort("containers", "CPU%", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSort("images", "SIZE", false); err != nil {
		t.Fatal(err)
	}

	// A new session reads the same file.
	s2 := OpenState(path)
	if col, desc, ok := s2.Sort("containers"); !ok || col != "CPU%" || !desc {
		t.Fatalf("containers: %q %v %v", col, desc, ok)
	}
	if col, desc, ok := s2.Sort("images"); !ok || col != "SIZE" || desc {
		t.Fatalf("images: %q %v %v", col, desc, ok)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("state file: %v, mode %v", err, info.Mode().Perm())
	}
}

func TestStateSurvivesACorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	if err := os.WriteFile(path, []byte("{{{ not yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := OpenState(path)
	if _, _, ok := s.Sort("containers"); ok {
		t.Fatal("read something out of a corrupt file")
	}
	if err := s.SetSort("containers", "NAME", false); err != nil {
		t.Fatal(err)
	}
	if col, _, ok := OpenState(path).Sort("containers"); !ok || col != "NAME" {
		t.Fatal("could not recover from a corrupt state file")
	}
}

func TestStateReportsAnUnwritableLocation(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := OpenState(filepath.Join(blocker, "state.yaml")).SetSort("x", "NAME", false); err == nil {
		t.Fatal("no error for a path that cannot be created")
	}
}
