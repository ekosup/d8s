package docker

import (
	"context"
	"io"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSSHArgs(t *testing.T) {
	tests := []struct {
		host    string
		want    []string
		wantErr bool
	}{
		{"ssh://edge.example.com", []string{"-o", "ConnectTimeout=30", "-T", "--", "edge.example.com", "docker", "system", "dial-stdio"}, false},
		{"ssh://ops@edge.example.com:2222", []string{"-o", "ConnectTimeout=30", "-T", "-l", "ops", "-p", "2222", "--", "edge.example.com", "docker", "system", "dial-stdio"}, false},
		{"ssh://ops@edge/var/run/alt.sock", []string{"-o", "ConnectTimeout=30", "-T", "-l", "ops", "--", "edge", "docker", "--host", "unix:///var/run/alt.sock", "system", "dial-stdio"}, false},
		{"ssh://", nil, true},
		{"ssh://ops:secret@edge", nil, true}, // a password in the URL is refused, as the docker CLI does
		{"tcp://edge:2375", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got, err := sshArgs(tt.host)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got  %v\nwant %v", got, tt.want)
			}
		})
	}
}

func TestCommandConnCarriesBytesBothWays(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available")
	}
	conn, err := dialCommand(context.Background(), "cat")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "ping\n" {
		t.Fatalf("read %q, %v", buf, err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	// After Close the process is gone and reads end.
	done := make(chan struct{})
	go func() { _, _ = conn.Read(buf); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("read still blocked after Close")
	}
}

func TestCommandConnReportsStartFailure(t *testing.T) {
	_, err := dialCommand(context.Background(), "d8s-no-such-binary")
	if err == nil || !strings.Contains(err.Error(), "d8s-no-such-binary") {
		t.Fatalf("err = %v", err)
	}
}
