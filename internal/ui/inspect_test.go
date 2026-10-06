package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
)

const webInspect = `{"Id":"1","Name":"/web","Config":{"Image":"nginx:alpine","Cmd":["nginx","-g","daemon off;"],"Tty":false},"RestartCount":3,"Mounts":[{"Type":"volume","Destination":"/data"}]}`

func inspectHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, fastWatch, docker.Container{ID: "1", Name: "web", Image: "nginx:alpine", State: "running", Created: created})
	h.fake.SetInspect(docker.KindContainer, "1", []byte(webInspect))
	h.show("containers")
	h.step()
	return h
}

func TestInspectShowsJSON(t *testing.T) {
	h := inspectHarness(t)
	h.app.key(tcell.KeyRune, 'd')
	h.until("inspect page", func() bool { return len(h.app.stack) == 2 })
	s := h.screen()
	for _, want := range []string{"Inspect: web", `"Image": "nginx:alpine"`, `"RestartCount": 3`, "<containers> <inspect>"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
	h.app.key(tcell.KeyEscape, 0)
	if !strings.Contains(h.screen(), "Containers[1]") {
		t.Fatal("esc did not return to the table")
	}
}

func TestInspectShowsYAML(t *testing.T) {
	h := inspectHarness(t)
	h.app.key(tcell.KeyRune, 'y')
	h.until("yaml page", func() bool { return len(h.app.stack) == 2 })
	s := h.screen()
	for _, want := range []string{"YAML: web", "Config:", "Image: nginx:alpine", "RestartCount: 3", "- nginx", "Tty: false"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
}

func TestInspectSearch(t *testing.T) {
	h := inspectHarness(t)
	h.app.key(tcell.KeyRune, 'd')
	h.until("inspect page", func() bool { return len(h.app.stack) == 2 })
	h.app.key(tcell.KeyRune, '/')
	h.app.typeText("Mounts")
	h.app.key(tcell.KeyEnter, 0)
	if s := h.screen(); !strings.Contains(s, "[1/1]") || !strings.Contains(s, `"Mounts"`) {
		t.Fatalf("search in inspect failed:\n%s", s)
	}
}

func TestInspectErrorIsShown(t *testing.T) {
	h := inspectHarness(t)
	h.fake.SetInspectError(errors.New("no such container"))
	h.app.key(tcell.KeyRune, 'd')
	h.until("error", func() bool { return strings.Contains(h.screen(), "no such container") })
	if len(h.app.stack) != 1 {
		t.Fatal("a page was opened despite the error")
	}
}

func TestJSONToYAMLKeepsNumbers(t *testing.T) {
	out, err := jsonToYAML([]byte(`{"a":1,"b":1.5,"c":"7","d":12345678901234567890,"e":null,"f":[true]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"a: 1\n", "b: 1.5\n", `c: "7"`, "d: 12345678901234567890\n", "e: null\n", "- true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
