package views

import (
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
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
	// Active = "this window is the focused one of its parent group".
	// We deliberately check ONLY the immediate owner: a recursive walk
	// would mark every window active whenever the Desktop is active.
	active := byte(0)
	if f.Owner != nil && f.Owner.GetState(consts.SfActive) {
		active = 1
	}
	chars := frameChars[active]
	w := f.Size.X
	h := f.Size.Y
	if w < 2 || h < 2 {
		return
	}

	// Classic TV dialog frame palette: light-gray-on-cyan passive,
	// bright-white-on-cyan active. Sourced from the theme so a host
	// can swap palettes at runtime.
	pal := theme.Get()
	color := pal.FrameNormal
	if active == 1 {
		color = pal.FrameActive
	}
	iconColor := pal.FrameIcons
	// Title-row attribute: while the owning window's flash window
	// is open, invert fg/bg so the bar visibly flips for the bell
	// flash. Only the top row gets the flip — side / bottom keep
	// their normal styling so the chrome isn't disorienting.
	titleColor := color
	titleIconColor := iconColor
	if f.windowFlashing() {
		titleColor = reverseFrameAttr(titleColor)
		titleIconColor = reverseFrameAttr(titleIconColor)
	}

	// --- Top row ---
	top := screen.MakeDrawBuffer(w)
	screen.DrawCell(top, 0, string(chars[0]), titleColor)
	for i := 1; i < w-1; i++ {
		screen.DrawCell(top, i, string(chars[4]), titleColor)
	}
	screen.DrawCell(top, w-1, string(chars[1]), titleColor)

	if title := f.windowTitle(); title != "" {
		ctitle := " " + title + " "
		startX := (w - len(ctitle)) / 2
		if startX < 4 {
			startX = 4
		}
		screen.DrawStr(top, startX, ctitle, titleColor)
	}
	if f.windowFlags()&consts.WfClose != 0 {
		screen.DrawStr(top, 2, "[■]", titleIconColor)
	}
	if f.windowFlags()&consts.WfZoom != 0 {
		screen.DrawStr(top, w-5, "[↕]", titleIconColor)
	}
	// Window number badge. Single-digit windows render their number;
	// double-digit (10+) renders "+" so the user still gets a visual
	// cue that the window has a number — the Alt-1..9 chord doesn't
	// reach windows past 9, those need the window-list dialog.
	if n := f.windowNumber(); n >= 1 && w > 7 {
		var badge string
		switch {
		case n <= 9:
			badge = " " + string(rune('0'+n)) + " "
		default:
			badge = " + "
		}
		screen.DrawStr(top, w-7, badge, titleIconColor)
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

// reverseFrameAttr flips the FG / BG bytes of a TV-packed attribute.
// Used for the title-bar flash overlay — same bit-shuffle pattern the
// selection highlight uses, but exposed here as a helper so Frame can
// call it without depending on the terminal widget.
func reverseFrameAttr(attr uint16) uint16 {
	fg := attr & 0x000F
	bg := (attr >> 8) & 0x000F
	fgRest := attr & 0x00F0
	bgRest := (attr >> 8) & 0x00F0
	return bg | bgRest | (fg << 8) | (fgRest << 8)
}

// windowFlashing walks the owner chain for a `Flashing() bool` and
// returns the first truthy result. Same dispatch pattern as
// windowTitle / windowFlags.
func (f *Frame) windowFlashing() bool {
	type flasher interface{ Flashing() bool }
	for o := f.Owner; o != nil; o = o.Owner {
		if t, ok := any(o.self).(flasher); ok {
			return t.Flashing()
		}
	}
	return false
}

func (f *Frame) windowNumber() int {
	type numberer interface{ Number() int }
	for o := f.Owner; o != nil; o = o.Owner {
		if t, ok := any(o.self).(numberer); ok {
			return t.Number()
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

	// OnMove fires after a title-bar drag completes, with the window's
	// final origin. Used by hosts (e.g., fvmux) to persist window
	// positions to disk. Fired on mouse-up only, not per cell of the
	// drag — streaming would write the session file thousands of
	// times during a single drag.
	OnMove func(geom.Point)

	// OnResize fires after a resize-handle drag completes, with the
	// window's final size. Same on-completion semantics as OnMove.
	OnResize func(geom.Point)

	// OnClose fires when Close() runs — frame close-box click, an
	// external Close() call, or a modal window's EndModal exit. Hosts
	// use this to drop the window from their own lists, stop attached
	// PTYs, etc. Fired BEFORE the actual removal (Owner.Delete or
	// EndModal), so the window is still live: Owner is non-nil,
	// children are intact, and the callback can walk descendants
	// safely (e.g., to find Terminal children that need Stop()).
	OnClose func()

	// Title-bar flash state. flashUntil is the wall-clock time at
	// which the inverted-titlebar visual reverts to normal. lastFlash
	// is the timestamp of the most recent FlashTitleBar invocation —
	// used to debounce a child program spamming BEL: subsequent
	// calls within 500ms of the previous one are dropped wholesale
	// (no extend, no re-flash) so the title bar can't strobe.
	flashUntil time.Time
	lastFlash  time.Time

	// minSize / maxSize are the resize limits honored by clampResize.
	// Zero on either axis means "no caller-supplied limit on that
	// axis" — the framework's 16×4 hard floor still applies to keep
	// children from getting negative widths, and the Base-default
	// 16384×16384 ceiling still applies. Set via SetSizeLimits.
	minSize geom.Point
	maxSize geom.Point
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
	// GrowMode = 0 means the window stays put when its parent resizes.
	// Using GfGrowAll here would translate every window's origin by
	// the desktop's size delta, dragging windows off-screen on shrink.
	// Maximize-tracking is handled separately in zoom().
	w.GrowMode = 0

	w.Frame = NewFrame(geom.Rect{B: w.Size})
	w.Insert(w.Frame)
}

// GetTypeID for serial registry.
func (w *Window) GetTypeID() string { return "window" }

// Draw paints the window: interior fill in the dialog palette, then
// every child (the Frame is first, so the border overlays the fill;
// content widgets follow), then the shadow if SfShadow is set.
func (w *Window) Draw() {
	// When ForceFullRedraw is on, invalidate every cell within the
	// window's rect first so the diff fires for every cell on every
	// Draw. See ForceFullRedraw's docs for when this is useful.
	if ForceFullRedraw {
		sx, sy := w.ScreenOrigin()
		InvalidateRect(sx, sy, w.Size.X, w.Size.Y)
	}

	// Interior fill: light-gray-on-cyan, the classic TV dialog body.
	bg := theme.Get().WindowBackground
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
	rb := getRootBackend()
	if rb == nil {
		return
	}
	sx, sy := w.ScreenOrigin()
	// Right edge: 2 columns wide, rows 1..h.
	for y := 1; y <= w.Size.Y; y++ {
		for dx := 0; dx < 2; dx++ {
			cellX := sx + w.Size.X + dx
			cellY := sy + y
			under := castShadow(rb.GetCell(cellX, cellY))
			rb.SetCell(cellX, cellY, under)
		}
	}
	// Bottom edge: cols 2..w+1 (offset by 2 so the corner doesn't
	// stick out past the left edge of the window).
	for dx := 2; dx < w.Size.X+2; dx++ {
		cellX := sx + dx
		cellY := sy + w.Size.Y
		under := castShadow(rb.GetCell(cellX, cellY))
		rb.SetCell(cellX, cellY, under)
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
		Attr: theme.Get().WindowShadow,
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

// SetNumber updates the title-bar number badge. Used by hosts that
// renumber windows after one closes (so the user's Alt-1..9 chord
// always maps to a stable position in the list).
func (w *Window) SetNumber(n int) {
	w.number = n
	MarkDirty()
}

// Flashing reports whether the title bar is currently rendering in
// its bell-flash inverted state. Used by Frame.Draw to swap the
// title-bar attrs. Exported so Frame can call it via interface
// assertion through the Owner chain — same pattern as Title / Flags /
// Number.
func (w *Window) Flashing() bool {
	return time.Now().Before(w.flashUntil)
}

// FlashTitleBar inverts the title-bar attributes for d, signalling
// the user where a bell-emitting child program lives. Spam-resistant:
// invocations within 500ms of the most recent call are dropped, so a
// terminal spewing BELs can't strobe the chrome. A time.AfterFunc
// posts a MarkDirty at the end of the flash so the revert happens on
// the main loop, not from inside a parser callback.
func (w *Window) FlashTitleBar(d time.Duration) {
	now := time.Now()
	if !w.lastFlash.IsZero() && now.Sub(w.lastFlash) < 500*time.Millisecond {
		return
	}
	w.lastFlash = now
	w.flashUntil = now.Add(d)
	MarkDirty()
	time.AfterFunc(d, MarkDirty)
}

// HandleEvent extends Group with title-bar drag, close-box and zoom-box
// click handling, click-to-raise for non-modal windows, and the window
// commands cmClose / cmZoom.
//
// At the end of mouse handling we always consume the event if no
// inner child claimed it — otherwise clicks on the upper of two
// overlapping windows fall through to the one below in the parent's
// dispatch loop (which iterates back-to-front but doesn't stop until
// a child sets EvNothing). A window in front of another should block
// clicks even on its empty body cells.
func (w *Window) HandleEvent(ev *drivers.Event) {
	wasMouse := ev.What&consts.EvMouse != 0
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
		// Resize handle: bottom-right corner cell (the ◢ glyph).
		if w.flags&consts.WfGrow != 0 &&
			local.X == w.Size.X-1 && local.Y == w.Size.Y-1 {
			w.resizeLoop(ev)
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
	// Block click-through to windows beneath us. The dispatch loop in
	// Group.HandleEvent only stops when ev.What == EvNothing; we have
	// to be the one that sets it, since the original mouse event might
	// have hit one of our blank body cells (no inner view to claim it).
	if wasMouse && ev.What&consts.EvMouse != 0 {
		w.ClearEvent(ev)
	}
}

// Close ends the modal loop with cmCancel for modal windows; for
// non-modal ones it removes the window from its parent group entirely.
// Exported so menu commands and other code paths can request closing.
// OnClose fires before the actual removal — the window is still
// attached and its children are reachable, so hosts can walk the
// subtree (e.g., to stop Terminal PTYs) before fv-go drops it.
func (w *Window) Close() {
	if w.OnClose != nil {
		w.OnClose()
	}
	if w.GetState(consts.SfModal) {
		w.EndModal(consts.CmCancel)
		return
	}
	if w.Owner != nil {
		w.Owner.Delete(w.self)
	}
}

// close is the lowercase alias retained for the click-handler call
// site; new callers should prefer Close.
func (w *Window) close() { w.Close() }

// SetSizeLimits installs a per-window minimum (and optional maximum)
// resize size. The clampResize path on the drag-resize handle, along
// with any other caller that consults SizeLimits, will honor these.
// Pass a zero component on either axis to leave that bound at the
// framework default — the 16×4 hard floor still applies as the
// absolute backstop in clampResize, and the Base-default 16384×16384
// ceiling still applies for maxima.
//
// Before this method existed, the only way to set a dialog's minimum
// was to subclass Dialog and override SizeLimits (with the matching
// SetSelf gotcha). SetSizeLimits replaces that boilerplate — both
// Window and Dialog inherit it through embedding, so a caller can do
// d := dialogs.NewDialog(...); d.SetSizeLimits(geom.Point{60,12},
// geom.Point{}) without writing a wrapper type.
func (w *Window) SetSizeLimits(min, max geom.Point) {
	w.minSize = min
	w.maxSize = max
}

// SizeLimits returns the configured (min, max). When SetSizeLimits
// has not been called or has been called with a zero component, that
// axis falls back to the Base-default (0,0)–(1<<14, 1<<14).
//
// Concrete subclasses can still override this method outright if they
// want to compute the limits from runtime state (e.g., "min height
// must accommodate however many children exist right now"); the
// virtual dispatch through Self() in clampResize routes to the
// subclass override as before.
func (w *Window) SizeLimits() (geom.Point, geom.Point) {
	defMin, defMax := w.Base.SizeLimits()
	minSz, maxSz := defMin, defMax
	if w.minSize.X > 0 {
		minSz.X = w.minSize.X
	}
	if w.minSize.Y > 0 {
		minSz.Y = w.minSize.Y
	}
	if w.maxSize.X > 0 {
		maxSz.X = w.maxSize.X
	}
	if w.maxSize.Y > 0 {
		maxSz.Y = w.maxSize.Y
	}
	return minSz, maxSz
}

// clampResize honors the view's declared SizeLimits floor (a dialog can
// either call SetSizeLimits or override SizeLimits() to keep its
// buttons / preview pane from being shrunk off-screen), then falls
// back to a hard floor of 16×4 so a missing limit still produces a
// drag-resizable window. Without either, the user could shrink the
// window past its content's left edge, leaving fixed-bound children
// with negative widths that crash MakeDrawBuffer. Extracted from
// resizeLoop for testability.
//
// We route SizeLimits through Self() so an outer struct that embeds
// Window can override the method — calling w.SizeLimits() directly
// would bypass the override, missing the whole point.
func clampResize(w *Window, reqW, reqH int) (int, int) {
	var minSz geom.Point
	if self := w.Self(); self != nil {
		minSz, _ = self.SizeLimits()
	}
	if reqW < minSz.X {
		reqW = minSz.X
	}
	if reqH < minSz.Y {
		reqH = minSz.Y
	}
	if reqW < 16 {
		reqW = 16
	}
	if reqH < 4 {
		reqH = 4
	}
	return reqW, reqH
}

// resizeLoop runs while the user holds the mouse on the bottom-right
// corner. Each motion event recomputes the window's size based on the
// cursor delta and re-issues ChangeBounds so children grow with the
// frame.
func (w *Window) resizeLoop(start *drivers.Event) {
	q := globalQueue.Load()
	if q == nil {
		return
	}
	startSize := w.Size
	startMouse := start.Where

	w.State |= consts.SfDragging
	defer func() {
		w.State &^= consts.SfDragging
		if w.OnResize != nil {
			w.OnResize(w.Size)
		}
	}()
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
		switch ev.What {
		case consts.EvMouseUp:
			return
		case consts.EvMouseMove, consts.EvMouseDown:
			newW, newH := clampResize(
				w,
				startSize.X+(ev.Where.X-startMouse.X),
				startSize.Y+(ev.Where.Y-startMouse.Y),
			)
			if newW != w.Size.X || newH != w.Size.Y {
				w.ChangeBounds(geom.Rect{
					A: w.Origin,
					B: geom.Point{X: w.Origin.X + newW, Y: w.Origin.Y + newH},
				})
			}
		}
	}
}

// dragLoop reads further mouse events until the button releases,
// translating the window by the cursor delta. We pull events directly
// from the global queue so the move is fluid; a redraw happens between
// each event via the pump callback.
func (w *Window) dragLoop(start *drivers.Event) {
	q := globalQueue.Load()
	if q == nil {
		return
	}
	prev := start.Where
	w.State |= consts.SfDragging
	defer func() {
		w.State &^= consts.SfDragging
		if w.OnMove != nil {
			w.OnMove(w.Origin)
		}
	}()
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
				newX := w.Origin.X + dx
				newY := w.Origin.Y + dy
				if w.Owner != nil {
					ext := w.Owner.GetExtent()
					// Keep the title bar accessible: leave at least
					// 8 cells horizontally on screen and don't allow
					// the top edge above row 0 or below the parent's
					// bottom-1.
					minX := 8 - w.Size.X
					maxX := ext.Width() - 8
					if newX < minX {
						newX = minX
					}
					if newX > maxX {
						newX = maxX
					}
					if newY < 0 {
						newY = 0
					}
					if newY > ext.Height()-1 {
						newY = ext.Height() - 1
					}
				}
				w.MoveTo(newX, newY)
				prev = ev.Where
			}
		}
	}
}

func (w *Window) zoom() {
	if w.Owner == nil {
		return
	}
	cur := w.GetBounds()
	if cur.Equals(w.Owner.GetExtent()) {
		// Already maximized — restore. Drop the desktop-tracking
		// grow bits so future desktop resizes don't drag this
		// window around.
		w.GrowMode = 0
		w.ChangeBounds(w.zoomRect)
		return
	}
	// Remember the pre-maximize bounds so the next zoom restores it.
	w.zoomRect = cur
	// While maximized, follow the desktop's right and bottom edges so
	// the window stays full-screen across terminal resizes.
	w.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	w.ChangeBounds(w.Owner.GetExtent())
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
	color := theme.Get().DesktopBackground
	for y := 0; y < b.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(b.Size.X)
		for x := 0; x < b.Size.X; x++ {
			screen.DrawCell(buf, x, string(b.Char), color)
		}
		b.WriteLine(0, y, b.Size.X, 1, buf)
	}
}
