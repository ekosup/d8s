package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    options
		wantErr string
	}{
		{name: "no arguments", want: options{command: "run"}},
		{name: "version", args: []string{"version"}, want: options{command: "version"}},
		{name: "info", args: []string{"info"}, want: options{command: "info"}},
		{name: "help flag", args: []string{"--help"}, want: options{command: "help"}},
		{name: "help command", args: []string{"help"}, want: options{command: "help"}},
		{name: "readonly", args: []string{"--readonly"}, want: options{command: "run", readOnly: true}},
		{name: "all flags", args: []string{"--readonly", "--context", "prod", "--config", "/c.yaml", "--log-file", "/l.log"},
			want: options{command: "run", readOnly: true, context: "prod", configPath: "/c.yaml", logFile: "/l.log"}},
		{name: "flags with info", args: []string{"--context", "prod", "info"}, want: options{command: "info", context: "prod"}},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: "nope"},
		{name: "unknown command", args: []string{"dance"}, wantErr: "dance"},
		{name: "flag without value", args: []string{"--context"}, wantErr: "context"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEnvWithContext(t *testing.T) {
	base := map[string]string{"DOCKER_HOST": "tcp://1.2.3.4:2375", "DOCKER_CONTEXT": "old", "HOME": "/home/u"}
	get := func(k string) string { return base[k] }

	env := envWithContext(get, "prod")
	if env("DOCKER_CONTEXT") != "prod" || env("DOCKER_HOST") != "" || env("HOME") != "/home/u" {
		t.Fatalf("--context must win over the environment: ctx=%q host=%q", env("DOCKER_CONTEXT"), env("DOCKER_HOST"))
	}
	env = envWithContext(get, "")
	if env("DOCKER_CONTEXT") != "old" || env("DOCKER_HOST") != "tcp://1.2.3.4:2375" {
		t.Fatal("without the flag the environment must be left alone")
	}
}

func TestUsageMentionsEveryFlagAndCommand(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)
	for _, want := range []string{"--readonly", "--context", "--config", "--log-file", "version", "info", "config.yaml"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("usage does not mention %q:\n%s", want, buf.String())
		}
	}
}

func TestInfoReportsWithoutADaemon(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{
		"HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "cfg"), "XDG_STATE_HOME": filepath.Join(dir, "state"),
		"DOCKER_HOST": "unix://" + filepath.Join(dir, "no.sock"), "DOCKER_CONFIG": dir,
	}
	var buf bytes.Buffer
	err := printInfo(&buf, options{command: "info"}, func(k string) string { return env[k] })
	out := buf.String()
	// A daemon that is down is part of the report, not a reason to print nothing.
	for _, want := range []string{"version", filepath.Join(dir, "cfg", "d8s", "config.yaml"), "not found", "no.sock", filepath.Join(dir, "state", "d8s")} {
		if !strings.Contains(out, want) {
			t.Fatalf("info output misses %q (err %v):\n%s", want, err, out)
		}
	}
}

func TestInfoReportsABrokenConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("skin: neon\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"HOME": dir, "D8S_CONFIG": path, "DOCKER_HOST": "unix://" + filepath.Join(dir, "no.sock"), "DOCKER_CONFIG": dir}
	var buf bytes.Buffer
	_ = printInfo(&buf, options{command: "info"}, func(k string) string { return env[k] })
	if !strings.Contains(buf.String(), "neon") || !strings.Contains(buf.String(), "line 1") {
		t.Fatalf("info does not report the config problem:\n%s", buf.String())
	}
}

// dockerContext writes a stored Docker context the way the docker CLI does.
func dockerContext(t *testing.T, dockerConfig, name, host string) {
	t.Helper()
	dir := filepath.Join(dockerConfig, "contexts", "meta", fmt.Sprintf("%x", sha256.Sum256([]byte(name))))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	meta := fmt.Sprintf(`{"Name":%q,"Metadata":{},"Endpoints":{"docker":{"Host":%q,"SkipTLSVerify":false}}}`, name, host)
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInfoWarnsAboutSettingsForAContextThatDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := "contexts:\n  portal1:\n    readOnly: true\n  portalprod:\n    production: true\n"
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	dockerContext(t, dir, "portalprod", "ssh://portal1")
	env := map[string]string{"HOME": dir, "D8S_CONFIG": path, "DOCKER_HOST": "unix://" + filepath.Join(dir, "no.sock"), "DOCKER_CONFIG": dir}
	var buf bytes.Buffer
	_ = printInfo(&buf, options{command: "info"}, func(k string) string { return env[k] })
	out := buf.String()
	if !strings.Contains(out, `"portal1" is not a Docker context`) {
		t.Fatalf("no warning for the misnamed context:\n%s", out)
	}
	if strings.Contains(out, `"portalprod" is not`) {
		t.Fatalf("warned about a context that exists:\n%s", out)
	}
}
