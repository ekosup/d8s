package resource

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

const (
	attrImageID = "image_id"
	attrRef     = "ref" // "repo:tag"; empty for a dangling image
	attrUsed    = "used"
)

// Images lists images, one row per tag like `docker images`.
func Images(now func() time.Time) Resource {
	return Resource{
		Name:    "images",
		Aliases: []string{"i", "image", "img"},
		Title:   "Images",
		Columns: []Column{
			{Name: "REPOSITORY"}, {Name: "TAG"}, {Name: "ID"}, {Name: "SIZE", Right: true}, {Name: "USED", Right: true}, {Name: "AGE"},
		},
		EventTypes: []string{"image", "container"},
		List: func(ctx context.Context, c docker.Client) ([]Row, error) {
			images, err := c.Images(ctx)
			if err != nil {
				return nil, err
			}
			containers, err := c.Containers(ctx)
			if err != nil {
				return nil, err
			}
			used := map[string]int{}
			for _, x := range containers {
				used[x.ImageID]++
			}
			var rows []Row
			for _, im := range images {
				refs := im.Tags
				if len(refs) == 0 {
					refs = []string{""}
				}
				for _, ref := range refs {
					rows = append(rows, imageRow(im, ref, used[im.ID], now()))
				}
			}
			return rows, nil
		},
		Actions: []Action{
			{
				Key: "ctrl-d", Name: "Delete", Confirm: true, Mutates: true,
				Run: func(ctx context.Context, c docker.Client, row Row) error {
					target := row.Attrs[attrRef]
					if target == "" {
						target = row.Attrs[attrImageID]
					}
					return c.Remove(ctx, docker.KindImage, target)
				},
				Warn: func(row Row) string {
					n, _ := strconv.Atoi(row.Attrs[attrUsed])
					if n == 0 {
						return ""
					}
					return fmt.Sprintf("It is used by %s; the daemon will refuse while they exist.", plural(n, "container"))
				},
			},
			{
				Key: "ctrl-p", Name: "Prune", Target: "dangling images", Confirm: true, Mutates: true,
				Run: func(ctx context.Context, c docker.Client, _ Row) error {
					_, err := c.Prune(ctx, docker.KindImage)
					return err
				},
			},
		},
		Inspect: func(ctx context.Context, c docker.Client, row Row) ([]byte, error) {
			return c.Inspect(ctx, docker.KindImage, row.Attrs[attrImageID])
		},
		Open: func(row Row) (Resource, bool) {
			id := row.Attrs[attrImageID]
			return containersWhere(now, "Containers(image "+row.Name()+")", func(x docker.Container) bool {
				return x.ImageID == id
			}), true
		},
		Pages: []TextPage{{
			Key: "h", Name: "History",
			Fetch: func(ctx context.Context, c docker.Client, row Row) ([]string, error) {
				layers, err := c.ImageHistory(ctx, row.Attrs[attrImageID])
				if err != nil {
					return nil, err
				}
				lines := []string{fmt.Sprintf("%-6s %10s  %s", "AGE", "SIZE", "CREATED BY")}
				for _, l := range layers {
					lines = append(lines, fmt.Sprintf("%-6s %10s  %s", humanAge(now().Sub(l.Created)), humanBytes(l.Size), l.CreatedBy))
				}
				return lines, nil
			},
		}},
	}
}

func imageRow(im docker.Image, ref string, used int, now time.Time) Row {
	repo, tag := "<none>", "<none>"
	id := im.ID
	if ref != "" {
		repo, tag = splitRef(ref)
		id = ref
	}
	age := now.Sub(im.Created)
	tone := ToneNormal
	if used == 0 {
		tone = ToneMuted
	}
	return Row{
		ID:    id,
		Cells: []string{repo, tag, shortID(im.ID), humanBytes(im.Size), strconv.Itoa(used), humanAge(age)},
		SortKeys: []string{"", "", "", fmt.Sprintf("%020d", max(im.Size, 0)), fmt.Sprintf("%010d", used),
			fmt.Sprintf("%020d", max(int64(age/time.Second), 0))},
		Tone:  tone,
		Attrs: map[string]string{attrImageID: im.ID, attrRef: ref, attrUsed: strconv.Itoa(used), attrName: imageName(ref, im.ID)},
	}
}

func imageName(ref, id string) string {
	if ref != "" {
		return ref
	}
	return shortID(id)
}

// splitRef separates "host:5000/team/app:1.2" into repository and tag. The
// tag separator is the last colon after the last slash.
func splitRef(ref string) (repo, tag string) {
	slash := strings.LastIndex(ref, "/")
	if colon := strings.LastIndex(ref, ":"); colon > slash {
		return ref[:colon], ref[colon+1:]
	}
	return ref, "<none>"
}

// shortID is the 12-character form of an object ID.
func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// humanBytes formats a size with binary units, one decimal.
func humanBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", max(n, 0))
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	v := float64(n) / 1024
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.1f%s", v, units[i])
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
