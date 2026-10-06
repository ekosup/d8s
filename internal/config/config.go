// Package config reads the user's settings file.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Skins that ship with d8s.
var Skins = []string{"dark", "light", "mono"}

// Limits a setting must respect to be usable.
const (
	MinRefresh   = 500 * time.Millisecond
	MinLogBuffer = 100
)

// Duration is a time.Duration written like "2s" or "500ms".
type Duration time.Duration

// Std returns the value as a time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	v, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("line %d: %q is not a duration; write it like 2s or 500ms", node.Line, node.Value)
	}
	*d = Duration(v)
	return nil
}

// ContextPolicy is what the user decided about one Docker context.
type ContextPolicy struct {
	ReadOnly   bool `yaml:"readOnly"`   // refuse every change on this context
	Production bool `yaml:"production"` // make it stand out in the header
}

// View customises one resource view.
type View struct {
	Columns []string `yaml:"columns"` // columns to show, in this order; empty = all
}

// Config is the content of the settings file.
type Config struct {
	Refresh     Duration                 `yaml:"refresh"`     // how often a view polls
	DefaultView string                   `yaml:"defaultView"` // view shown at start; "auto" = services on a swarm manager, else containers
	LogBuffer   int                      `yaml:"logBuffer"`   // lines a log page keeps
	LogTail     int                      `yaml:"logTail"`     // lines fetched when a log page opens
	Shell       string                   `yaml:"shell"`       // shell to run in containers; empty = bash, else sh
	ReadOnly    bool                     `yaml:"readOnly"`    // refuse every change, on every context
	Skin        string                   `yaml:"skin"`
	Contexts    map[string]ContextPolicy `yaml:"contexts"`
	Aliases     map[string]string        `yaml:"aliases"` // command -> "view" or "view /filter"
	Hotkeys     map[string]string        `yaml:"hotkeys"` // key -> command
	Views       map[string]View          `yaml:"views"`
}

// Default returns the settings used when the file says nothing.
func Default() Config {
	return Config{
		Refresh:     Duration(2 * time.Second),
		DefaultView: "auto",
		LogBuffer:   5000,
		LogTail:     1000,
		Skin:        "dark",
	}
}

// Policy returns what applies to a context, combining its own entry with
// the global read-only setting and the --readonly flag.
func (c Config) Policy(context string, readOnlyFlag bool) ContextPolicy {
	p := c.Contexts[context]
	p.ReadOnly = p.ReadOnly || c.ReadOnly || readOnlyFlag
	return p
}

// Path returns where the settings file is looked for: $D8S_CONFIG, else
// the XDG config directory.
func Path(getenv func(string) string) string {
	if p := getenv("D8S_CONFIG"); p != "" {
		return p
	}
	base := getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(getenv("HOME"), ".config")
	}
	return filepath.Join(base, "d8s", "config.yaml")
}

// Load reads the file at path on top of the defaults. A missing or empty
// file is not an error. Any problem is reported with the file, the line
// and the offending text.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // a misspelt key is a mistake, not something to ignore
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return Default(), located(path, data, err.Error())
	}
	if msg := cfg.validate(data); msg != "" {
		return Default(), located(path, data, msg)
	}
	return cfg, nil
}

var aliasName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// validate checks values the type system cannot. It returns a message
// starting with "line N: ", or "".
func (c Config) validate(data []byte) string {
	lines := keyLines(data)
	at := func(key string) string { return fmt.Sprintf("line %d: ", lines[key]) }
	switch {
	case c.Refresh.Std() < MinRefresh:
		return at("refresh") + fmt.Sprintf("refresh must be at least %s", MinRefresh)
	case c.LogBuffer < MinLogBuffer:
		return at("logBuffer") + fmt.Sprintf("logBuffer must be at least %d", MinLogBuffer)
	case c.LogTail < 0:
		return at("logTail") + "logTail cannot be negative"
	case !slices.Contains(Skins, c.Skin):
		return at("skin") + fmt.Sprintf("unknown skin %q; choose one of %s", c.Skin, strings.Join(Skins, ", "))
	}
	for name, target := range c.Aliases {
		switch {
		case !aliasName.MatchString(name):
			return at("aliases."+name) + fmt.Sprintf("alias %q must be lowercase letters, digits, - or _", name)
		case strings.TrimSpace(target) == "":
			return at("aliases."+name) + fmt.Sprintf("alias %q has no command to run", name)
		}
	}
	for key, cmd := range c.Hotkeys {
		if strings.TrimSpace(cmd) == "" {
			return at("hotkeys."+key) + fmt.Sprintf("hotkey %q has no command to run", key)
		}
	}
	return ""
}

// keyLines maps "key" and "section.key" to the line each is written on.
func keyLines(data []byte) map[string]int {
	out := map[string]int{}
	var root yaml.Node
	if yaml.Unmarshal(data, &root) != nil || len(root.Content) == 0 {
		return out
	}
	doc := root.Content[0]
	for i := 0; i+1 < len(doc.Content); i += 2 {
		key, val := doc.Content[i], doc.Content[i+1]
		out[key.Value] = key.Line
		if val.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(val.Content); j += 2 {
				out[key.Value+"."+val.Content[j].Value] = val.Content[j].Line
			}
		}
	}
	return out
}

var lineRef = regexp.MustCompile(`line (\d+):`)

// located turns a message containing "line N:" into an error that names
// the file and quotes that line.
func located(path string, data []byte, msg string) error {
	msg = strings.TrimPrefix(msg, "yaml: ")
	msg = strings.TrimPrefix(msg, "unmarshal errors:\n  ")
	m := lineRef.FindStringSubmatch(msg)
	if m == nil {
		return fmt.Errorf("%s: %s", path, msg)
	}
	n, _ := strconv.Atoi(m[1])
	// The YAML library counts lines from zero for errors raised by its
	// parser ("did not find expected ...") and from one for all others.
	if strings.Contains(msg, "did not find expected") {
		n++
	}
	source := ""
	if lines := strings.Split(string(data), "\n"); n >= 1 && n <= len(lines) {
		source = strings.TrimSpace(lines[n-1])
	}
	rest := strings.TrimSpace(strings.Replace(msg, m[0], "", 1))
	if source == "" {
		return fmt.Errorf("%s: line %d: %s", path, n, rest)
	}
	return fmt.Errorf("%s: line %d: `%s`: %s", path, n, source, rest)
}
