package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

// AnyEvent in EventTypes means every daemon event triggers a refresh.
const AnyEvent = "*"

// Events lists recent daemon events, newest first. recent supplies them;
// loc is the time zone they are shown in.
func Events(recent func() []docker.Event, loc *time.Location) Resource {
	return Resource{
		Name:       "events",
		Aliases:    []string{"ev", "event"},
		Title:      "Events",
		Columns:    []Column{{Name: "TIME"}, {Name: "TYPE"}, {Name: "ACTION"}, {Name: "NAME"}, {Name: "ID"}},
		EventTypes: []string{AnyEvent},
		SortColumn: 0,
		SortDesc:   true,
		List: func(context.Context, docker.Client) ([]Row, error) {
			events := recent()
			rows := make([]Row, 0, len(events))
			for i, ev := range events {
				// The index keeps events of the same instant apart and in order.
				key := fmt.Sprintf("%020d-%06d", ev.Time.UnixNano(), i)
				tone := ToneNormal
				switch ev.Action {
				case "die", "kill", "oom", "destroy":
					tone = ToneWarn
				}
				rows = append(rows, Row{
					ID:       key,
					Cells:    []string{ev.Time.In(loc).Format("15:04:05"), ev.Type, ev.Action, ev.Name, shortID(ev.ID)},
					SortKeys: []string{key, "", "", "", ""},
					Tone:     tone,
				})
			}
			return rows, nil
		},
	}
}
