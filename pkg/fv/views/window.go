package views

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
)

// Frame draws a window's border, title, close icon, and zoom icon.
type Frame struct {
	Base
}

// NewFrame returns a Frame sized to wrap an enclosing window.
func NewFrame(bounds geom.Rect) *Frame {
	f := &Frame{Base: NewBase(bounds)}
	f.SetSelf(f)
	f.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	f.Options = 0
	f.EventMask = consts.EvBroadcast
	return f
}

// GetTypeID for serial registry.
func (f *Frame) GetTypeID() string { return "frame" }

// frameChars: pos 0 = passive, pos 1 = active.
// Each set: tl, tr, bl, br, h, v.
var frameChars = [2][6]rune{
	{'┌', '┐', '└', '┘', '─', '│'}, // passive — single line
	{'╔', '╗', '╚', '╝', '═', '║'}, // active — double line
}

// Draw paints only the border cells. The interior is left to whatever
// drew it — typically Window.Draw, which fills first then asks the
// frame (its first child) to overlay the border.
func (f *Frame) Draw() {
	active := byte(0)
	// Walk up starting at the Frame's direct owner — that's the Window's
	// Group, whose `self` IS the Window. Earlier this skipped the
	// Window and started at the Desktop, which is why the title and
	// close/zoom icons didn't appear.
	for o := f.Owner; o != nil; o = o.Owner {
		if o.GetState(consts.SfActive) {
			active = 1
			break
		}
	}
	chars := frameChars[active]
	w := f.Size.X
	h := f.Size.Y
	if w < 2 || h < 2 {
		return
	}

	// Classic TV dialog frame palette: light-gray-on-cyan passive,
	// bright-white-on-cyan active.
	color := types.MakeAttr(0x07, 0x03)
	if active == 1 {
		color = types.MakeAttr(0x0F, 0x03)
	}
	iconColor := types.MakeAttr(0x0E, 0x03)

	// --- Top row ---
	top := screen.MakeDrawBuffer(w)
	screen.DrawCell(top, 0, string(chars[0]), color)
	for i := 1; i < w-1; i++ {
		screen.DrawCell(top, i, string(chars[4]), color)
	}
	screen.DrawCell(top, w-1, string(chars[1]), color)

	if title := f.windowTitle(); title != "" {
		ctitle := " " + title + " "
		startX := (w - len(ctitle)) / 2
		if startX < 4 {
			startX = 4
		}
		screen.DrawStr(top, startX, ctitle, color)
	}
	if f.windowFlags()&consts.WfClose != 0 {
		screen.DrawStr(top, 2, "[■]", iconColor)
	}
	if f.windowFlags()&consts.WfZoom != 0 {
		screen.DrawStr(top, w-5, "[↕]", iconColor)
	}
	f.WriteLine(0, 0, w, 1, top)

	// --- Side rows: write left and right cells only, leave interior alone. ---
	leftCell := screen.DrawBuffer{{Ch: string(chars[5]), Attr: color}}
	rightCell := screen.DrawBuffer{{Ch: string(chars[5]), Attr: color}}
	for y := 1; y < h-1; y++ {
		f.WriteLine(0, y, 1, 1, leftCell)
		f.WriteLine(w-1, y, 1, 1, rightCell)
	}

	// --- Bottom row ---
	bot := screen.MakeDrawBuffer(w)
	screen.DrawCell(bot, 0, string(chars[2]), color)
	for i := 1; i < w-1; i++ {
		screen.DrawCell(bot, i, string(chars[4]), color)
	}
	screen.DrawCell(bot, w-1, string(chars[3]), color)
	if f.windowFlags()&consts.WfGrow != 0 {
		screen.DrawCell(bot, w-1, "◢", iconColor)
	}
	f.WriteLine(0, h-1, w, 1, bot)
}

func (f *Frame) windowTitle() string {
	type titler interface{ Title() string }
	for o := f.Owner; o != nil; o = o.Owner {
		if t, ok := any(o.self).(titler); ok {
			return t.Title()
		}
	}
	return ""
}

func (f *Frame) windowFlags() byte {
	type flagger interface{ Flags() byte }
	for o := f.Owner; o != nil; o = o.Owner {
		if t, ok := any(o.self).(flagger); ok {
			return t.Flags()
		}
	}
	return 0
}

// Window is a framed, possibly movable, possibly resizable Group.
type Window struct {
	Group

	title    string
	flags    byte
	number   int
	zoomRect geom.Rect

	Frame *Frame
}

// NewWindow constructs a Window with the given bounds, title, and number.
// Number 0 means no number annotation in the frame title.
func NewWindow(bounds geom.Rect, title string, number int) *Window {
	w := &Window{}
	InitWindow(w, bounds, title, number)
	return w
}

// InitWindow initializes w in place. Used by NewDialog and other
// constructors that embed a Window by value — those must NOT struct-copy
// the result of NewWindow, because the copy would orphan the inserted
// Frame's Owner pointer.
func InitWindow(w *Window, bounds geom.Rect, title string, number int) {
	w.title = title
	w.number = number
	w.flags = consts.WfMove | consts.WfGrow | consts.WfClose | consts.WfZoom
	w.zoomRect = bounds
	InitGroup(&w.Group, bounds)
	w.SetSelf(w)
	w.State |= consts.SfShadow
	w.Options |= consts.OfSelectable | consts.OfTopSelect
	w.GrowMode = consts.GfGrowAll

	w.Frame = NewFrame(geom.Rect{B: w.Size})
	w.Insert(w.Frame)
}

// GetTypeID for serial registry.
func (w *Window) GetTypeID() string { return "window" }

// Draw paints the window: interior fill in the dialog palette, then
// every child (the Frame is first, so the border overlays the fill;
// content widgets follow), then the shadow if SfShadow is set.
func (w *Window) Draw() {
	// Interior fill: light-gray-on-cyan, the classic TV dialog body.
	bg := types.MakeAttr(0x07, 0x03)
	for y := 0; y < w.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(w.Size.X)
		for x := 0; x < w.Size.X; x++ {
			screen.DrawCell(buf, x, " ", bg)
		}
		w.WriteLine(0, y, w.Size.X, 1, buf)
	}
	w.Group.Draw()
	if w.GetState(consts.SfShadow) {
		w.drawShadow()
	}
}

// drawShadow paints a "cast shadow" on the right and bottom of the
// window: it reads whatever the cells underneath the shadow currently
// hold (typically the desktop wallpaper or another window), then
// rewrites them with a darkened color while preserving the glyph.
//
// This works because Group.Draw runs children in z-order, so by the
// time this Window's drawShadow runs, the cells around its border
// already hold whatever was drawn behind it for this frame.
func (w *Window) drawShadow() {
	if rootBackend == nil {
		return
	}
	sx, sy := w.ScreenOrigin()
	// Right edge: 2 columns wide, rows 1..h.
	for y := 1; y <= w.Size.Y; y++ {
		for dx := 0; dx < 2; dx++ {
			cellX := sx + w.Size.X + dx
			cellY := sy + y
			under := castShadow(rootBackend.GetCell(cellX, cellY))
			rootBackend.SetCell(cellX, cellY, under)
		}
	}
	// Bottom edge: cols 2..w+1 (offset by 2 so the corner doesn't
	// stick out past the left edge of the window).
	for dx := 2; dx < w.Size.X+2; dx++ {
		cellX := sx + dx
		cellY := sy + w.Size.Y
		under := castShadow(rootBackend.GetCell(cellX, cellY))
		rootBackend.SetCell(cellX, cellY, under)
	}
}

// castShadow returns c with its colors reduced to a dim-gray-on-black
// rendering, keeping the existing glyph. The result looks like the
// underlying content fading into darkness — closer to TV's classic
// drop-shadow effect than a solid block.
func castShadow(c types.DrawCell) types.DrawCell {
	if c.Ch == "" {
		c.Ch = " "
	}
	return types.DrawCell{
		Ch:   c.Ch,
		Attr: types.MakeAttr(0x08, 0x00),
	}
}

// Title returns the window's title (used by Frame).
func (w *Window) Title() string { return w.title }

// SetTitle changes the window title and forces a redraw.
func (w *Window) SetTitle(s string) {
	w.title = s
	if w.Frame != nil {
		w.Frame.Draw()
	}
}

// Flags returns the window flags (Wf*).
func (w *Window) Flags() byte { return w.flags }

// Number returns the window number (0 means none).
func (w *Window) Number() int { return w.number }

// HandleEvent extends Group with title-bar drag, close-box and zoom-box
// click handling, click-to-raise for non-modal windows, and the window
// commands cmClose / cmZoom.
func (w *Window) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		local := w.MakeLocal(ev.Where)
		// Close box: cells 2..4 on row 0. Acts directly on this window
		// instead of broadcasting cmClose, so only this window closes
		// (a broadcast would make every visible window close itself).
		if local.Y == 0 && w.flags&consts.WfClose != 0 &&
			local.X >= 2 && local.X <= 4 {
			w.close()
			w.ClearEvent(ev)
			return
		}
		// Zoom box: cells (w-5)..(w-3) on row 0.
		if local.Y == 0 && w.flags&consts.WfZoom != 0 &&
			local.X >= w.Size.X-5 && local.X <= w.Size.X-3 {
			w.zoom()
			w.ClearEvent(ev)
			return
		}
		// Click anywhere else on the window raises it to the top of
		// the parent's z-order. Only meaningful for non-modal windows
		// — modal windows are already on top by definition.
		if !w.GetState(consts.SfModal) && w.Owner != nil {
			w.Owner.MakeFirst(w.self)
			w.Owner.Focus(w.self)
		}
		// Title-bar drag (anywhere on row 0 that isn't a hot zone).
		if local.Y == 0 && w.flags&consts.WfMove != 0 {
			w.dragLoop(ev)
			w.ClearEvent(ev)
			return
		}
	}
	w.Group.HandleEvent(ev)
	if ev.What == consts.EvCommand {
		switch ev.Command {
		case consts.CmClose:
			if w.flags&consts.WfClose != 0 {
				w.close()
				w.ClearEvent(ev)
			}
		case consts.CmZoom:
			w.zoom()
			w.ClearEvent(ev)
		}
	}
}

// close ends the modal loop with cmCancel for modal windows; for
// non-modal ones it removes the window from its parent group entirely.
func (w *Window) close() {
	if w.GetState(consts.SfModal) {
		w.EndModal(consts.CmCancel)
		return
	}
	if w.Owner != nil {
		w.Owner.Delete(w.self)
	}
}

// dragLoop reads further mouse events until the button releases,
// translating the window by the cursor delta. We pull events directly
// from the global queue so the move is fluid; a redraw happens between
// each event via the pump callback.
func (w *Window) dragLoop(start *drivers.Event) {
	q := globalQueue
	if q == nil {
		return
	}
	prev := start.Where
	w.State |= consts.SfDragging
	defer func() { w.State &^= consts.SfDragging }()
	for {
		if pumpFn != nil {
			pumpFn()
		}
		ev, ok := q.Get()
		if !ok {
			if waitFn != nil {
				waitFn()
			}
			continue
		}
		switch {
		case ev.What == consts.EvMouseUp:
			return
		case ev.What == consts.EvMouseMove || ev.What == consts.EvMouseDown:
			dx := ev.Where.X - prev.X
			dy := ev.Where.Y - prev.Y
			if dx != 0 || dy != 0 {
				w.MoveTo(w.Origin.X+dx, w.Origin.Y+dy)
				prev = ev.Where
			}
		}
	}
}

func (w *Window) zoom() {
	cur := w.GetBounds()
	if cur.Equals(w.zoomRect) && w.Owner != nil {
		// Maximize against owner's extent.
		w.SetBounds(w.Owner.GetExtent())
	} else if w.Owner != nil {
		w.SetBounds(w.zoomRect)
	}
}

// Background fills its area with a single character. Used by Desktop.
type Background struct {
	Base
	Char rune
}

// NewBackground builds a Background covering bounds, drawn with c.
func NewBackground(bounds geom.Rect, c rune) *Background {
	b := &Background{Base: NewBase(bounds), Char: c}
	b.SetSelf(b)
	b.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	return b
}

// GetTypeID for serial registry.
func (b *Background) GetTypeID() string { return "background" }

// Draw fills the area with Char.
func (b *Background) Draw() {
	color := types.MakeAttr(0x07, 0x01) // legacy desktop blue
	for y := 0; y < b.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(b.Size.X)
		for x := 0; x < b.Size.X; x++ {
			screen.DrawCell(buf, x, string(b.Char), color)
		}
		b.WriteLine(0, y, b.Size.X, 1, buf)
	}
}
