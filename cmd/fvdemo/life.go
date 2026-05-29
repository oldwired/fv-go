package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/sixel"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/sixelcanvas"
)

// --- Engine ---------------------------------------------------------

// boundaryMode selects how the universe behaves at its edges.
type boundaryMode int

const (
	bmWrap      boundaryMode = iota // toroidal: edges connect
	bmDead                          // off-grid cells are permanently empty
	bmDeflect                       // reflective wall: edge cell stands in for its missing neighbor
	bmEliminate                     // any connected pattern that touches the border is removed whole
)

func (m boundaryMode) String() string {
	switch m {
	case bmWrap:
		return "Wrap"
	case bmDead:
		return "Dead"
	case bmDeflect:
		return "Deflect"
	case bmEliminate:
		return "Eliminate"
	}
	return "?"
}

// board is the finite Game-of-Life universe. It is pure logic with no
// view dependency so it can be unit-tested directly.
type board struct {
	w, h        int
	cells, next []bool
	mode        boundaryMode
}

func newBoard(w, h int, mode boundaryMode) *board {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &board{
		w:     w,
		h:     h,
		cells: make([]bool, w*h),
		next:  make([]bool, w*h),
		mode:  mode,
	}
}

func (b *board) at(x, y int) bool {
	if x < 0 || x >= b.w || y < 0 || y >= b.h {
		return false
	}
	return b.cells[y*b.w+x]
}

func (b *board) set(x, y int, v bool) {
	if x < 0 || x >= b.w || y < 0 || y >= b.h {
		return
	}
	b.cells[y*b.w+x] = v
}

func (b *board) toggle(x, y int) {
	if x < 0 || x >= b.w || y < 0 || y >= b.h {
		return
	}
	b.cells[y*b.w+x] = !b.cells[y*b.w+x]
}

func (b *board) clear() {
	for i := range b.cells {
		b.cells[i] = false
	}
}

func (b *board) randomize(rng *rand.Rand, density float64) {
	for i := range b.cells {
		b.cells[i] = rng.Float64() < density
	}
}

// resize reallocates the grid, preserving the overlapping top-left
// region of the existing population.
func (b *board) resize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if w == b.w && h == b.h {
		return
	}
	cells := make([]bool, w*h)
	cw, ch := w, h
	if b.w < cw {
		cw = b.w
	}
	if b.h < ch {
		ch = b.h
	}
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			cells[y*w+x] = b.cells[y*b.w+x]
		}
	}
	b.w, b.h = w, h
	b.cells = cells
	b.next = make([]bool, w*h)
}

// neighborAlive resolves a possibly-off-grid neighbor coordinate
// according to the active boundary mode.
func (b *board) neighborAlive(x, y int) bool {
	switch b.mode {
	case bmWrap:
		x = ((x % b.w) + b.w) % b.w
		y = ((y % b.h) + b.h) % b.h
		return b.cells[y*b.w+x]
	case bmDeflect:
		if x < 0 {
			x = 0
		} else if x >= b.w {
			x = b.w - 1
		}
		if y < 0 {
			y = 0
		} else if y >= b.h {
			y = b.h - 1
		}
		return b.cells[y*b.w+x]
	default: // bmDead, bmEliminate: off-grid is empty
		return b.at(x, y)
	}
}

// step advances one generation using the B3/S23 rule.
func (b *board) step() {
	for y := 0; y < b.h; y++ {
		for x := 0; x < b.w; x++ {
			n := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					if b.neighborAlive(x+dx, y+dy) {
						n++
					}
				}
			}
			alive := b.cells[y*b.w+x]
			b.next[y*b.w+x] = n == 3 || (alive && n == 2)
		}
	}
	if b.mode == bmEliminate {
		b.eliminateBorderClusters()
	}
	b.cells, b.next = b.next, b.cells
}

// eliminateBorderClusters clears, in b.next, every 8-connected live
// cluster that has at least one cell on the border.
func (b *board) eliminateBorderClusters() {
	visited := make([]bool, b.w*b.h)
	var stack []int
	flood := func(start int) {
		stack = append(stack[:0], start)
		visited[start] = true
		for len(stack) > 0 {
			idx := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			b.next[idx] = false
			cx, cy := idx%b.w, idx/b.w
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := cx+dx, cy+dy
					if nx < 0 || nx >= b.w || ny < 0 || ny >= b.h {
						continue
					}
					ni := ny*b.w + nx
					if !visited[ni] && b.next[ni] {
						visited[ni] = true
						stack = append(stack, ni)
					}
				}
			}
		}
	}
	for x := 0; x < b.w; x++ {
		if i := x; b.next[i] && !visited[i] {
			flood(i)
		}
		if i := (b.h-1)*b.w + x; b.next[i] && !visited[i] {
			flood(i)
		}
	}
	for y := 0; y < b.h; y++ {
		if i := y * b.w; b.next[i] && !visited[i] {
			flood(i)
		}
		if i := y*b.w + (b.w - 1); b.next[i] && !visited[i] {
			flood(i)
		}
	}
}

// --- Viewport / zoom helpers ----------------------------------------

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// textMag is the number of screen characters per board cell in text
// mode (floors at a 1:1 view; SIXEL covers finer-grained zoom-out).
func textMag(zoom int) int { return clampInt(zoom, 1, 4) }

// sixelPxPerCell is the number of pixels per board cell in SIXEL mode.
func sixelPxPerCell(zoom int) int { return clampInt(zoom*2, 2, 16) }

// --- Text renderer --------------------------------------------------

// lifeTextView paints the board as terminal cells. It is passive: it
// reads the owning window's board + viewport and does no input handling.
type lifeTextView struct {
	views.Base
	win *lifeWindow
}

func newLifeTextView(bounds geom.Rect, win *lifeWindow) *lifeTextView {
	v := &lifeTextView{Base: views.NewBase(bounds), win: win}
	v.SetSelf(v)
	v.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	return v
}

func (v *lifeTextView) GetTypeID() string { return "life.text" }

func (v *lifeTextView) Draw() {
	w := v.win
	mag := textMag(w.zoom)
	aliveAttr := types.MakeAttr(15, 0)
	deadAttr := types.MakeAttr(8, 0)
	for y := 0; y < v.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(v.Size.X)
		by := w.viewY + y/mag
		for x := 0; x < v.Size.X; x++ {
			bx := w.viewX + x/mag
			if w.b.at(bx, by) {
				screen.DrawCell(buf, x, "█", aliveAttr)
			} else {
				screen.DrawCell(buf, x, " ", deadAttr)
			}
		}
		v.WriteLine(0, y, v.Size.X, 1, buf)
	}
}

// --- Controller window ----------------------------------------------

const (
	lifeAliveRGB uint32 = 0x33FF66
	lifeBGRGB    uint32 = 0x101018
)

// lifeWindow owns the engine, the viewport, the animation tick, and all
// input. It mirrors the gridDemoWindow pattern (a custom Window subclass
// that overrides HandleEvent).
type lifeWindow struct {
	views.Window
	app    *app.Application
	b      *board
	text   *lifeTextView
	canvas *sixelcanvas.SixelCanvasView
	hint   *dialogs.StaticText

	sixelMode bool
	paused    bool

	viewX, viewY int
	zoom         int
	density      float64
	rng          *rand.Rand
}

func newLifeWindow(a *app.Application, bounds geom.Rect) *lifeWindow {
	w := &lifeWindow{
		app:     a,
		zoom:    1,
		density: 0.25,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	views.InitWindow(&w.Window, bounds, "", 0)
	w.SetSelf(w)
	w.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY

	w.b = newBoard(256, 160, bmWrap)
	w.b.randomize(w.rng, w.density)

	W, H := w.Size.X, w.Size.Y
	renderRect := geom.NewRect(1, 1, W-1, H-2)

	w.text = newLifeTextView(renderRect, w)
	w.Insert(w.text)

	cw, ch := sixel.CellSize()
	w.canvas = sixelcanvas.New(renderRect, w.text.Size.X*cw, w.text.Size.Y*ch)
	w.canvas.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	w.canvas.BG = lifeBGRGB
	w.canvas.SetState(consts.SfVisible, false)
	w.Insert(w.canvas)

	w.hint = dialogs.NewStaticText(geom.NewRect(1, H-2, W-1, H-1),
		" g:mode  space:pause  s:step  r:rand  c:clear  b:border  arrows:pan  +/-:zoom  o:options")
	w.hint.GrowMode = consts.GfGrowLoY | consts.GfGrowHiY | consts.GfGrowHiX
	w.Insert(w.hint)

	w.retitle()
	anim.Register(w, 120*time.Millisecond)
	return w
}

func (w *lifeWindow) GetTypeID() string { return "life.window" }

func (w *lifeWindow) retitle() {
	state := "running"
	if w.paused {
		state = "paused"
	}
	mode := "text"
	if w.sixelMode {
		mode = "sixel"
	}
	w.SetTitle(fmt.Sprintf("Game of Life — %s · %s · %s · zoom %d · %dx%d",
		mode, state, w.b.mode, w.zoom, w.b.w, w.b.h))
}

// visibleCells reports how many board cells fit across the render area
// in the active render mode at the current zoom.
func (w *lifeWindow) visibleCells() (cols, rows int) {
	if w.sixelMode {
		pxc := sixelPxPerCell(w.zoom)
		return w.canvas.PixelW / pxc, w.canvas.PixelH / pxc
	}
	mag := textMag(w.zoom)
	return w.text.Size.X / mag, w.text.Size.Y / mag
}

func (w *lifeWindow) clampViewport() {
	cols, rows := w.visibleCells()
	w.viewX = clampInt(w.viewX, 0, max0(w.b.w-cols))
	w.viewY = clampInt(w.viewY, 0, max0(w.b.h-rows))
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// paintCanvas renders the board into the SIXEL canvas pixel buffer.
func (w *lifeWindow) paintCanvas() {
	w.canvas.Clear(lifeBGRGB)
	pxc := sixelPxPerCell(w.zoom)
	cols := w.canvas.PixelW / pxc
	rows := w.canvas.PixelH / pxc
	for sy := 0; sy < rows; sy++ {
		by := w.viewY + sy
		for sx := 0; sx < cols; sx++ {
			if w.b.at(w.viewX+sx, by) {
				w.canvas.FillRect(sx*pxc, sy*pxc, pxc, pxc, lifeAliveRGB)
			}
		}
	}
}

// refresh repaints after a manual change (mouse, zoom, pan, settings),
// which matters while paused since Tick does no work then.
func (w *lifeWindow) refresh() {
	if w.sixelMode {
		w.paintCanvas()
	}
	views.MarkDirty()
}

// Tick advances the simulation while running.
func (w *lifeWindow) Tick(now time.Time) bool {
	if w.paused {
		return false
	}
	w.b.step()
	if w.sixelMode {
		w.paintCanvas()
	}
	return true
}

func (w *lifeWindow) toggleMode() {
	w.sixelMode = !w.sixelMode
	w.text.SetState(consts.SfVisible, !w.sixelMode)
	w.canvas.SetState(consts.SfVisible, w.sixelMode)
	w.clampViewport()
	w.retitle()
	w.refresh()
}

// boardCellAt maps a screen point to a board cell in the active mode.
func (w *lifeWindow) boardCellAt(p geom.Point) (int, int) {
	local := w.text.MakeLocal(p)
	if w.sixelMode {
		cw, ch := sixel.CellSize()
		pxc := sixelPxPerCell(w.zoom)
		return w.viewX + (local.X*cw)/pxc, w.viewY + (local.Y*ch)/pxc
	}
	mag := textMag(w.zoom)
	return w.viewX + local.X/mag, w.viewY + local.Y/mag
}

func (w *lifeWindow) HandleEvent(ev *drivers.Event) {
	switch ev.What {
	case consts.EvKeyDown:
		if w.handleKey(ev) {
			w.ClearEvent(ev)
			return
		}
	case consts.EvMouseDown:
		// Only paint when the click lands in the render area; clicks on
		// the frame (title-bar drag, close box, resize corner) must reach
		// Window.HandleEvent.
		if w.text.MouseInView(ev.Where) {
			bx, by := w.boardCellAt(ev.Where)
			w.b.toggle(bx, by)
			w.refresh()
			w.ClearEvent(ev)
			return
		}
	}
	w.Window.HandleEvent(ev)
}

// handleKey processes a key, returning true if it was consumed.
func (w *lifeWindow) handleKey(ev *drivers.Event) bool {
	switch ev.KeyCode {
	case consts.KbSpaceBar:
		w.paused = !w.paused
		w.retitle()
		w.refresh()
		return true
	case consts.KbLeft:
		w.viewX -= panStep(w.zoom)
		w.clampViewport()
		w.refresh()
		return true
	case consts.KbRight:
		w.viewX += panStep(w.zoom)
		w.clampViewport()
		w.refresh()
		return true
	case consts.KbUp:
		w.viewY -= panStep(w.zoom)
		w.clampViewport()
		w.refresh()
		return true
	case consts.KbDown:
		w.viewY += panStep(w.zoom)
		w.clampViewport()
		w.refresh()
		return true
	}
	switch ev.UnicodeChar {
	case 'g', 'G':
		w.toggleMode()
		return true
	case 's', 'S':
		w.paused = true
		w.b.step()
		w.retitle()
		w.refresh()
		return true
	case 'r', 'R':
		w.b.randomize(w.rng, w.density)
		w.refresh()
		return true
	case 'c', 'C':
		w.b.clear()
		w.refresh()
		return true
	case 'b', 'B':
		w.b.mode = (w.b.mode + 1) % 4
		w.retitle()
		w.refresh()
		return true
	case 'o', 'O':
		w.editSettings()
		return true
	case '+', '=':
		w.zoom = clampInt(w.zoom+1, 1, 8)
		w.clampViewport()
		w.retitle()
		w.refresh()
		return true
	case '-', '_':
		w.zoom = clampInt(w.zoom-1, 1, 8)
		w.clampViewport()
		w.retitle()
		w.refresh()
		return true
	}
	return false
}

func panStep(zoom int) int {
	s := 8 / textMag(zoom)
	if s < 1 {
		return 1
	}
	return s
}

func (w *lifeWindow) ChangeBounds(r geom.Rect) {
	w.Window.ChangeBounds(r)
	cw, ch := sixel.CellSize()
	w.canvas.Resize(w.text.Size.X*cw, w.text.Size.Y*ch)
	w.clampViewport()
	w.refresh()
}

// --- Settings dialog ------------------------------------------------

func (w *lifeWindow) editSettings() {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 46, 16), "Game of Life — Settings")

	width := dialogs.NewInputLong(geom.NewRect(20, 2, 30, 3), 8, 4096, 4)
	width.SetInt(int64(w.b.w))
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 2, 19, 3), "~W~idth (8-4096):", &width.InputLine))
	d.Insert(width)

	height := dialogs.NewInputLong(geom.NewRect(20, 4, 30, 5), 8, 4096, 4)
	height.SetInt(int64(w.b.h))
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 4, 19, 5), "~H~eight (8-4096):", &height.InputLine))
	d.Insert(height)

	dens := dialogs.NewInputLong(geom.NewRect(20, 6, 30, 7), 1, 99, 2)
	dens.SetInt(int64(w.density*100 + 0.5))
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 6, 19, 7), "~D~ensity %:", &dens.InputLine))
	d.Insert(dens)

	d.Insert(dialogs.NewLabel(geom.NewRect(2, 8, 44, 9), "~B~oundary:", nil))
	boundary := dialogs.NewRadioButtons(geom.NewRect(2, 9, 44, 13),
		[]string{"Wrap (toroidal)", "Dead (empty edges)", "Deflect (walls)", "Eliminate (kill at edge)"})
	boundary.PressOne(int(w.b.mode))
	d.Insert(boundary)

	d.Insert(dialogs.NewButton(geom.NewRect(8, 13, 18, 15), "O~K~", consts.CmOK, dialogs.BfDefault))
	d.Insert(dialogs.NewButton(geom.NewRect(24, 13, 36, 15), "Cancel", consts.CmCancel, 0))

	if w.app.Desktop.ExecView(d) != consts.CmOK {
		return
	}

	w.density = float64(dens.Int()) / 100
	for i := 0; i < 4; i++ {
		if boundary.Mark(i) {
			w.b.mode = boundaryMode(i)
			break
		}
	}
	w.b.resize(int(width.Int()), int(height.Int()))
	w.clampViewport()
	w.retitle()
	w.refresh()
}

func showGameOfLife(a *app.Application) {
	win := newLifeWindow(a, geom.NewRect(2, 2, 74, 32))
	a.Desktop.InsertWindow(win)
}
