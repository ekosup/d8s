package resource

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ekosup/d8s/internal/docker"
)

const (
	// maxStatsContainers bounds the per-refresh cost: one API call per
	// running container. Beyond it the columns show "-".
	maxStatsContainers = 100
	statsConcurrency   = 8
	statsTimeout       = 3 * time.Second
	statsPageRefresh   = time.Second
)

// usage is a sample turned into what the UI shows.
type usage struct {
	docker.Stats
	cpu    float64 // percent of one CPU; 100 = one full core
	hasCPU bool    // false until there are two samples to compare
}

// statsSampler remembers the previous sample of each container, which is
// what turns cumulative CPU counters into a percentage.
type statsSampler struct {
	mu   sync.Mutex
	prev map[string]docker.Stats
}

func (s *statsSampler) sample(ctx context.Context, c docker.Client, id string) (usage, error) {
	ctx, cancel := context.WithTimeout(ctx, statsTimeout)
	defer cancel()
	cur, err := c.ContainerStats(ctx, id)
	if err != nil {
		return usage{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u := usage{Stats: cur}
	if prev, ok := s.prev[id]; ok && cur.CPUSystem > prev.CPUSystem && cur.CPUTotal >= prev.CPUTotal {
		u.cpu = float64(cur.CPUTotal-prev.CPUTotal) / float64(cur.CPUSystem-prev.CPUSystem) * float64(max(cur.OnlineCPUs, 1)) * 100
		u.hasCPU = true
	}
	if s.prev == nil {
		s.prev = map[string]docker.Stats{}
	}
	s.prev[id] = cur
	return u, nil
}

// forget drops samples of containers that are gone.
func (s *statsSampler) forget(keep map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.prev {
		if !keep[id] {
			delete(s.prev, id)
		}
	}
}

// WithStats adds live CPU and memory columns and a stats page to a
// container resource.
func WithStats(res Resource) Resource {
	sampler := &statsSampler{}
	at := len(res.Columns) - 2 // before PORTS and AGE
	res = withColumns(res, at, []Column{{Name: "CPU%", Right: true}, {Name: "MEM", Right: true}},
		func(ctx context.Context, c docker.Client, cs []docker.Container) map[string]extraCells {
			running := map[string]bool{}
			for _, x := range cs {
				if x.State == "running" {
					running[x.ID] = true
				}
			}
			sampler.forget(running)
			if len(running) > maxStatsContainers {
				return nil
			}
			var (
				mu  sync.Mutex
				wg  sync.WaitGroup
				sem = make(chan struct{}, statsConcurrency)
				out = make(map[string]extraCells, len(running))
			)
			for id := range running {
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer wg.Done()
					defer func() { <-sem }()
					u, err := sampler.sample(ctx, c, id)
					if err != nil {
						return // the row keeps "-"; the list itself must not fail
					}
					e := extraCells{
						cells: []string{"-", humanBytes(int64(u.MemUsage))},
						keys:  []string{"", fmt.Sprintf("%020d", u.MemUsage)},
					}
					if u.hasCPU {
						e.cells[0] = fmt.Sprintf("%.1f", u.cpu)
						e.keys[0] = fmt.Sprintf("%020.3f", u.cpu)
					}
					mu.Lock()
					out[id] = e
					mu.Unlock()
				}()
			}
			wg.Wait()
			return out
		})

	res.Pages = append(res.Pages, TextPage{
		Key: "m", Name: "Stats", Refresh: statsPageRefresh,
		Fetch: func(ctx context.Context, c docker.Client, row Row) ([]string, error) {
			u, err := sampler.sample(ctx, c, row.ID)
			if err != nil {
				return nil, err
			}
			cpu := "measuring…"
			if u.hasCPU {
				cpu = fmt.Sprintf("%.1f%%", u.cpu)
			}
			mem := humanBytes(int64(u.MemUsage))
			if u.MemLimit > 0 {
				mem = fmt.Sprintf("%s / %s (%.1f%%)", mem, humanBytes(int64(u.MemLimit)), float64(u.MemUsage)/float64(u.MemLimit)*100)
			}
			return []string{
				fmt.Sprintf("%-10s %s  (%s available)", "CPU", cpu, plural(int(u.OnlineCPUs), "CPU")),
				fmt.Sprintf("%-10s %s", "MEMORY", mem),
				fmt.Sprintf("%-10s received %s, sent %s", "NETWORK", humanBytes(int64(u.NetRx)), humanBytes(int64(u.NetTx))),
				fmt.Sprintf("%-10s read %s, written %s", "BLOCK I/O", humanBytes(int64(u.BlkRead)), humanBytes(int64(u.BlkWrite))),
				fmt.Sprintf("%-10s %d", "PROCESSES", u.PIDs),
			}, nil
		},
	})
	return res
}
