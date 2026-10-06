package docker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplainConnectError(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "docker.sock")
	if err := os.WriteFile(existing, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		host string
		err  error
		want []string
	}{
		{"missing socket", "unix:///tidak/ada", errors.New("dial unix /tidak/ada: connect: no such file or directory"), []string{"socket not found", "/tidak/ada"}},
		{"permission denied", "unix://" + existing, errors.New("dial unix: connect: permission denied"), []string{"permission denied", "docker group"}},
		{"daemon down on socket", "unix://" + existing, errors.New("dial unix: connect: connection refused"), []string{"not responding", existing}},
		{"tcp unreachable", "tcp://10.0.0.1:2375", errors.New("dial tcp 10.0.0.1:2375: i/o timeout"), []string{"cannot reach", "tcp://10.0.0.1:2375"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := explainConnectError(tt.host, tt.err)
			if !errors.Is(got, tt.err) {
				t.Fatalf("cause is not wrapped: %v", got)
			}
			for _, w := range tt.want {
				if !strings.Contains(got.Error(), w) {
					t.Fatalf("message %q does not contain %q", got.Error(), w)
				}
			}
		})
	}
}
