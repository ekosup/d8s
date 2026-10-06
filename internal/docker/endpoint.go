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
)

// DefaultHost is the daemon address of the built-in "default" context.
const DefaultHost = "unix:///var/run/docker.sock"

const defaultContext = "default"

// Endpoint is a resolved daemon address and the context it came from.
type Endpoint struct {
	Context string
	Host    string
}

// ResolveEndpoint picks the daemon the same way the docker CLI does:
// DOCKER_HOST, then DOCKER_CONTEXT, then currentContext from config.json,
// then the default socket. getenv is injected so tests need no real env.
func ResolveEndpoint(getenv func(string) string) (Endpoint, error) {
	if host := getenv("DOCKER_HOST"); host != "" {
		return Endpoint{Context: "DOCKER_HOST", Host: host}, nil
	}
	dir, err := configDir(getenv)
	if err != nil {
		return Endpoint{}, err
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
	host, err := contextHost(dir, name)
	if err != nil {
		return Endpoint{}, err
	}
	return Endpoint{Context: name, Host: host}, nil
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
