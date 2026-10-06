package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ekosup/d8s/internal/resource"
)

const fetchTimeout = 15 * time.Second

// capabilityBindings are the keys a view gets from what its resource can
// do beyond listing: inspect now, logs and shell as they are added.
func (a *App) capabilityBindings(res resource.Resource, view *tableView) []binding {
	var out []binding
	if res.Inspect != nil {
		out = append(out,
			runeBinding('d', "d", "Inspect", func() { a.openInspect(res, view, false) }),
			runeBinding('y', "y", "YAML", func() { a.openInspect(res, view, true) }),
		)
	}
	if res.Logs != nil {
		out = append(out, runeBinding('l', "l", "Logs", func() { a.openLogs(res, view) }))
	}
	return out
}

// openInspect fetches the selected object's description and shows it as
// indented JSON, or as YAML.
func (a *App) openInspect(res resource.Resource, view *tableView, asYAML bool) {
	row, ok := view.SelectedRow()
	if !ok || a.client == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		raw, err := res.Inspect(ctx, a.client, row)
		text, label, name := "", "Inspect", "inspect"
		if err == nil {
			if asYAML {
				label, name = "YAML", "yaml"
				text, err = jsonToYAML(raw)
			} else {
				text, err = indentJSON(raw)
			}
		}
		a.queue(func() {
			if err != nil {
				a.Flash(flashError, "inspect "+row.Name()+": "+oneLine(err.Error()))
				return
			}
			p := newPager(label+": "+row.Name(), 0)
			p.SetLines(strings.Split(strings.TrimRight(text, "\n"), "\n"))
			a.pushPager(name, p, nil)
		})
	}()
}

func indentJSON(raw []byte) (string, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return "", fmt.Errorf("format JSON: %w", err)
	}
	return buf.String(), nil
}

// jsonToYAML converts JSON to YAML, keeping numbers as numbers.
func jsonToYAML(raw []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // float64 would mangle large integers such as byte counts
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", fmt.Errorf("parse JSON: %w", err)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(yamlValue(v)); err != nil {
		return "", fmt.Errorf("format YAML: %w", err)
	}
	return buf.String(), nil
}

func yamlValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = yamlValue(e)
		}
	case []any:
		for i, e := range x {
			x[i] = yamlValue(e)
		}
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if u, err := strconv.ParseUint(x.String(), 10, 64); err == nil {
			return u
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String()
	}
	return v
}
