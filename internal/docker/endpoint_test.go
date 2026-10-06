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

func writeTLS(t *testing.T, configDir, name string, files ...string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(name))
	dir := filepath.Join(configDir, "contexts", "tls", hex.EncodeToString(sum[:]), "docker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("pem"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestListContexts(t *testing.T) {
	dir := t.TempDir()
	writeContext(t, dir, "rootless", "unix:///run/user/1000/docker.sock")
	writeContext(t, dir, "prod", "tcp://10.0.0.5:2376")
	writeContext(t, dir, "edge", "ssh://ops@edge.example.com")
	env := map[string]string{"DOCKER_CONFIG": dir}

	got, err := ListContexts(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	want := []Endpoint{
		{Context: "default", Host: DefaultHost},
		{Context: "edge", Host: "ssh://ops@edge.example.com"},
		{Context: "prod", Host: "tcp://10.0.0.5:2376"},
		{Context: "rootless", Host: "unix:///run/user/1000/docker.sock"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i].Context != want[i].Context || got[i].Host != want[i].Host {
			t.Fatalf("entry %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestListContextsWithoutDockerConfig(t *testing.T) {
	env := map[string]string{"DOCKER_CONFIG": filepath.Join(t.TempDir(), "missing")}
	got, err := ListContexts(func(k string) string { return env[k] })
	if err != nil || len(got) != 1 || got[0].Context != "default" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestContextTLSMaterial(t *testing.T) {
	dir := t.TempDir()
	writeContext(t, dir, "prod", "tcp://10.0.0.5:2376")
	tlsDir := writeTLS(t, dir, "prod", "ca.pem", "cert.pem", "key.pem")
	writeContext(t, dir, "plain", "tcp://10.0.0.6:2375")
	env := map[string]string{"DOCKER_CONFIG": dir, "DOCKER_CONTEXT": "prod"}

	ep, err := ResolveEndpoint(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if ep.TLS == nil || ep.TLS.CA != filepath.Join(tlsDir, "ca.pem") || ep.TLS.Cert != filepath.Join(tlsDir, "cert.pem") || ep.TLS.Key != filepath.Join(tlsDir, "key.pem") {
		t.Fatalf("TLS: %+v", ep.TLS)
	}

	env["DOCKER_CONTEXT"] = "plain"
	if ep, _ = ResolveEndpoint(func(k string) string { return env[k] }); ep.TLS != nil {
		t.Fatalf("TLS set for a context without certificates: %+v", ep.TLS)
	}
}

func TestEnvTLSMaterial(t *testing.T) {
	certs := t.TempDir()
	env := map[string]string{"DOCKER_HOST": "tcp://10.0.0.1:2376", "DOCKER_TLS_VERIFY": "1", "DOCKER_CERT_PATH": certs, "DOCKER_CONFIG": t.TempDir()}
	ep, err := ResolveEndpoint(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if ep.TLS == nil || ep.TLS.CA != filepath.Join(certs, "ca.pem") || ep.TLS.Key != filepath.Join(certs, "key.pem") {
		t.Fatalf("TLS: %+v", ep.TLS)
	}
	delete(env, "DOCKER_TLS_VERIFY")
	if ep, _ = ResolveEndpoint(func(k string) string { return env[k] }); ep.TLS != nil {
		t.Fatal("TLS set without DOCKER_TLS_VERIFY")
	}
}
