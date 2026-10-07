package resource

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ekosup/d8s/internal/docker"
)

func TestContextRows(t *testing.T) {
	eps := []docker.Endpoint{
		{Context: "default", Host: docker.DefaultHost},
		{Context: "prod", Host: "tcp://10.0.0.5:2376", TLS: &docker.TLSFiles{CA: "/c/ca.pem"}},
	}
	res := Contexts(func() ([]docker.Endpoint, error) { return eps, nil }, func() string { return "prod" })
	rows, err := res.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"default | unix:///var/run/docker.sock | ", "prod | tcp://10.0.0.5:2376 | *"}
	if got := cellsOf(rows); !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
	if rows[1].Tone != ToneGood {
		t.Fatal("the active context should stand out")
	}

	ep, ok := res.Connect(rows[1])
	if !ok || ep.Host != "tcp://10.0.0.5:2376" || ep.TLS == nil || ep.TLS.CA != "/c/ca.pem" {
		t.Fatalf("Connect lost the endpoint details: %+v", ep)
	}
	if _, ok := res.Connect(Row{ID: "gone"}); ok {
		t.Fatal("Connect accepted an unknown context")
	}
}

func TestContextListError(t *testing.T) {
	boom := errors.New("boom")
	res := Contexts(func() ([]docker.Endpoint, error) { return nil, boom }, func() string { return "" })
	if _, err := res.List(context.Background(), nil); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestContextRowsShowPolicy(t *testing.T) {
	eps := []docker.Endpoint{
		{Context: "default", Host: docker.DefaultHost},
		{Context: "portalprod", Host: "ssh://portal1"},
		{Context: "scprod", Host: "ssh://scprod"},
	}
	policy := func(name string) ContextPolicy {
		switch name {
		case "portalprod":
			return ContextPolicy{ReadOnly: true, Production: true}
		case "scprod":
			return ContextPolicy{Production: true}
		}
		return ContextPolicy{}
	}
	res := Contexts(func() ([]docker.Endpoint, error) { return eps, nil }, func() string { return "default" }, WithContextPolicy(policy))
	if got, want := columnNames(res), []string{"NAME", "ENDPOINT", "MODE", "PROD", "ACTIVE"}; !slices.Equal(got, want) {
		t.Fatalf("columns %v, want %v", got, want)
	}
	rows, err := res.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"default | unix:///var/run/docker.sock | read-write |  | *",
		"portalprod | ssh://portal1 | read-only | yes | ",
		"scprod | ssh://scprod | read-write | yes | ",
	}
	if got := cellsOf(rows); !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
	// A production context that can still be changed is the one to notice.
	if rows[2].Tone != ToneWarn || rows[1].Tone != ToneNormal || rows[0].Tone != ToneGood {
		t.Fatalf("tones: %v %v %v", rows[0].Tone, rows[1].Tone, rows[2].Tone)
	}
}

func columnNames(res Resource) []string {
	names := make([]string, len(res.Columns))
	for i, c := range res.Columns {
		names[i] = c.Name
	}
	return names
}
