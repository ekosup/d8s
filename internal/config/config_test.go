package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultsWithoutAFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := Default()
	if cfg.Refresh != want.Refresh || cfg.DefaultView != "auto" || cfg.LogBuffer != 5000 || cfg.LogTail != 1000 ||
		cfg.Skin != "dark" || cfg.ReadOnly || cfg.Refresh.Std() != 2*time.Second {
		t.Fatalf("defaults: %+v", cfg)
	}
}

func TestLoadEverything(t *testing.T) {
	cfg, err := Load(write(t, `
refresh: 5s
defaultView: svc
logBuffer: 20000
logTail: 200
shell: /bin/zsh
readOnly: true
skin: light
contexts:
  prod-swarm:
    readOnly: true
    production: true
  staging:
    production: false
aliases:
  web: services /shop_web
hotkeys:
  f2: svc
  ctrl-w: web
views:
  containers:
    columns: [NAME, STATE, CPU%, MEM]
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Refresh.Std() != 5*time.Second || cfg.DefaultView != "svc" || cfg.LogBuffer != 20000 || cfg.LogTail != 200 ||
		cfg.Shell != "/bin/zsh" || !cfg.ReadOnly || cfg.Skin != "light" {
		t.Fatalf("scalars: %+v", cfg)
	}
	if p := cfg.Contexts["prod-swarm"]; !p.ReadOnly || !p.Production {
		t.Fatalf("context policy: %+v", p)
	}
	if cfg.Aliases["web"] != "services /shop_web" || cfg.Hotkeys["f2"] != "svc" {
		t.Fatalf("aliases %v hotkeys %v", cfg.Aliases, cfg.Hotkeys)
	}
	if !slices.Equal(cfg.Views["containers"].Columns, []string{"NAME", "STATE", "CPU%", "MEM"}) {
		t.Fatalf("columns: %v", cfg.Views["containers"].Columns)
	}
}

func TestPartialFileKeepsOtherDefaults(t *testing.T) {
	cfg, err := Load(write(t, "skin: light\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Skin != "light" || cfg.LogBuffer != 5000 || cfg.Refresh.Std() != 2*time.Second || cfg.DefaultView != "auto" {
		t.Fatalf("got %+v", cfg)
	}
}

func TestEmptyFileIsFine(t *testing.T) {
	for _, content := range []string{"", "\n\n", "# only a comment\n"} {
		if cfg, err := Load(write(t, content)); err != nil || cfg.LogBuffer != 5000 {
			t.Fatalf("content %q: %+v, %v", content, cfg, err)
		}
	}
}

func TestErrorsPointAtTheLine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		line    string
		about   string
	}{
		{"broken yaml", "skin: dark\nrefresh: [oops\n", "line 2", ""},
		{"unknown key", "skin: dark\nrefesh: 5s\n", "line 2", "refesh"},
		{"wrong type", "logBuffer: lots\n", "line 1", "logBuffer"},
		{"bad duration", "skin: dark\n\nrefresh: fast\n", "line 3", "refresh"},
		{"refresh too small", "refresh: 10ms\n", "line 1", "at least"},
		{"log buffer too small", "skin: dark\nlogBuffer: 5\n", "line 2", "logBuffer"},
		{"unknown skin", "\n\nskin: neon\n", "line 3", "neon"},
		{"negative tail", "logTail: -1\n", "line 1", "logTail"},
		{"unknown key in context", "contexts:\n  prod:\n    readonly: true\n", "line 3", "readonly"},
		{"alias with no target", "aliases:\n  web: ''\n", "line 2", "web"},
		{"alias named with a space", "aliases:\n  'my web': svc\n", "line 2", "my web"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := write(t, tt.content)
			_, err := Load(path)
			if err == nil {
				t.Fatal("expected an error")
			}
			msg := err.Error()
			if !strings.Contains(msg, tt.line) || !strings.Contains(msg, tt.about) || !strings.Contains(msg, path) {
				t.Fatalf("error %q should name the file, %q and %q", msg, tt.line, tt.about)
			}
		})
	}
}

func TestPolicy(t *testing.T) {
	cfg := Default()
	cfg.Contexts = map[string]ContextPolicy{"prod": {ReadOnly: true, Production: true}, "staging": {}}

	if p := cfg.Policy("prod", false); !p.ReadOnly || !p.Production {
		t.Fatalf("prod: %+v", p)
	}
	if p := cfg.Policy("staging", false); p.ReadOnly || p.Production {
		t.Fatalf("staging: %+v", p)
	}
	if p := cfg.Policy("unknown", false); p.ReadOnly || p.Production {
		t.Fatalf("unknown: %+v", p)
	}
	// The --readonly flag and the global setting apply everywhere.
	if p := cfg.Policy("staging", true); !p.ReadOnly {
		t.Fatal("the flag did not force read-only")
	}
	cfg.ReadOnly = true
	if p := cfg.Policy("unknown", false); !p.ReadOnly {
		t.Fatal("the global setting did not apply")
	}
}

func TestPathResolution(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := Path(env(map[string]string{"D8S_CONFIG": "/tmp/x.yaml", "XDG_CONFIG_HOME": "/xdg"})); got != "/tmp/x.yaml" {
		t.Fatalf("D8S_CONFIG: %q", got)
	}
	if got := Path(env(map[string]string{"XDG_CONFIG_HOME": "/xdg"})); got != "/xdg/d8s/config.yaml" {
		t.Fatalf("XDG: %q", got)
	}
	if got := Path(env(map[string]string{"HOME": "/home/u"})); got != "/home/u/.config/d8s/config.yaml" {
		t.Fatalf("HOME: %q", got)
	}
}

func TestUnknownContexts(t *testing.T) {
	cfg := Config{Contexts: map[string]ContextPolicy{
		"scprod":  {Production: true},
		"portal1": {ReadOnly: true},
		"Zeta":    {},
	}}
	got := cfg.UnknownContexts([]string{"default", "scprod", "portalprod"})
	if want := []string{"Zeta", "portal1"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := (Config{}).UnknownContexts([]string{"default"}); len(got) != 0 {
		t.Fatalf("no contexts configured, yet %v", got)
	}
}
