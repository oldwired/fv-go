package main

import (
	"math"

	"github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// desktopWindows returns the *Window children of the desktop, skipping
// the wallpaper Background. The slice order mirrors the desktop's
// children order, so the last element is the topmost window.
func desktopWindows(a *app.Application) []*views.Window {
	var ws []*views.Window
	for _, c := range a.Desktop.Children {
		if c == a.Desktop.Background {
			continue
		}
		if w, ok := c.(*views.Window); ok {
			ws = append(ws, w)
		}
	}
	return ws
}

// focusedWindow returns whichever desktop window has SfFocused, or
// the topmost window as a fallback, or nil if none.
func focusedWindow(a *app.Application) *views.Window {
	ws := desktopWindows(a)
	if len(ws) == 0 {
		return nil
	}
	for _, w := range ws {
		if w.GetState(consts.SfFocused) {
			return w
		}
	}
	return ws[len(ws)-1]
}

func zoomFocused(a *app.Application) {
	if w := focusedWindow(a); w != nil {
		ev := drivers.Event{What: consts.EvCommand, Command: consts.CmZoom}
		w.HandleEvent(&ev)
	}
}

// cascadeWindows resizes every window to a uniform 75%-of-desktop size
// and offsets them diagonally by 2 cells per step. Mirrors the classic
// TV "Cascade" command.
func cascadeWindows(a *app.Application) {
	ws := desktopWindows(a)
	n := len(ws)
	if n == 0 {
		return
	}
	cols, rows := a.Desktop.Size.X, a.Desktop.Size.Y
	winW, winH := cols*3/4, rows*3/4
	if winW < 30 {
		winW = 30
	}
	if winH < 8 {
		winH = 8
	}
	for i, w := range ws {
		off := i * 2
		x := safeOffset(off, cols-winW)
		y := safeOffset(off, rows-winH)
		w.ChangeBounds(geom.NewRect(x, y, x+winW, y+winH))
	}
}

// cascadeNoResize keeps each window's current size but offsets the
// origins diagonally so they aren't perfectly stacked. Useful when
// you've sized windows individually and just want them un-overlapped.
func cascadeNoResize(a *app.Application) {
	ws := desktopWindows(a)
	if len(ws) == 0 {
		return
	}
	desktopW, desktopH := a.Desktop.Size.X, a.Desktop.Size.Y
	for i, w := range ws {
		off := i * 2
		x := safeOffset(off, desktopW-w.Size.X)
		y := safeOffset(off, desktopH-w.Size.Y)
		w.ChangeBounds(geom.NewRect(x, y, x+w.Size.X, y+w.Size.Y))
	}
}

// safeOffset returns off wrapped into [0, max] with max clamped to >= 0.
// Avoids divide-by-zero / negative-modulo when the desktop is barely
// larger than the window.
func safeOffset(off, max int) int {
	if max <= 0 {
		return 0
	}
	x := off % max
	if x < 0 {
		x = 0
	}
	return x
}

// tileWindows arranges all desktop windows in a near-square grid that
// fills the desktop. Number of columns is round(sqrt(n)); the last
// column / row absorbs any remainder so the layout exactly fills the
// available area.
func tileWindows(a *app.Application) {
	ws := desktopWindows(a)
	n := len(ws)
	if n == 0 {
		return
	}
	cols := int(math.Ceil(math.Sqrt(float64(n))))
	if cols < 1 {
		cols = 1
	}
	rows := (n + cols - 1) / cols
	tileGrid(a, ws, cols, rows)
}

// tileHorizontal arranges windows as N horizontal strips, each spanning
// the full desktop width. So with 3 windows you get three rows.
func tileHorizontal(a *app.Application) {
	ws := desktopWindows(a)
	if len(ws) == 0 {
		return
	}
	tileGrid(a, ws, 1, len(ws))
}

// tileVertical arranges windows as N vertical strips, each spanning the
// full desktop height. So with 3 windows you get three columns.
func tileVertical(a *app.Application) {
	ws := desktopWindows(a)
	if len(ws) == 0 {
		return
	}
	tileGrid(a, ws, len(ws), 1)
}

// tileGrid places ws into a cols x rows grid, filling row by row.
// The last column and last row absorb any remainder so the grid is
// exactly the desktop's size.
func tileGrid(a *app.Application, ws []*views.Window, cols, rows int) {
	desktopW, desktopH := a.Desktop.Size.X, a.Desktop.Size.Y
	cellW := desktopW / cols
	cellH := desktopH / rows
	for i, w := range ws {
		col := i % cols
		row := i / cols
		x0 := col * cellW
		y0 := row * cellH
		x1 := x0 + cellW
		y1 := y0 + cellH
		if col == cols-1 {
			x1 = desktopW
		}
		if row == rows-1 {
			y1 = desktopH
		}
		w.ChangeBounds(geom.NewRect(x0, y0, x1, y1))
	}
}

// cycleWindow walks the desktop's window stack by step (+1 = next,
// -1 = previous) and raises the chosen window.
func cycleWindow(a *app.Application, step int) {
	ws := desktopWindows(a)
	n := len(ws)
	if n == 0 {
		return
	}
	cur := -1
	for i, w := range ws {
		if w.GetState(consts.SfFocused) {
			cur = i
			break
		}
	}
	if cur < 0 {
		cur = n - 1
	}
	next := ((cur+step)%n + n) % n
	a.Desktop.MakeFirst(ws[next])
	a.Desktop.Focus(ws[next])
}

func closeFocused(a *app.Application) {
	if w := focusedWindow(a); w != nil {
		w.Close()
	}
}

func closeAll(a *app.Application) {
	for _, w := range desktopWindows(a) {
		w.Close()
	}
}

// selectWindowNum brings the Nth window forward, by Window.Number().
// Used by the Alt+0..9 shortcut.
func selectWindowNum(a *app.Application, n int) {
	if n == 0 {
		return
	}
	for _, w := range desktopWindows(a) {
		if w.Number() == n {
			a.Desktop.MakeFirst(w)
			a.Desktop.Focus(w)
			return
		}
	}
}
