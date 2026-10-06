package docker

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// explainConnectError turns a dial failure into a message that names the
// cause, keeping the original error in the chain.
func explainConnectError(host string, err error) error {
	path, isUnix := strings.CutPrefix(host, "unix://")
	if !isUnix {
		return fmt.Errorf("cannot reach docker daemon at %s: %w", host, err)
	}
	if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("docker socket not found at %s (is the daemon running?): %w", path, err)
	}
	if errors.Is(err, fs.ErrPermission) || strings.Contains(err.Error(), "permission denied") {
		return fmt.Errorf("permission denied on %s (add your user to the docker group): %w", path, err)
	}
	return fmt.Errorf("docker daemon is not responding on %s: %w", path, err)
}
