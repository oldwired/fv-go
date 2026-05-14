package views

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// Group owns a collection of child views and routes events / drawing
// to them. The Pascal version uses a circular doubly-linked list with
// a Last+Next pattern; we use a slice — Z-order is back-to-front in
// insertion order, which matches the iteration semantics callers rely
// on.
type Group struct {
	Base

	Children []View
	current  int    // index of focused child; -1 when none
	endState uint16 // command from EndModal that terminates ExecView

	Buffer []byte // (kept for parity; we don't use buffered redraw yet)
	Phase  byte
	Clip   geom.Rect
}

// NewGroup constructs a Group with the given bounds.
func NewGroup(bounds geom.Rect) *Group {
	g := &Group{}
	InitGroup(g, bounds)
	return g
}

// InitGroup initializes g in place. Used by struct-embedding constructors
// (NewWindow, NewDialog, NewProgram, ...) that need to fill in an
// already-allocated Group field rather than struct-copying a new one —
// struct copies break child Owner pointers that were set during Insert.
func InitGroup(g *Group, bounds geom.Rect) {
	g.Base = NewBase(bounds)
	g.SetSelf(g)
	g.Options = consts.OfSelectable | consts.OfBuffered
	g.Clip = g.GetExtent()
	g.current = -1
	g.Children = nil
}

// GetTypeID is the serial.Registry key.
func (g *Group) GetTypeID() string { return "group" }

// First returns the first child or nil.
func (g *Group) First() View {
	if len(g.Children) == 0 {
		return nil
	}
	return g.Children[0]
}

// Last returns the last child or nil.
func (g *Group) Last() View {
	if len(g.Children) == 0 {
		return nil
	}
	return g.Children[len(g.Children)-1]
}

// Current returns the currently focused child or nil.
func (g *Group) Current() View {
	if g.current < 0 || g.current >= len(g.Children) {
		return nil
	}
	return g.Children[g.current]
}

// Insert appends a child. Sets Owner, makes selectable children current
// if no other child has focus, and updates exposure.
func (g *Group) Insert(v View) {
	g.InsertBefore(v, nil)
}

// InnerGroup returns the receiver. Group-embedding views (Window,
// Dialog, Tabs, …) inherit this via method promotion and therefore
// expose their embedded Group to walkers that want to recurse.
func (g *Group) InnerGroup() *Group { return g }

// InsertBefore inserts v immediately before target. If target is nil
// (the common case), v lands at the end.
func (g *Group) InsertBefore(v, target View) {
	if v == nil {
		return
	}
	v.BaseView().Owner = g

	// Honor OfCentered{X,Y} by repositioning v inside g's extent before
	// it joins the children list. Mirrors the Pascal TGroup.Insert
	// behavior where centered views are placed when added.
	if opts := v.BaseView().Options; opts&(consts.OfCenterX|consts.OfCenterY) != 0 {
		bv := v.BaseView()
		x := bv.Origin.X
		y := bv.Origin.Y
		ext := g.GetExtent()
		if opts&consts.OfCenterX != 0 {
			x = (ext.Width() - bv.Size.X) / 2
		}
		if opts&consts.OfCenterY != 0 {
			y = (ext.Height() - bv.Size.Y) / 2
		}
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		bv.Origin = geom.Point{X: x, Y: y}
	}

	// Locate target index; default to len(Children).
	idx := len(g.Children)
	if target != nil {
		for i, c := range g.Children {
			if c == target {
				idx = i
				break
			}
		}
	}

	g.Children = append(g.Children, nil)
	copy(g.Children[idx+1:], g.Children[idx:])
	g.Children[idx] = v

	if v.BaseView().Options&consts.OfSelectable != 0 && g.current < 0 {
		g.current = idx
		v.BaseView().State |= consts.SfSelected | consts.SfFocused
	}
	v.BaseView().State |= consts.SfExposed
	g.refreshActive()
	// Inserting a child changes the visible tree — flag the program dirty
	// so the next idle pass actually paints it. Without this, modal popups
	// (MenuBox, dialogs, FuzzyFinder, …) opened between events never get
	// their first draw, so the user's first keypress appears to do nothing.
	MarkDirty()
}

// refreshActive sets SfActive on the currently-focused child and
// clears it from every other child. Frame.Draw walks up the owner
// chain looking for SfActive to decide between active (double-line)
// and passive (single-line) rendering.
func (g *Group) refreshActive() {
	for i, c := range g.Children {
		if i == g.current {
			c.BaseView().State |= consts.SfActive
		} else {
			c.BaseView().State &^= consts.SfActive
		}
	}
}

// Delete removes v. No-op if v isn't in this group. Also clears
// v.Owner so any "am I still alive?" checks (animation tickers,
// async callbacks) can detect that the view is detached and unhook.
//
// Before detaching we capture the view's screen rect and invalidate
// the cellbuf for that region, so any cell that diff-equalled across
// the removal boundary (typical for SIXEL canvases that emit only
// blanks into the cellbuf) still re-emits and overwrites lingering
// graphics in the terminal.
func (g *Group) Delete(v View) {
	for i, c := range g.Children {
		if c == v {
			sx, sy := c.BaseView().ScreenOrigin()
			sw, sh := c.BaseView().Size.X, c.BaseView().Size.Y
			g.Children = append(g.Children[:i], g.Children[i+1:]...)
			if g.current == i {
				g.current = -1
				if len(g.Children) > 0 {
					g.current = 0
					g.Children[0].BaseView().State |= consts.SfSelected
				}
			}
			g.refreshActive()
			c.BaseView().Owner = nil
			InvalidateRect(sx, sy, sw, sh)
			MarkDirty()
			return
		}
	}
}

// MakeFirst moves v to the end of the children slice, which means it
// will be drawn last (on top) and receive mouse events first. Used by
// non-modal windows to raise themselves on click. Marks the program
// dirty so a programmatic raise (no user event in flight) still
// repaints — Insert/Delete already do this, this brings raise into
// parity.
func (g *Group) MakeFirst(v View) {
	for i, c := range g.Children {
		if c == v {
			if i == len(g.Children)-1 {
				return
			}
			g.Children = append(g.Children[:i], g.Children[i+1:]...)
			g.Children = append(g.Children, v)
			g.current = len(g.Children) - 1
			g.refreshActive()
			MarkDirty()
			return
		}
	}
}

// Focus selects v as the current child. SfSelected and SfFocused are
// always set on v, even if v is already current — this matters because
// initial Insert only marks SfSelected, so a click on the
// already-current child still needs to gain SfFocused. Marks the
// program dirty for the same reason as MakeFirst.
func (g *Group) Focus(v View) {
	for i, c := range g.Children {
		if c == v {
			if i != g.current {
				if cur := g.Current(); cur != nil {
					cur.BaseView().State &^= consts.SfSelected | consts.SfFocused
				}
				g.current = i
			}
			v.BaseView().State |= consts.SfSelected | consts.SfFocused
			g.refreshActive()
			MarkDirty()
			return
		}
	}
}

// SelectNext moves focus to the next selectable child, wrapping around.
// forward=false moves backward.
func (g *Group) SelectNext(forward bool) {
	n := len(g.Children)
	if n == 0 {
		return
	}
	step := 1
	if !forward {
		step = -1
	}
	for off := 1; off <= n; off++ {
		i := (g.current + off*step + n*n) % n
		if g.Children[i].BaseView().Options&consts.OfSelectable != 0 {
			g.Focus(g.Children[i])
			return
		}
	}
}

// HandleEvent walks children. The order matches Pascal's TGroup:
//
//  1. Positional events (mouse) go to the topmost child under the point.
//  2. Keyboard events: views with OfPreProcess get first chance, then
//     the focused child, then OfPostProcess views.
//  3. Broadcast / command events: focused child first, then the rest
//     (so a buried button can catch a cmDefault broadcast).
func (g *Group) HandleEvent(ev *drivers.Event) {
	switch {
	case ev.What&consts.EvMouse != 0:
		for i := len(g.Children) - 1; i >= 0; i-- {
			c := g.Children[i]
			if c.BaseView().GetState(consts.SfVisible) &&
				c.BaseView().MouseInView(ev.Where) {
				c.HandleEvent(ev)
				if ev.What == consts.EvNothing {
					return
				}
			}
		}
	case ev.What&consts.EvKeyboard != 0:
		// PreProcess: every child whose Options has OfPreProcess gets
		// a first look. Iterate in z-order from front to back so a
		// menubar (inserted second) takes priority over a status line.
		for _, c := range g.Children {
			if c.BaseView().Options&consts.OfPreProcess == 0 {
				continue
			}
			if c == g.Current() {
				continue
			}
			c.HandleEvent(ev)
			if ev.What == consts.EvNothing {
				return
			}
		}
		// Focused child.
		if c := g.Current(); c != nil {
			c.HandleEvent(ev)
			if ev.What == consts.EvNothing {
				return
			}
		}
		// PostProcess.
		for _, c := range g.Children {
			if c.BaseView().Options&consts.OfPostProcess == 0 {
				continue
			}
			if c == g.Current() {
				continue
			}
			c.HandleEvent(ev)
			if ev.What == consts.EvNothing {
				return
			}
		}
		// Standard navigation.
		if ev.What == consts.EvKeyDown {
			switch ev.KeyCode {
			case consts.KbTab:
				g.SelectNext(true)
				ev.What = consts.EvNothing
				return
			case consts.KbShiftTab:
				g.SelectNext(false)
				ev.What = consts.EvNothing
				return
			}
		}
	case ev.What&consts.EvBroadcast != 0,
		ev.What&consts.EvCommand != 0:
		if c := g.Current(); c != nil {
			c.HandleEvent(ev)
			if ev.What == consts.EvNothing {
				return
			}
		}
		for _, c := range g.Children {
			if c == g.Current() {
				continue
			}
			c.HandleEvent(ev)
			if ev.What == consts.EvNothing {
				return
			}
		}
	}
}

// Draw paints children back-to-front. Real Z-order clipping is handled
// in the Base.WriteLine path (which writes directly to the backend);
// here we just visit children in order.
func (g *Group) Draw() {
	for _, c := range g.Children {
		if c.BaseView().GetState(consts.SfVisible) {
			c.BaseView().State |= consts.SfExposed
			c.Draw()
		}
	}
}

// ChangeBounds installs new bounds on this group and walks children
// applying their GrowMode. Recurses into nested Groups so a deeply
// nested Frame stretches when an outer Window is resized.
func (g *Group) ChangeBounds(r geom.Rect) {
	oldSize := g.Size
	g.SetBounds(r)
	delta := geom.Point{X: g.Size.X - oldSize.X, Y: g.Size.Y - oldSize.Y}
	if delta.X == 0 && delta.Y == 0 {
		return
	}
	for _, c := range g.Children {
		c.ChangeBounds(c.BaseView().CalcBounds(delta))
	}
}

// EndModal terminates an active ExecView with the given command.
func (g *Group) EndModal(cmd uint16) {
	g.endState = cmd
	g.State |= consts.SfModal
}

// EndStateValue returns the command that EndModal recorded (0 if none).
// Promoted through embedding so Window/Dialog also expose it. ExecView
// uses this via interface to pick up modal-termination requests no
// matter the concrete type.
func (g *Group) EndStateValue() uint16 { return g.endState }

// ClearEndState resets the recorded EndModal command.
func (g *Group) ClearEndState() { g.endState = 0 }

// Valid walks children: a group is valid for a command iff every child
// reports valid. Mirrors TGroup.Valid.
func (g *Group) Valid(command uint16) bool {
	for _, c := range g.Children {
		if !c.Valid(command) {
			return false
		}
	}
	return true
}

// ExecView runs v as a modal child of g, returning the cmXxx command
// that terminated the modal loop.
func (g *Group) ExecView(v View) uint16 {
	if v == nil {
		return consts.CmCancel
	}
	bv := v.BaseView()
	bv.State |= consts.SfModal | consts.SfVisible | consts.SfExposed
	g.Insert(v)
	g.Focus(v)
	defer g.Delete(v)

	if globalQueue == nil {
		return consts.CmCancel
	}
	type endStater interface {
		EndStateValue() uint16
		ClearEndState()
	}
	for {
		if pumpFn != nil {
			pumpFn()
		}
		// Check for an EndModal request that came from a non-event
		// path (animation tick, asynchronous broadcast). Without this,
		// timer-driven dialogs would only close when the user happens
		// to press a key.
		if es, ok := v.(endStater); ok {
			if cmd := es.EndStateValue(); cmd != 0 {
				es.ClearEndState()
				return cmd
			}
		}
		ev, ok := globalQueue.Get()
		if !ok {
			if waitFn != nil {
				waitFn()
			}
			continue
		}
		// Look for a modal-terminating command before dispatch. We
		// still call v.HandleEvent so child views can run their own
		// cleanup (Dialog ClearEvent, Validators, etc.), but we
		// remember the command and exit afterwards.
		var modalCmd uint16
		if ev.What == consts.EvCommand {
			switch ev.Command {
			case consts.CmOK, consts.CmCancel, consts.CmYes, consts.CmNo:
				if v.Valid(ev.Command) {
					modalCmd = ev.Command
				} else {
					ev.What = consts.EvNothing
				}
			}
		}
		v.HandleEvent(&ev)
		MarkDirty()
		// Pick up EndModal even when v's concrete type is *Dialog or
		// *Window — both inherit EndStateValue via Group embedding.
		// Direct *Group type assertion misses those.
		if es, ok := v.(interface {
			EndStateValue() uint16
			ClearEndState()
		}); ok {
			if cmd := es.EndStateValue(); cmd != 0 {
				es.ClearEndState()
				return cmd
			}
		}
		if modalCmd != 0 {
			return modalCmd
		}
		if cmd := captureModalCmd(&ev); cmd != 0 {
			return cmd
		}
	}
}

// pumpFn / waitFn / dirtyFn / callSoonFn are set by app.NewProgram so
// modal loops (ExecView, MenuBox.Run, popupmenu.Run, fuzzyfinder.Run,
// etc.) can drive the same idle-redraw + blocking-input cycle the main
// Program.Run uses, without owning the goroutine. callSoonFn lets
// widget code (Terminal, async data sources) marshal a callback back
// onto the UI goroutine without importing pkg/fv/app.
var (
	pumpFn     func()          // drain term events, redraw, flush; non-blocking
	waitFn     func()          // block until at least one event is queued
	dirtyFn    func()          // mark the program dirty so the next idle redraws
	callSoonFn func(fn func()) // schedule fn on the UI goroutine
)

// SetPump installs the idle callback (drain+draw+flush).
func SetPump(f func()) { pumpFn = f }

// SetWait installs the blocking-event-wait callback.
func SetWait(f func()) { waitFn = f }

// SetMarkDirty installs the mark-dirty callback.
func SetMarkDirty(f func()) { dirtyFn = f }

// SetCallSoon installs the scheduler callback used by CallSoon.
// app.NewProgram wires this to Program.CallSoon.
func SetCallSoon(f func(fn func())) { callSoonFn = f }

// MarkDirty asks the program to repaint on the next idle pass.
// Modal loops call this after handling each event so the
// pump+pulse+dirty triad correctly registers "state has changed".
func MarkDirty() {
	if dirtyFn != nil {
		dirtyFn()
	}
}

// CallSoon delivers fn on the UI goroutine via the installed scheduler.
//
// IMPORTANT: when no Program is installed (typically in unit tests
// that instantiate views without a runtime), CallSoon executes fn
// inline on the caller's goroutine. Tests that care about goroutine
// affinity must install a scheduler via SetCallSoon. This is
// bootstrap-only fallback behavior — do not rely on it in production.
func CallSoon(fn func()) {
	if fn == nil {
		return
	}
	if callSoonFn != nil {
		callSoonFn(fn)
		return
	}
	fn()
}

func captureModalCmd(ev *drivers.Event) uint16 {
	if ev.What != consts.EvCommand {
		return 0
	}
	switch ev.Command {
	case consts.CmOK, consts.CmCancel, consts.CmYes, consts.CmNo, consts.CmQuit:
		return ev.Command
	}
	return 0
}
