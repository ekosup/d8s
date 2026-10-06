package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeContext(t *testing.T, configDir, name, host string) {
	t.Helper()
	sum := sha256.Sum256([]byte(name))
	dir := filepath.Join(configDir, "contexts", "meta", hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := `{"Name":"` + name + `","Metadata":{},"Endpoints":{"docker":{"Host":"` + host + `","SkipTLSVerify":false}}}`
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		current     string // currentContext in config.json; "" = no config file
		wantContext string
		wantHost    string
		wantErr     string
	}{
		{name: "no config falls back to default", wantContext: "default", wantHost: DefaultHost},
		{name: "DOCKER_HOST wins over everything", env: map[string]string{"DOCKER_HOST": "tcp://10.0.0.1:2375", "DOCKER_CONTEXT": "rootless"}, current: "rootless", wantContext: "DOCKER_HOST", wantHost: "tcp://10.0.0.1:2375"},
		{name: "DOCKER_CONTEXT wins over config", env: map[string]string{"DOCKER_CONTEXT": "rootless"}, current: "default", wantContext: "rootless", wantHost: "unix:///run/user/1000/docker.sock"},
		{name: "current context from config", current: "rootless", wantContext: "rootless", wantHost: "unix:///run/user/1000/docker.sock"},
		{name: "config says default", current: "default", wantContext: "default", wantHost: DefaultHost},
		{name: "unknown context is an error", env: map[string]string{"DOCKER_CONTEXT": "nope"}, wantErr: `context "nope"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeContext(t, dir, "rootless", "unix:///run/user/1000/docker.sock")
			if tt.current != "" {
				cfg := `{"currentContext":"` + tt.current + `"}`
				if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			env := map[string]string{"DOCKER_CONFIG": dir}
			for k, v := range tt.env {
				env[k] = v
			}
			got, err := ResolveEndpoint(func(k string) string { return env[k] })
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got err %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Context != tt.wantContext || got.Host != tt.wantHost {
				t.Fatalf("got %+v, want context %q host %q", got, tt.wantContext, tt.wantHost)
			}
		})
	}
}
