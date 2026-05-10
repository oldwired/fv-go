// Package views ports Views.pas: View, Group, Frame, Window, ScrollBar,
// Scroller, ListViewer, Background.
//
// Inheritance maps to Go interface + struct embedding. The View interface
// declares the polymorphic methods (Draw, HandleEvent, GetPalette,
// GetData/SetData, SizeLimits, etc.). The Base struct implements the
// non-polymorphic state every view shares (origin, size, owner, state
// flags) plus default behavior for the polymorphic methods.
//
// Concrete subtypes (Window, Dialog, Button, ...) embed Base and shadow
// only the methods they care about. To make virtual dispatch work, Base
// holds a `self View` interface field that's set during construction;
// when Base methods need to call back into the concrete type they go
// through `b.self`.
package views

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
)

// View is the polymorphic contract every visible thing in fv-go satisfies.
// The non-polymorphic state lives in Base; only the methods listed here
// are subject to virtual dispatch (i.e., parent groups call them via
// the interface, so concrete subtypes can shadow them).
type View interface {
	// State + identity
	BaseView() *Base
	GetTypeID() string

	// Layout
	SizeLimits() (min, max geom.Point)
	// ChangeBounds installs new bounds and propagates to children
	// according to their GrowMode. Concrete Group types override the
	// default Base impl to recurse.
	ChangeBounds(r geom.Rect)

	// Lifecycle
	SetState(state uint16, enable bool)
	HandleEvent(ev *drivers.Event)

	// Rendering
	Draw()
	GetPalette() []byte

	// Data binding
	DataSize() int
	GetData(buf []byte)
	SetData(buf []byte)
	Valid(command uint16) bool
}

// Base is embedded by every concrete view. It holds the shared state
// from TView: position, size, ownership, flags, palette walk-up, the
// command set, and the helpers Group / Frame need to lay out children.
type Base struct {
	Owner  *Group
	Origin geom.Point
	Size   geom.Point
	Cursor geom.Point

	State    uint16
	Options  uint16
	GrowMode byte
	DragMode byte
	HelpCtx  uint16

	EventMask uint16
	Commands  drivers.CommandSet

	self View // virtual-dispatch back-pointer; set by SetSelf
}

// SetSelf installs the virtual-dispatch back-pointer. Constructors of
// concrete types must call this so that Base methods which need to
// invoke a subclass override go through the interface.
func (b *Base) SetSelf(v View) { b.self = v }

// Self returns the concrete view, or nil if SetSelf hasn't been called.
func (b *Base) Self() View { return b.self }

// BaseView is the View interface accessor for Base.
func (b *Base) BaseView() *Base { return b }

// NewBase initializes a Base with the FV-default option flags and the
// given bounds (Origin = bounds.A, Size = (Width, Height)).
func NewBase(bounds geom.Rect) Base {
	return Base{
		Origin:    bounds.A,
		Size:      geom.Point{X: bounds.Width(), Y: bounds.Height()},
		State:     consts.SfVisible,
		EventMask: consts.EvMouseDown | consts.EvKeyDown | consts.EvCommand,
	}
}

// Default implementations follow. Concrete types shadow whichever ones
// need divergent behavior.

func (b *Base) GetTypeID() string { return "view" }

func (b *Base) SizeLimits() (geom.Point, geom.Point) {
	return geom.Point{X: 0, Y: 0}, geom.Point{X: 1 << 14, Y: 1 << 14}
}

func (b *Base) SetState(state uint16, enable bool) {
	if enable {
		b.State |= state
	} else {
		b.State &^= state
	}
}

func (b *Base) GetState(state uint16) bool { return b.State&state == state }

func (b *Base) HandleEvent(ev *drivers.Event) {
	// Default: handle nothing.
}

func (b *Base) Draw() {
	// Default: clear our extent to a blank background.
	for y := 0; y < b.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(b.Size.X)
		b.WriteLine(0, y, b.Size.X, 1, buf)
	}
}

func (b *Base) GetPalette() []byte { return nil }

func (b *Base) DataSize() int      { return 0 }
func (b *Base) GetData(buf []byte) {}
func (b *Base) SetData(buf []byte) {}
func (b *Base) Valid(uint16) bool  { return true }

// MoveTo translates Origin to (x, y) without resizing.
func (b *Base) MoveTo(x, y int) {
	r := geom.Rect{
		A: geom.Point{X: x, Y: y},
		B: geom.Point{X: x + b.Size.X, Y: y + b.Size.Y},
	}
	b.SetBounds(r)
}

// GrowTo resizes the view, keeping Origin.
func (b *Base) GrowTo(w, h int) {
	r := geom.Rect{
		A: b.Origin,
		B: geom.Point{X: b.Origin.X + w, Y: b.Origin.Y + h},
	}
	b.SetBounds(r)
}

// SetBounds installs new origin + size. Subclasses can intercept by
// overriding; the default just records.
func (b *Base) SetBounds(r geom.Rect) {
	b.Origin = r.A
	b.Size = geom.Point{X: r.Width(), Y: r.Height()}
}

// ChangeBounds is the polymorphic resize entry point. The Base default
// just sets the bounds — Group overrides to also recompute child
// bounds via GrowMode (so resizing a Window stretches its Frame).
func (b *Base) ChangeBounds(r geom.Rect) {
	b.SetBounds(r)
}

// CalcBounds computes the new bounds this view should occupy when its
// parent grew by delta. Each gf*Grow* bit pulls one corner along.
func (b *Base) CalcBounds(delta geom.Point) geom.Rect {
	cur := b.GetBounds()
	if b.GrowMode&consts.GfGrowLoX != 0 {
		cur.A.X += delta.X
	}
	if b.GrowMode&consts.GfGrowLoY != 0 {
		cur.A.Y += delta.Y
	}
	if b.GrowMode&consts.GfGrowHiX != 0 {
		cur.B.X += delta.X
	}
	if b.GrowMode&consts.GfGrowHiY != 0 {
		cur.B.Y += delta.Y
	}
	return cur
}

// GetBounds returns the rectangle (Origin, Origin+Size).
func (b *Base) GetBounds() geom.Rect {
	return geom.Rect{
		A: b.Origin,
		B: geom.Point{X: b.Origin.X + b.Size.X, Y: b.Origin.Y + b.Size.Y},
	}
}

// GetExtent returns the local-coordinate rectangle (0,0)-(Size.X,Size.Y).
func (b *Base) GetExtent() geom.Rect {
	return geom.Rect{B: b.Size}
}

// MakeLocal converts a global point to view-local coordinates.
func (b *Base) MakeLocal(p geom.Point) geom.Point {
	g := b.globalOrigin()
	return geom.Point{X: p.X - g.X, Y: p.Y - g.Y}
}

// MakeGlobal converts a view-local point to global coordinates.
func (b *Base) MakeGlobal(p geom.Point) geom.Point {
	g := b.globalOrigin()
	return geom.Point{X: p.X + g.X, Y: p.Y + g.Y}
}

// globalOrigin walks up the owner chain to compute screen-coordinate
// origin.
func (b *Base) globalOrigin() geom.Point {
	x, y := b.Origin.X, b.Origin.Y
	for o := b.Owner; o != nil; o = o.Owner {
		x += o.Origin.X
		y += o.Origin.Y
	}
	return geom.Point{X: x, Y: y}
}

// Show / Hide toggle SfVisible.
func (b *Base) Show() {
	if !b.GetState(consts.SfVisible) {
		b.self.SetState(consts.SfVisible, true)
	}
}

func (b *Base) Hide() {
	if b.GetState(consts.SfVisible) {
		b.self.SetState(consts.SfVisible, false)
	}
}

// MouseInView reports whether a global point lies inside this view's
// extent.
func (b *Base) MouseInView(p geom.Point) bool {
	return b.GetBounds().Move(b.globalOriginDelta()).Contains(p)
}

// globalOriginDelta returns the delta from the local Origin to the
// global origin (i.e., the parent's accumulated offset).
func (b *Base) globalOriginDelta() (int, int) {
	dx, dy := 0, 0
	for o := b.Owner; o != nil; o = o.Owner {
		dx += o.Origin.X
		dy += o.Origin.Y
	}
	return dx, dy
}

// PutEvent re-injects an event into the program queue. Stub: bound by
// the App layer at startup via SetEventQueue.
var globalQueue *drivers.Queue

// SetEventQueue is called once by app.NewProgram to plug in the queue.
func SetEventQueue(q *drivers.Queue) { globalQueue = q }

// GetEventQueue returns the program-wide queue (set by app.NewProgram),
// or nil if not yet wired. Used by MenuBox.Run, ExecView, and similar
// loops that need to pull events synchronously.
func GetEventQueue() *drivers.Queue { return globalQueue }

// GetPump returns the pump callback set by app.NewProgram, or nil.
// MenuBox / dialogs call it to drive event collection during a modal
// loop without owning the program loop.
func GetPump() func() { return pumpFn }

// GetWait returns the blocking-wait callback. Modal loops call it when
// their queue is empty so they don't busy-spin.
func GetWait() func() { return waitFn }

func (b *Base) PutEvent(ev *drivers.Event) {
	if globalQueue != nil {
		globalQueue.Put(*ev)
	}
}

// ClearEvent marks the event as consumed, attributing it to this view.
func (b *Base) ClearEvent(ev *drivers.Event) {
	ev.What = consts.EvNothing
	ev.InfoPtr = b.self
}

// WriteLine emits one row of the view's draw buffer to the screen via
// the parent group's clip + Z-order machinery. With no owner (root
// program), it writes directly to the Backend.
func (b *Base) WriteLine(x, y, w, h int, buf screen.DrawBuffer) {
	if !b.GetState(consts.SfExposed | consts.SfVisible) {
		return
	}
	gx, gy := b.globalOriginDelta()
	gx += b.Origin.X
	gy += b.Origin.Y
	if rootBackend == nil {
		return
	}
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			if col >= len(buf) {
				break
			}
			// In this pared-down port, Z-order clipping happens at the
			// Group layer (Group.dispatchDraw collects rows and only
			// hands rows to children that intersect their dirty rect).
			rootBackend.SetCell(gx+x+col, gy+y+row, buf[col])
		}
	}
}

// WriteBuf is the rectangular form of WriteLine.
func (b *Base) WriteBuf(x, y, w, h int, buf screen.DrawBuffer) {
	for row := 0; row < h; row++ {
		offset := row * w
		if offset >= len(buf) {
			return
		}
		end := offset + w
		if end > len(buf) {
			end = len(buf)
		}
		b.WriteLine(x, y+row, end-offset, 1, buf[offset:end])
	}
}

// WriteStr is convenience: build a buffer and emit one line.
func (b *Base) WriteStr(x, y int, s string, color byte) {
	buf := screen.MakeDrawBuffer(b.Size.X)
	screen.DrawStr(buf, 0, s, types.MakeAttr(color, 0))
	b.WriteLine(x, y, b.Size.X, 1, buf)
}

// WriteChar is convenience.
func (b *Base) WriteChar(x, y int, c rune, color byte, count int) {
	buf := screen.MakeDrawBuffer(b.Size.X)
	screen.DrawChar(buf, 0, c, types.MakeAttr(color, 0), count)
	b.WriteLine(x, y, count, 1, buf)
}

// rootBackend is set by app.Application.Init so the view tree can write
// cells. Tests can leave it nil — WriteLine becomes a no-op.
var rootBackend RootBackend

// RootBackend is the slice of term.Backend that views can call directly.
type RootBackend interface {
	SetCell(x, y int, c types.DrawCell)
	GetCell(x, y int) types.DrawCell
	Flush() error
}

// SetRootBackend wires the screen target. Called by app.Application.Init.
func SetRootBackend(rb RootBackend) { rootBackend = rb }

// Flush pushes the current back buffer to the terminal. Returns nil if
// no backend is wired.
func Flush() error {
	if rootBackend == nil {
		return nil
	}
	return rootBackend.Flush()
}

// GetCell reads one cell of the current back buffer at screen coordinates
// (x, y). Used by shadow rendering to preserve underlying glyphs.
func GetCell(x, y int) types.DrawCell {
	if rootBackend == nil {
		return types.DrawCell{}
	}
	return rootBackend.GetCell(x, y)
}

// ScreenOrigin returns the screen-coords origin of this view (Origin
// plus accumulated ancestor origins). Useful for views that need to
// read cells outside their own bounds.
func (b *Base) ScreenOrigin() (int, int) {
	dx, dy := b.globalOriginDelta()
	return dx + b.Origin.X, dy + b.Origin.Y
}

// Alive reports whether this view is still attached to a parent
// group. Group.Delete clears Owner, so detached views report false —
// which lets the anim package's Pulse drop tickers whose view has
// been removed (e.g., a closed gadget window).
func (b *Base) Alive() bool { return b.Owner != nil }

// GetColor maps a color index through the view's owner chain palette.
// For now this is the identity unless a Group overrides MapColor.
func (b *Base) GetColor(color uint16) uint16 {
	pal := b.self.GetPalette()
	idx := color & 0xFF
	if idx > 0 && pal != nil && int(idx-1) < len(pal) {
		return uint16(pal[idx-1])
	}
	return color
}
