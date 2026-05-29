package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/profile"
	"github.com/oldwired/fv-go/pkg/fv/sixel"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/battery"
	"github.com/oldwired/fv-go/pkg/fv/widgets/cpucore"
	"github.com/oldwired/fv-go/pkg/fv/widgets/cpumeter"
	"github.com/oldwired/fv-go/pkg/fv/widgets/diskusage"
	"github.com/oldwired/fv-go/pkg/fv/widgets/editor"
	"github.com/oldwired/fv-go/pkg/fv/widgets/editorgutter"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"
	"github.com/oldwired/fv-go/pkg/fv/widgets/hyperlink"
	"github.com/oldwired/fv-go/pkg/fv/widgets/imageview"
	"github.com/oldwired/fv-go/pkg/fv/widgets/logviewer"
	"github.com/oldwired/fv-go/pkg/fv/widgets/markdown"
	"github.com/oldwired/fv-go/pkg/fv/widgets/network"
	"github.com/oldwired/fv-go/pkg/fv/widgets/process"
	"github.com/oldwired/fv-go/pkg/fv/widgets/ramview"
	"github.com/oldwired/fv-go/pkg/fv/widgets/sixelcanvas"
	"github.com/oldwired/fv-go/pkg/fv/widgets/stddlg"
	"github.com/oldwired/fv-go/pkg/fv/widgets/syntax"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"
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
	cmAppImageView
	cmAppImageViewOpen
	cmAppCanvas
	cmAppTerminal
	cmTerminalChildExited
	cmAppProfileDump
	cmAppHyperlink
	cmAppUnicode
	cmAppSaveDesktop
	cmAppLoadDesktop
	cmAppGameOfLife
)

// dispatchExtension routes the new menu items. The event is needed so
// commands that carry InfoPtr (e.g. cmTerminalChildExited carrying a
// *Window) can read it.
func dispatchExtension(a *app.Application, cmd uint16, ev *drivers.Event) bool {
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
	case cmAppImageView:
		showImageView(a)
	case cmAppImageViewOpen:
		showImageViewOpen(a)
	case cmAppCanvas:
		showCanvas(a)
	case cmAppTerminal:
		showTerminal(a)
	case cmTerminalChildExited:
		// Custom event posted by the terminal view's OnExit goroutine.
		// InfoPtr is the *Window to remove — we couldn't call Delete
		// directly from a goroutine (the view tree isn't goroutine-safe).
		// Routing it through the queue lets the main loop deliver the
		// removal at a safe point.
		if win, ok := ev.InfoPtr.(*views.Window); ok {
			a.Desktop.Delete(win.Self())
		}
	case cmAppProfileDump:
		showProfileDump(a)
	case cmAppHyperlink:
		showHyperlinkDemo(a)
	case cmAppUnicode:
		showUnicodeDemo(a)
	case cmAppSaveDesktop:
		saveDesktopFile(a)
	case cmAppLoadDesktop:
		loadDesktopFile(a)
	case cmAppGameOfLife:
		showGameOfLife(a)
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
	win := views.NewWindow(geom.NewRect(1, 1, 78, 24), "Markdown", 0)
	w, h := win.Size.X, win.Size.Y
	scroll := views.NewScrollBar(geom.NewRect(w-2, 1, w-1, h-1))
	win.Insert(scroll)
	md := markdown.New(geom.NewRect(1, 1, w-2, h-1), scroll)
	md.SetMarkdown(`# Markdown Showcase

ATX H1 above; Setext styles below.

Setext H1
=========

Setext H2
---------

## Inline styles

Plain text, **bold**, __also bold__, *italic*, _also italic_,
` + "`inline code`" + `, ~~strikethrough~~. Escapes work too:
\*literal stars\* and snake_case_var stay literal.

## Lists

- Dash bullet
* Star bullet
+ Plus bullet

1. Numbered first
2. Numbered second

## Code

Fenced with language hint:

` + "```go" + `
func main() {
    fmt.Println("Hello, world!")
}
` + "```" + `

Or indented:

    int x = 42;

## Blockquote

> Markdown can write *this* and **that**.
>
> Empty line preserved above.

## Links

Inline: [the project](https://example.com/fv-go)
Autolink: <https://github.com/oldwired/fv-go>
Email:    <hello@example.com>
Image:    ![Diagram](images/diagram.png)

## Tables

| Feature       | Status   | Notes                       |
| :----------   | :------: | --------------------------: |
| **Headings**  | *done*   | ATX + Setext                |
| ` + "`Inline`" + `      | *done*   | bold, italic, ~~strike~~    |
| Links         | *done*   | [docs](https://example.com) |
| ![pic](x.png) | <h@x.io> | autolink + email            |
| Tables        | *done*   | even cells with **markup**  |

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

// showImageViewOpen runs a file-open dialog and shows the chosen image
// in a window sized to match the image's native pixel dimensions
// (clamped to the desktop area, aspect ratio preserved). Cancel = no-op.
func showImageViewOpen(a *app.Application) {
	path, ok := stddlg.ShowModern(&a.Desktop.Group, stddlg.ModeOpen, "Open Image",
		"", "*")
	if !ok {
		return
	}
	img, err := imageview.LoadFile(path)
	if err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"Couldn't open %s:\n%s", []any{path, err.Error()}, msgbox.OKOnly)
		return
	}
	useSixel := sixel.IsSupported()
	cols, rows := imageview.PreferredCells(img, useSixel)
	// Reserve 2 cells for the window frame on each axis. Clamp the
	// content area to whatever the desktop allows, preserving aspect.
	deskExt := a.Desktop.GetExtent()
	cols, rows = imageview.FitCells(cols, rows, deskExt.Width()-2, deskExt.Height()-2)
	winW, winH := cols+2, rows+2
	x := deskExt.A.X + (deskExt.Width()-winW)/2
	y := deskExt.A.Y + (deskExt.Height()-winH)/2
	if x < deskExt.A.X {
		x = deskExt.A.X
	}
	if y < deskExt.A.Y {
		y = deskExt.A.Y
	}
	win := views.NewWindow(geom.NewRect(x, y, x+winW, y+winH), filepath.Base(path), 0)
	iv := imageview.New(geom.NewRect(1, 1, winW-1, winH-1))
	iv.SetImage(img)
	win.Insert(iv)
	a.Desktop.InsertWindow(win)
}

// showTerminal opens an embedded terminal running the user's $SHELL
// (falling back to /bin/sh). The window auto-closes when the shell
// exits, and the OSC 0/1/2 title — set by zsh / bash / vim / etc. —
// becomes the window caption.
func showTerminal(a *app.Application) {
	flags := int(consts.WfMove | consts.WfClose | consts.WfGrow | consts.WfZoom)
	win := views.NewWindow(geom.NewRect(2, 1, 82, 26), "Terminal", flags)
	w, h := win.Size.X, win.Size.Y
	t := terminal.New(geom.NewRect(1, 1, w-1, h-1))
	win.Insert(t)
	a.Desktop.InsertWindow(win)

	// Diagnostic banner — proves the parser+renderer are alive even
	// before any shell output arrives. Helpful when the shell itself
	// is silent (login configs not running, broken $SHELL, …).
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	_, _ = t.Write([]byte("\x1b[36m[fv-go terminal] launching " + shell + " …\x1b[0m\r\n"))

	// Title updates: feed back into the window's caption.
	t.OnTitle = func(title string) {
		if title != "" {
			win.SetTitle(title)
		}
	}
	// Auto-close on child exit. We can't call Delete from the wait
	// goroutine (the view tree isn't goroutine-safe), so post a custom
	// command back through the event queue and let dispatchExtension
	// handle it on the main loop.
	t.OnExit = func(error) {
		q := views.GetEventQueue()
		if q != nil {
			q.Put(drivers.Event{What: consts.EvCommand, Command: cmTerminalChildExited, InfoPtr: win})
		}
	}

	// No args — let the shell auto-detect interactivity from the TTY.
	// -i has been known to confuse zsh's startup on macOS when its
	// config files run a non-trivial setup; plain `zsh` works.
	if err := t.Start(shell, nil, nil); err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"Couldn't start %s:\n%s", []any{shell, err.Error()}, msgbox.OKOnly)
		a.Desktop.Delete(win.Self())
		return
	}
	if pid := t.PID(); pid != 0 {
		_, _ = t.Write([]byte("\x1b[2m[pid " + strconv.Itoa(pid) + "]\x1b[0m\r\n"))
	}
}

// showCanvas opens a SixelCanvasView with a bouncing-balls animation
// drawn into a pixel buffer. Demonstrates the SetPixel / FillRect /
// DrawLine primitives + the Tick-driven update path.
func showCanvas(a *app.Application) {
	win := views.NewWindow(geom.NewRect(2, 1, 60, 20), "Canvas (bouncing balls)", 0)
	w, h := win.Size.X, win.Size.Y
	c := sixelcanvas.New(geom.NewRect(1, 1, w-1, h-1), 320, 200)
	c.BG = 0x101030 // matches the Clear() color so undercovered cells blend in
	balls := []*ball{
		{x: 40, y: 30, dx: 1.7, dy: 1.1, color: 0xFF3030, radius: 6},
		{x: 120, y: 80, dx: -1.3, dy: 1.4, color: 0x30C0FF, radius: 8},
		{x: 200, y: 120, dx: 1.0, dy: -1.6, color: 0xFFE030, radius: 5},
		{x: 250, y: 60, dx: -0.9, dy: -1.2, color: 0x60FF60, radius: 7},
	}
	c.OnTick = func() {
		c.Clear(0x101030)
		for _, b := range balls {
			b.step(float64(c.PixelW), float64(c.PixelH))
			c.FillRect(int(b.x)-b.radius, int(b.y)-b.radius, b.radius*2, b.radius*2, b.color)
		}
	}
	win.Insert(c)
	a.Desktop.InsertWindow(win)
}

type ball struct {
	x, y   float64
	dx, dy float64
	color  uint32
	radius int
}

func (b *ball) step(w, h float64) {
	b.x += b.dx
	b.y += b.dy
	if b.x < float64(b.radius) {
		b.x = float64(b.radius)
		b.dx = -b.dx
	}
	if b.x > w-float64(b.radius) {
		b.x = w - float64(b.radius)
		b.dx = -b.dx
	}
	if b.y < float64(b.radius) {
		b.y = float64(b.radius)
		b.dy = -b.dy
	}
	if b.y > h-float64(b.radius) {
		b.y = h - float64(b.radius)
		b.dy = -b.dy
	}
}

// showImageView opens a window with a synthesized 480×320 test image —
// a hue-cycling Mandelbrot-ish fractal under a vertical-stripe color
// strip. Generated in code so the demo doesn't need any asset files.
// The view falls back to half-block automatically on terminals that
// don't support SIXEL.
func showImageView(a *app.Application) {
	win := views.NewWindow(geom.NewRect(2, 1, 60, 22), "Image View", 0)
	w, h := win.Size.X, win.Size.Y
	iv := imageview.New(geom.NewRect(1, 1, w-1, h-1))
	iv.SetImage(makeTestImage(480, 320))
	win.Insert(iv)
	a.Desktop.InsertWindow(win)
}

// makeTestImage builds a colorful image: 6 vertical color bars on top,
// a Julia/Mandelbrot-style fractal painted in HSV underneath. Pure CPU,
// runs in a few ms at 480×320.
func makeTestImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	stripeH := h / 8
	bars := []color.RGBA{
		{255, 0, 0, 255}, {255, 128, 0, 255}, {255, 255, 0, 255},
		{0, 255, 0, 255}, {0, 128, 255, 255}, {128, 0, 255, 255},
	}
	for y := 0; y < stripeH; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, bars[(x*len(bars))/w])
		}
	}
	for y := stripeH; y < h; y++ {
		fy := (float64(y-stripeH)/float64(h-stripeH))*2.0 - 1.0
		for x := 0; x < w; x++ {
			fx := (float64(x)/float64(w))*3.0 - 2.0
			zr, zi := 0.0, 0.0
			n := 0
			const maxIter = 64
			for ; n < maxIter; n++ {
				zr2, zi2 := zr*zr-zi*zi+fx, 2*zr*zi+fy
				zr, zi = zr2, zi2
				if zr*zr+zi*zi > 4 {
					break
				}
			}
			if n == maxIter {
				img.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
				continue
			}
			hue := float64(n) / maxIter
			img.SetRGBA(x, y, hsvToRGB(hue, 0.85, 1.0))
		}
	}
	return img
}

// hsvToRGB converts h, s, v in [0, 1] to a 24-bit RGBA color.
func hsvToRGB(h, s, v float64) color.RGBA {
	if s == 0 {
		c := uint8(v * 255)
		return color.RGBA{c, c, c, 255}
	}
	h6 := h * 6
	sector := int(math.Floor(h6))
	f := h6 - float64(sector)
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	var r, g, b float64
	switch sector % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	case 5:
		r, g, b = v, p, q
	}
	return color.RGBA{uint8(r * 255), uint8(g * 255), uint8(b * 255), 255}
}

// showProfileDump opens a small window listing what the term profile
// detected at startup. Useful for verifying that color / mouse /
// SIXEL / hyperlink probes ran correctly on the host terminal.
func showProfileDump(a *app.Application) {
	p := profile.Get()
	body := fmt.Sprintf(
		`Color system:       %v
ANSI supported:     %v
Interactive:        %v
Legacy console:     %v
Unicode:            %v
Hyperlink (OSC 8):  %v
SIXEL graphics:     %v
CI environment:     %v
TERM:               %s
TERM_PROGRAM:       %s
COLORTERM:          %s
`,
		p.ColorSystem, p.AnsiSupported, p.Interactive, p.LegacyConsole,
		p.Unicode, p.HyperlinkSupport, p.SixelSupport, p.IsCI,
		os.Getenv("TERM"), os.Getenv("TERM_PROGRAM"), os.Getenv("COLORTERM"))

	win := views.NewWindow(geom.NewRect(4, 2, 60, 18), "Profile",
		int(consts.WfMove|consts.WfClose))
	w, h := win.Size.X, win.Size.Y
	txt := dialogs.NewStaticText(geom.NewRect(2, 1, w-2, h-1), body)
	win.Insert(txt)
	a.Desktop.InsertWindow(win)
}

// showHyperlinkDemo opens a window with a few hyperlink.New views.
// On a terminal that honors OSC 8 (iTerm2, WezTerm, kitty, recent
// Windows Terminal, …) the URLs are clickable; everywhere else they
// stay underlined.
func showHyperlinkDemo(a *app.Application) {
	win := views.NewWindow(geom.NewRect(4, 2, 64, 14), "Hyperlinks",
		int(consts.WfMove|consts.WfClose))
	w, h := win.Size.X, win.Size.Y
	_ = h
	win.Insert(dialogs.NewStaticText(geom.NewRect(2, 1, w-2, 2),
		"OSC 8 hyperlinks — clickable in modern terminals:"))
	win.Insert(hyperlink.New(geom.NewRect(2, 3, w-2, 4),
		"fv-go on GitHub", "https://github.com/oldwired/fv-go"))
	win.Insert(hyperlink.New(geom.NewRect(2, 5, w-2, 6),
		"Free Vision (Delphi)", "https://github.com/oldwired/fv-delphi-modern"))
	win.Insert(hyperlink.New(geom.NewRect(2, 7, w-2, 8),
		"Turbo Vision background", "https://en.wikipedia.org/wiki/Turbo_Vision"))
	win.Insert(dialogs.NewStaticText(geom.NewRect(2, 9, w-2, 10),
		"Cmd / Ctrl + click in iTerm2 / WezTerm / kitty to follow."))
	a.Desktop.InsertWindow(win)
}

// showUnicodeDemo demonstrates the unicode width tables: a mix of
// BMP text, CJK ideographs, emoji ZWJ sequences, and combining marks.
// Use it to verify rendering when changing fonts.
func showUnicodeDemo(a *app.Application) {
	win := views.NewWindow(geom.NewRect(4, 2, 64, 16), "Unicode",
		int(consts.WfMove|consts.WfClose))
	w, h := win.Size.X, win.Size.Y
	_ = h
	lines := []string{
		"ASCII:    The quick brown fox jumps over the lazy dog.",
		"Latin-1:  café crème brûlée naïve résumé Zürich",
		"Greek:    Ζεύς απαθηνής λοιπóν με τους θεούς",
		"Hebrew:   שלום עולם — right-to-left",
		"CJK:      你好世界 — 日本語 — 한국어",
		"Emoji:    🍎🍊🍋  👨‍👩‍👧‍👦  🏳️‍🌈  ✨🚀💫",
		"Combine:  a + ̈ = ä  e + ́ = é  o + ̃ = õ",
	}
	for i, line := range lines {
		win.Insert(dialogs.NewStaticText(
			geom.NewRect(2, 1+i, w-2, 2+i), line))
	}
	a.Desktop.InsertWindow(win)
}

// saveDesktopFile pops a save-file dialog and writes the desktop layout
// to the chosen path via the new app.SaveDesktop helper.
func saveDesktopFile(a *app.Application) {
	path, ok := stddlg.ShowModern(&a.Desktop.Group, stddlg.ModeSave,
		"Save Desktop", "", "*.fvd")
	if !ok {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"Couldn't write %s:\n%s", []any{path, err.Error()}, msgbox.OKOnly)
		return
	}
	defer func() { _ = f.Close() }()
	if err := a.SaveDesktop(f); err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"Save failed: %s", []any{err.Error()}, msgbox.OKOnly)
	}
}

// loadDesktopFile is the symmetric load path.
func loadDesktopFile(a *app.Application) {
	path, ok := stddlg.ShowModern(&a.Desktop.Group, stddlg.ModeOpen,
		"Load Desktop", "", "*.fvd")
	if !ok {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"Couldn't open %s:\n%s", []any{path, err.Error()}, msgbox.OKOnly)
		return
	}
	defer func() { _ = f.Close() }()
	if err := a.LoadDesktop(f); err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"Load failed: %s", []any{err.Error()}, msgbox.OKOnly)
	}
}
