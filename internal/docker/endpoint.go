package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// DefaultHost is the daemon address of the built-in "default" context.
const DefaultHost = "unix:///var/run/docker.sock"

const defaultContext = "default"

// Endpoint is a resolved daemon address and the context it came from.
type Endpoint struct {
	Context string
	Host    string
	TLS     *TLSFiles // nil for a connection without client certificates
}

// TLSFiles are the PEM files that secure a TCP connection to a daemon.
type TLSFiles struct {
	CA, Cert, Key string
}

// ResolveEndpoint picks the daemon the same way the docker CLI does:
// DOCKER_HOST, then DOCKER_CONTEXT, then currentContext from config.json,
// then the default socket. getenv is injected so tests need no real env.
func ResolveEndpoint(getenv func(string) string) (Endpoint, error) {
	dir, err := configDir(getenv)
	if err != nil {
		return Endpoint{}, err
	}
	if host := getenv("DOCKER_HOST"); host != "" {
		ep := Endpoint{Context: "DOCKER_HOST", Host: host}
		if getenv("DOCKER_TLS_VERIFY") != "" {
			certs := getenv("DOCKER_CERT_PATH")
			if certs == "" {
				certs = dir
			}
			ep.TLS = &TLSFiles{CA: filepath.Join(certs, "ca.pem"), Cert: filepath.Join(certs, "cert.pem"), Key: filepath.Join(certs, "key.pem")}
		}
		return ep, nil
	}
	name := getenv("DOCKER_CONTEXT")
	if name == "" {
		if name, err = currentContext(dir); err != nil {
			return Endpoint{}, err
		}
	}
	if name == "" || name == defaultContext {
		return Endpoint{Context: defaultContext, Host: DefaultHost}, nil
	}
	return contextEndpoint(dir, name)
}

// ListContexts returns the built-in default context followed by every
// context the docker CLI has stored, sorted by name.
func ListContexts(getenv func(string) string) ([]Endpoint, error) {
	out := []Endpoint{{Context: defaultContext, Host: DefaultHost}}
	dir, err := configDir(getenv)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, "contexts", "meta"))
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read docker contexts: %w", err)
	}
	var stored []Endpoint
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name, err := contextName(filepath.Join(dir, "contexts", "meta", e.Name(), "meta.json"))
		if err != nil || name == "" || name == defaultContext {
			continue // not a context directory; the CLI ignores these too
		}
		ep, err := contextEndpoint(dir, name)
		if err != nil {
			continue
		}
		stored = append(stored, ep)
	}
	sort.Slice(stored, func(i, j int) bool { return stored[i].Context < stored[j].Context })
	return append(out, stored...), nil
}

func contextName(metaPath string) (string, error) {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return "", err
	}
	var meta struct {
		Name string `json:"Name"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", err
	}
	return meta.Name, nil
}

// contextEndpoint reads a stored context: its host, and its client
// certificates when the context has them.
func contextEndpoint(dir, name string) (Endpoint, error) {
	host, err := contextHost(dir, name)
	if err != nil {
		return Endpoint{}, err
	}
	ep := Endpoint{Context: name, Host: host}
	sum := sha256.Sum256([]byte(name))
	certs := filepath.Join(dir, "contexts", "tls", hex.EncodeToString(sum[:]), "docker")
	if _, err := os.Stat(filepath.Join(certs, "ca.pem")); err == nil {
		ep.TLS = &TLSFiles{CA: filepath.Join(certs, "ca.pem"), Cert: filepath.Join(certs, "cert.pem"), Key: filepath.Join(certs, "key.pem")}
	}
	return ep, nil
}

func configDir(getenv func(string) string) (string, error) {
	if dir := getenv("DOCKER_CONFIG"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate docker config: %w", err)
	}
	return filepath.Join(home, ".docker"), nil
}

func currentContext(dir string) (string, error) {
	path := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read docker config: %w", err)
	}
	var cfg struct {
		CurrentContext string `json:"currentContext"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg.CurrentContext, nil
}

func contextHost(dir, name string) (string, error) {
	sum := sha256.Sum256([]byte(name))
	path := filepath.Join(dir, "contexts", "meta", hex.EncodeToString(sum[:]), "meta.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("docker context %q not found", name)
	}
	if err != nil {
		return "", fmt.Errorf("read docker context %q: %w", name, err)
	}
	var meta struct {
		Endpoints map[string]struct {
			Host string `json:"Host"`
		} `json:"Endpoints"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", fmt.Errorf("parse docker context %q: %w", name, err)
	}
	host := meta.Endpoints["docker"].Host
	if host == "" {
		return "", fmt.Errorf("docker context %q has no docker endpoint", name)
	}
	return host, nil
}
