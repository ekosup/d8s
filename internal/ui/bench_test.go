//go:build bench && unix

package ui

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/ekosup/d8s/internal/docker"
	"github.com/ekosup/d8s/internal/docker/dockertest"
	"github.com/ekosup/d8s/internal/resource"
	"github.com/ekosup/d8s/internal/store"
)

// Performance targets from the product requirements. `make bench` runs
// this file against synthetic data served by the in-memory client, so the
// numbers measure d8s itself, not a daemon.
const (
	targetFirstTable = 300 * time.Millisecond
	targetKeypress   = 50 * time.Millisecond
	targetMemory     = 100 << 20 // bytes, with 1,000 containers loaded
	targetIdleCPU    = 2.0       // percent of one core
)

func syntheticContainers(n int) []docker.Container {
	cs := make([]docker.Container, n)
	for i := range cs {
		state, status := "running", "Up 3 hours"
		if i%5 == 0 {
			state, status = "exited", "Exited (0) 2 days ago"
		}
		cs[i] = docker.Container{
			ID: fmt.Sprintf("%064x", i), Name: fmt.Sprintf("svc-%04d-worker-%d", i%300, i),
			Image: fmt.Sprintf("registry.example.internal/team-%d/service-%d:1.%d.%d", i%12, i%300, i%40, i%7),
			State: state, Status: status, Created: time.Now().Add(-time.Duration(i) * time.Minute),
			Ports:  []docker.Port{{IP: "0.0.0.0", Public: uint16(20000 + i), Private: 8080, Proto: "tcp"}},
			Labels: map[string]string{"com.docker.compose.project": fmt.Sprintf("project-%d", i%40)},
		}
	}
	return cs
}

func syntheticSwarm(f *dockertest.Fake, services int) {
	f.SetSwarm(docker.SwarmInfo{Active: true, Manager: true, Leader: true, NodeID: "n0"})
	nodes := make([]docker.Node, 20)
	for i := range nodes {
		nodes[i] = docker.Node{ID: fmt.Sprintf("n%d", i), Hostname: fmt.Sprintf("node-%02d", i), State: "ready", Availability: "active", Role: "worker"}
	}
	f.SetNodes(nodes...)
	ss := make([]docker.Service, services)
	var ts []docker.Task
	for i := range ss {
		id := fmt.Sprintf("s%d", i)
		ss[i] = docker.Service{ID: id, Name: fmt.Sprintf("stack%02d_service-%d", i%25, i), Stack: fmt.Sprintf("stack%02d", i%25),
			Mode: "replicated", Image: fmt.Sprintf("registry.example.internal/app-%d:2.%d", i, i%9), Desired: 3, Running: 3, Created: time.Now()}
		for slot := 1; slot <= 3; slot++ {
			ts = append(ts, docker.Task{ID: fmt.Sprintf("t%d-%d", i, slot), ServiceID: id, Slot: slot, NodeID: fmt.Sprintf("n%d", (i+slot)%20),
				DesiredState: "running", State: "running", Image: ss[i].Image, Timestamp: time.Now()})
		}
	}
	f.SetServices(ss...)
	f.SetTasks(ts...)
}

// measured is one line of the report.
type measured struct {
	name, value, target string
	ok                  bool
}

func benchApp(fake *dockertest.Fake, info docker.Info, poll time.Duration) (*App, chan func()) {
	reg, err := resource.Default(time.Now)
	if err != nil {
		panic(err)
	}
	app := NewApp(info, WithClient(fake), WithRegistry(reg), WithWatchOptions(store.Options{Poll: poll}))
	queue := make(chan func(), 1024)
	app.queue = func(f func()) { queue <- f }
	return app, queue
}

func drawOnce(app *App, screen tcell.SimulationScreen) {
	w, h := screen.Size()
	app.root.SetRect(0, 0, w, h)
	app.root.Draw(screen)
	screen.Show()
}

func firstTable(t *testing.T, fake *dockertest.Fake, info docker.Info, view string) time.Duration {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(200, 50)

	start := time.Now()
	app, queue := benchApp(fake, info, time.Hour)
	defer app.Close()
	res, err := app.registry.Lookup(view)
	if err != nil {
		t.Fatal(err)
	}
	app.ShowResource(res)
	(<-queue)() // the first snapshot
	drawOnce(app, screen)
	return time.Since(start)
}

func cpuTime() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

func TestPerformanceTargets(t *testing.T) {
	var report []measured
	add := func(name, value, target string, ok bool) {
		report = append(report, measured{name, value, target, ok})
	}

	// 1. Time to the first table, large container list.
	containers := dockertest.NewFake(dockertest.WithContainers(syntheticContainers(2000)...))
	d := firstTable(t, containers, testInfo, "containers")
	add("first table, 2,000 containers", d.Round(time.Millisecond).String(), "< "+targetFirstTable.String(), d < targetFirstTable)

	// 2. Time to the first table, large swarm.
	swarm := dockertest.NewFake()
	syntheticSwarm(swarm, 500)
	swarmInfo := testInfo
	swarmInfo.Swarm = docker.SwarmInfo{Active: true, Manager: true, Leader: true, NodeID: "n0"}
	d = firstTable(t, swarm, swarmInfo, "services")
	add("first table, 500 services", d.Round(time.Millisecond).String(), "< "+targetFirstTable.String(), d < targetFirstTable)
	d = firstTable(t, swarm, swarmInfo, "tasks")
	add("first table, 1,500 tasks", d.Round(time.Millisecond).String(), "< "+targetFirstTable.String(), d < targetFirstTable)

	// 3. Key presses on a 2,000-row table: move, sort, filter, each with a redraw.
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(200, 50)
	app, queue := benchApp(containers, testInfo, time.Hour)
	res, _ := app.registry.Lookup("containers")
	app.ShowResource(res)
	(<-queue)()
	drawOnce(app, screen)
	keys := []struct {
		k tcell.Key
		r rune
	}{{tcell.KeyRune, 'j'}, {tcell.KeyRune, 'j'}, {tcell.KeyRune, 'G'}, {tcell.KeyRune, 'g'}, {tcell.KeyRune, 'I'}, {tcell.KeyRune, 'A'},
		{tcell.KeyRune, '/'}, {tcell.KeyRune, 's'}, {tcell.KeyRune, 'v'}, {tcell.KeyRune, 'c'}, {tcell.KeyEnter, 0}, {tcell.KeyEscape, 0}}
	var worst time.Duration
	for range 5 {
		for _, k := range keys {
			start := time.Now()
			app.key(k.k, k.r)
			drawOnce(app, screen)
			worst = max(worst, time.Since(start))
		}
	}
	app.Close()
	screen.Fini()
	add("slowest key press, 2,000 rows", worst.Round(time.Millisecond).String(), "< "+targetKeypress.String(), worst < targetKeypress)

	// 4. Memory with 1,000 containers on screen.
	runtime.GC()
	thousand := dockertest.NewFake(dockertest.WithContainers(syntheticContainers(1000)...))
	screen = tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(200, 50)
	app, queue = benchApp(thousand, testInfo, 2*time.Second)
	res, _ = app.registry.Lookup("containers")
	app.ShowResource(res)
	(<-queue)()
	drawOnce(app, screen)
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	// Sys is everything obtained from the OS: the closest to what `ps` reports.
	add("memory, 1,000 containers", fmt.Sprintf("%.1f MiB", float64(ms.Sys)/(1<<20)), fmt.Sprintf("< %d MiB", targetMemory>>20), ms.Sys < targetMemory)

	// 5. CPU while idle: the same view left alone, polling every 2 s.
	const idleFor = 8 * time.Second
	cpuStart, wallStart := cpuTime(), time.Now()
	deadline := time.After(idleFor)
idle:
	for {
		select {
		case f := <-queue:
			f()
			drawOnce(app, screen)
		case <-deadline:
			break idle
		}
	}
	pct := float64(cpuTime()-cpuStart) / float64(time.Since(wallStart)) * 100
	app.Close()
	screen.Fini()
	add("CPU while idle, 1,000 containers", fmt.Sprintf("%.2f%%", pct), fmt.Sprintf("< %.0f%%", targetIdleCPU), pct < targetIdleCPU)

	var sb strings.Builder
	failed := 0
	fmt.Fprintf(&sb, "\n%-36s %-12s %-10s %s\n", "MEASUREMENT", "RESULT", "TARGET", "")
	for _, m := range report {
		verdict := "pass"
		if !m.ok {
			verdict = "FAIL"
			failed++
		}
		fmt.Fprintf(&sb, "%-36s %-12s %-10s %s\n", m.name, m.value, m.target, verdict)
	}
	fmt.Fprintf(&sb, "\nSynthetic data from the in-memory client; stats columns are off above %d running containers.\n", 100)
	fmt.Fprint(os.Stdout, sb.String())
	if failed > 0 {
		t.Fatalf("%d of %d targets missed", failed, len(report))
	}
}
