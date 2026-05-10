package main

import (
	"math/rand"
	"runtime"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/battery"
	"github.com/oldwired/fv-go/pkg/fv/widgets/cpucore"
	"github.com/oldwired/fv-go/pkg/fv/widgets/cpumeter"
	"github.com/oldwired/fv-go/pkg/fv/widgets/diskusage"
	"github.com/oldwired/fv-go/pkg/fv/widgets/editor"
	"github.com/oldwired/fv-go/pkg/fv/widgets/editorgutter"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"
	"github.com/oldwired/fv-go/pkg/fv/widgets/logviewer"
	"github.com/oldwired/fv-go/pkg/fv/widgets/markdown"
	"github.com/oldwired/fv-go/pkg/fv/widgets/network"
	"github.com/oldwired/fv-go/pkg/fv/widgets/process"
	"github.com/oldwired/fv-go/pkg/fv/widgets/ramview"
	"github.com/oldwired/fv-go/pkg/fv/widgets/syntax"
	"github.com/oldwired/fv-go/pkg/fv/widgets/uptime"
)

// Demo command IDs for the new batch.
const (
	cmAppEditorWithGutter uint16 = 250 + iota
	cmAppEditorSyntax
	cmAppLogViewer
	cmAppMarkdown
	cmAppFuzzy

	cmAppUptime
	cmAppCPUMeter
	cmAppCPUCores
	cmAppRAM
	cmAppDisk
	cmAppBattery
	cmAppNetwork
	cmAppProcess
)

// dispatchExtension routes the new menu items.
func dispatchExtension(a *app.Application, cmd uint16) bool {
	switch cmd {
	case cmAppEditorWithGutter:
		showEditorGutter(a)
	case cmAppEditorSyntax:
		showEditorSyntax(a)
	case cmAppLogViewer:
		showLogViewer(a)
	case cmAppMarkdown:
		showMarkdown(a)
	case cmAppFuzzy:
		showFuzzyFinder(a)
	case cmAppUptime:
		showUptime(a)
	case cmAppCPUMeter:
		showCPUMeter(a)
	case cmAppCPUCores:
		showCPUCores(a)
	case cmAppRAM:
		showRAM(a)
	case cmAppDisk:
		showDisk(a)
	case cmAppBattery:
		showBattery(a)
	case cmAppNetwork:
		showNetwork(a)
	case cmAppProcess:
		showProcess(a)
	default:
		return false
	}
	return true
}

// --- Editor extensions ----------------------------------------------

// showEditorGutter opens an editor with line numbers + a clickable
// breakpoints column on its left.
func showEditorGutter(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 76, 22), "Editor + Gutter", 0)
	w, h := win.Size.X, win.Size.Y
	scroll := views.NewScrollBar(geom.NewRect(w-2, 1, w-1, h-1))
	win.Insert(scroll)

	const gutterW = 8
	ed := editor.New(geom.NewRect(1+gutterW, 1, w-2, h-1), nil, scroll)
	ed.SetText(loremIpsum())
	win.Insert(ed)

	bps := editorgutter.NewBreakpoints()
	g := editorgutter.New(geom.NewRect(1, 1, 1+gutterW, h-1), ed,
		editorgutter.NewLineNumbers(4), bps)
	g.OnClick = func(line int) { bps.Toggle(line) }
	win.Insert(g)

	a.Desktop.InsertWindow(win)
}

// showEditorSyntax opens the editor with Go-syntax coloring on a
// preset Go snippet.
func showEditorSyntax(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 78, 22), "Editor + Syntax (Go)", 0)
	w, h := win.Size.X, win.Size.Y
	scroll := views.NewScrollBar(geom.NewRect(w-2, 1, w-1, h-1))
	win.Insert(scroll)
	ed := editor.New(geom.NewRect(1, 1, w-2, h-1), nil, scroll)
	ed.Colorer = syntax.GoSyntax().ToEditorColorer()
	ed.SetText(`// Sample Go code with syntax highlighting.
package main

import "fmt"

// Greet says hello to the world.
func Greet(name string) string {
    if name == "" {
        return "Hello, world!"
    }
    return fmt.Sprintf("Hello, %s!", name)
}

func main() {
    for i := 0; i < 3; i++ {
        fmt.Println(Greet("fv-go"))
    }
}
`)
	win.Insert(ed)
	a.Desktop.InsertWindow(win)
}

func loremIpsum() string {
	return `Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.
Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.
Duis aute irure dolor in reprehenderit in voluptate velit esse.
Excepteur sint occaecat cupidatat non proident, sunt in culpa qui.
Officia deserunt mollit anim id est laborum.
Click any line in the gutter to toggle a breakpoint dot.
`
}

// --- LogViewer ------------------------------------------------------

func showLogViewer(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 80, 20), "Log Viewer", 0)
	w, h := win.Size.X, win.Size.Y
	scroll := views.NewScrollBar(geom.NewRect(w-2, 1, w-1, h-1))
	win.Insert(scroll)
	lv := logviewer.New(geom.NewRect(1, 1, w-2, h-1), nil, scroll)
	win.Insert(lv)
	// Seed some entries.
	now := time.Now()
	lv.AppendAt(now.Add(-3*time.Second), logviewer.LevelInfo, "boot", "Application starting up")
	lv.AppendAt(now.Add(-2*time.Second), logviewer.LevelDebug, "config", "Loaded settings from ~/.config/fv-go")
	lv.AppendAt(now.Add(-1500*time.Millisecond), logviewer.LevelWarn, "auth", "Token cache nearing expiry")
	lv.AppendAt(now.Add(-1*time.Second), logviewer.LevelInfo, "main", "Listening on 127.0.0.1:8080")
	lv.AppendAt(now, logviewer.LevelError, "db", "Connection refused (after 5 retries)")
	a.Desktop.InsertWindow(win)
}

// --- MarkdownView ---------------------------------------------------

func showMarkdown(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 70, 22), "Markdown", 0)
	w, h := win.Size.X, win.Size.Y
	scroll := views.NewScrollBar(geom.NewRect(w-2, 1, w-1, h-1))
	win.Insert(scroll)
	md := markdown.New(geom.NewRect(1, 1, w-2, h-1), scroll)
	md.SetMarkdown(`# Markdown Showcase

This widget renders a basic subset of Markdown.

## Inline styles

Regular text, **bold text**, *italic text*, and ` + "`inline code`" + `.

## Lists

- First bullet point
- Second one
- Third one

1. Numbered first
2. Numbered second

## Code block

` + "```" + `
func main() {
    fmt.Println("Hello, world!")
}
` + "```" + `

## Blockquote

> "Markdown is a way to write…"

## Link

Visit [the project](https://example.com/fv-go) for more.

---

End of demo.`)
	win.Insert(md)
	a.Desktop.InsertWindow(win)
}

// --- FuzzyFinder ----------------------------------------------------

func showFuzzyFinder(a *app.Application) {
	items := []string{
		"cmd/fvdemo/main.go",
		"cmd/fvdemo/widgets.go",
		"cmd/fvdemo/apps.go",
		"pkg/fv/views/group.go",
		"pkg/fv/views/window.go",
		"pkg/fv/views/scroll.go",
		"pkg/fv/dialogs/dialog.go",
		"pkg/fv/dialogs/inputline.go",
		"pkg/fv/widgets/editor/editor.go",
		"pkg/fv/widgets/grid/grid.go",
		"pkg/fv/widgets/hexedit/hexedit.go",
		"pkg/fv/widgets/syntax/syntax.go",
		"pkg/fv/widgets/markdown/markdown.go",
		"pkg/fv/widgets/logviewer/logviewer.go",
	}
	ff := fuzzyfinder.New(geom.NewRect(8, 4, 60, 16), items)
	idx := ff.Run(&a.Desktop.Group)
	if idx >= 0 {
		// Use msgbox via the existing dispatchWidget infrastructure;
		// here just cheat with a short-lived dialog.
		d := dialogs.NewDialog(geom.NewRect(0, 0, 50, 7), "Picked")
		d.Insert(dialogs.NewStaticText(geom.NewRect(2, 2, 48, 4), "You picked: "+items[idx]))
		d.Insert(dialogs.NewButton(geom.NewRect(20, 4, 30, 5), "O~K~", consts.CmOK, dialogs.BfDefault))
		a.Desktop.ExecView(d)
	}
}

// --- System gadgets -------------------------------------------------

func showUptime(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 30, 6), "Uptime", 0)
	w, h := win.Size.X, win.Size.Y
	u := uptime.New(geom.NewRect(1, 1, w-1, h-1))
	win.Insert(u)
	a.Desktop.InsertWindow(win)
}

func showCPUMeter(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 50, 6), "CPU Meter", 0)
	w, h := win.Size.X, win.Size.Y
	level := 0.5
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	c := cpumeter.New(geom.NewRect(1, 1, w-1, h-1), func() float64 {
		level += (rng.Float64() - 0.5) * 0.2
		if level < 0.05 {
			level = 0.05
		}
		if level > 0.98 {
			level = 0.98
		}
		return level
	}, 200*time.Millisecond)
	win.Insert(c)
	a.Desktop.InsertWindow(win)
}

func showCPUCores(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 60, 12), "CPU Cores", 0)
	w, h := win.Size.X, win.Size.Y
	cores := runtime.NumCPU()
	if cores > 8 {
		cores = 8
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	state := make([]float64, cores)
	for i := range state {
		state[i] = 0.5
	}
	v := cpucore.New(geom.NewRect(1, 1, w-1, h-1), func() []float64 {
		for i := range state {
			state[i] += (rng.Float64() - 0.5) * 0.3
			if state[i] < 0 {
				state[i] = 0
			}
			if state[i] > 1 {
				state[i] = 1
			}
		}
		return state
	}, 250*time.Millisecond)
	win.Insert(v)
	a.Desktop.InsertWindow(win)
}

func showRAM(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 50, 6), "RAM", 0)
	w, h := win.Size.X, win.Size.Y
	r := ramview.New(geom.NewRect(1, 1, w-1, h-1), func() ramview.Stats {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return ramview.Stats{Used: ms.Alloc, Total: ms.Sys + 64*1024*1024}
	}, time.Second)
	win.Insert(r)
	a.Desktop.InsertWindow(win)
}

func showDisk(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 70, 8), "Disk Usage", 0)
	w, h := win.Size.X, win.Size.Y
	d := diskusage.New(geom.NewRect(1, 1, w-1, h-1), func() []diskusage.Volume {
		// Synthetic; real usage would call syscall.Statfs / Win32 APIs.
		return []diskusage.Volume{
			{Path: "/", Used: 80 * 1024 * 1024 * 1024, Total: 240 * 1024 * 1024 * 1024},
			{Path: "/Users", Used: 142 * 1024 * 1024 * 1024, Total: 240 * 1024 * 1024 * 1024},
			{Path: "/tmp", Used: 1024 * 1024 * 1024, Total: 4 * 1024 * 1024 * 1024},
		}
	}, 5*time.Second)
	win.Insert(d)
	a.Desktop.InsertWindow(win)
}

func showBattery(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 40, 6), "Battery", 0)
	w, h := win.Size.X, win.Size.Y
	pct := 0.85
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := battery.New(geom.NewRect(1, 1, w-1, h-1), func() battery.Status {
		pct -= rng.Float64() * 0.005
		if pct < 0 {
			pct = 1.0
		}
		return battery.Status{Charge: pct, OnAC: false, Charging: false}
	}, 2*time.Second)
	win.Insert(b)
	a.Desktop.InsertWindow(win)
}

func showNetwork(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 60, 8), "Network", 0)
	w, h := win.Size.X, win.Size.Y
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	n := network.New(geom.NewRect(1, 1, w-1, h-1), func() []network.Interface {
		return []network.Interface{
			{Name: "en0", BytesInPS: rng.Float64() * 1024 * 1024 * 8, BytesOutPS: rng.Float64() * 1024 * 512},
			{Name: "lo0", BytesInPS: rng.Float64() * 1024, BytesOutPS: rng.Float64() * 1024},
			{Name: "tun0", BytesInPS: rng.Float64() * 1024 * 64, BytesOutPS: rng.Float64() * 1024 * 32},
		}
	}, time.Second)
	win.Insert(n)
	a.Desktop.InsertWindow(win)
}

func showProcess(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 60, 14), "Processes", 0)
	w, h := win.Size.X, win.Size.Y
	procs := []process.Process{
		{PID: 1, CPU: 0.01, Mem: 0.005, Command: "init"},
		{PID: 117, CPU: 0.05, Mem: 0.014, Command: "kernel_task"},
		{PID: 432, CPU: 0.20, Mem: 0.080, Command: "fvdemo"},
		{PID: 891, CPU: 0.42, Mem: 0.230, Command: "code"},
		{PID: 1024, CPU: 0.08, Mem: 0.040, Command: "Terminal"},
		{PID: 1502, CPU: 0.01, Mem: 0.012, Command: "WindowServer"},
	}
	p := process.New(geom.NewRect(1, 1, w-1, h-1), func() []process.Process {
		// Wiggle the values.
		for i := range procs {
			procs[i].CPU = jitter(procs[i].CPU, 0.05)
			procs[i].Mem = jitter(procs[i].Mem, 0.01)
		}
		return procs
	}, 800*time.Millisecond)
	win.Insert(p)
	a.Desktop.InsertWindow(win)
}

func jitter(v, amp float64) float64 {
	v += (rand.Float64() - 0.5) * amp
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
