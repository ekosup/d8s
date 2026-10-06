package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"gopkg.in/yaml.v3"

	"github.com/ekosup/d8s/internal/resource"
)

const fetchTimeout = 15 * time.Second

// capabilityBindings are the keys a view gets from what its resource can
// do beyond listing: inspect now, logs and shell as they are added.
func (a *App) capabilityBindings(res resource.Resource, view *tableView) []binding {
	var out []binding
	if res.Connect != nil {
		out = append(out, keyBinding(tcell.KeyEnter, "enter", "Use", func() {
			if row, ok := view.SelectedRow(); ok {
				if ep, ok := res.Connect(row); ok {
					a.switchTo(ep)
				}
			}
		}))
	}
	if res.Open != nil {
		out = append(out, keyBinding(tcell.KeyEnter, "enter", "Open", func() {
			if row, ok := view.SelectedRow(); ok {
				if child, ok := res.Open(row); ok {
					a.PushResource(child)
				}
			}
		}))
	}
	if res.Inspect != nil {
		out = append(out,
			runeBinding('d', "d", "Inspect", func() { a.openInspect(res, view, false) }),
			runeBinding('y', "y", "YAML", func() { a.openInspect(res, view, true) }),
		)
	}
	if res.Logs != nil {
		out = append(out, runeBinding('l', "l", "Logs", func() { a.openLogs(res, view) }))
	}
	if res.Exec != nil {
		out = append(out, runeBinding('s', "s", "Shell", func() { a.openShell(res, view) }))
	}
	for _, tp := range res.Pages {
		b, ok := parseKey(tp.Key)
		if !ok {
			continue
		}
		b.desc = tp.Name
		b.do = func() { a.openTextPage(tp, view) }
		out = append(out, b)
	}
	return out
}

// openTextPage fetches and shows one of a resource's extra text views. A
// page with a refresh interval keeps fetching while it is open.
func (a *App) openTextPage(tp resource.TextPage, view *tableView) {
	row, ok := view.SelectedRow()
	if !ok || a.client == nil {
		return
	}
	client := a.client
	fetch := func(ctx context.Context) ([]string, error) {
		ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
		defer cancel()
		return tp.Fetch(ctx, client, row)
	}
	label := strings.ToLower(tp.Name)
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		lines, err := fetch(ctx)
		if err != nil {
			cancel()
			a.queue(func() { a.Flash(flashError, label+" "+row.Name()+": "+oneLine(err.Error())) })
			return
		}
		p := newPager(tp.Name+": "+row.Name(), 0)
		a.queue(func() {
			p.SetLines(lines)
			a.pushPager(label, p, cancel)
		})
		if tp.Refresh <= 0 {
			return
		}
		tick := time.NewTicker(tp.Refresh)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				lines, err := fetch(ctx)
				if ctx.Err() != nil {
					return
				}
				a.queue(func() {
					if err != nil {
						a.Flash(flashError, label+" "+row.Name()+": "+oneLine(err.Error()))
						return
					}
					p.SetLines(lines)
				})
			}
		}
	}()
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
