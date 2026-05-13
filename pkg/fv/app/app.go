// Package app ports App.pas: Program, Application, Desktop, Background.
//
// Application is the entry point: it owns the term backend, the event
// queue, and a tree (MenuBar over Desktop over StatusLine) that mirrors
// the canonical Turbo Vision layout. Run loops until Quit().
package app

import (
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/clipboard"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/help"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Program is the top-level group: it owns the menu bar, status line,
// desktop, and runs the event loop.
type Program struct {
	views.Group

	MenuBar    views.View
	StatusLine views.View
	Desktop    *Desktop

	backend   term.Backend
	queue     *drivers.Queue
	quit      bool
	dirty     bool          // set when an event/anim/etc. has changed state and a redraw is owed
	waitTimer *time.Timer   // reused across waitOne() calls to skip per-wake allocation
	wake      chan struct{} // signaled by MarkDirty from goroutines so waitOne returns
	// without needing a fresh user keystroke (PTY output, async data, …).

	// OnCommand, if non-nil, is invoked for every EvCommand event the
	// view tree didn't consume. Returning true marks the command as
	// handled and prevents further propagation.
	OnCommand func(cmd uint16, ev *drivers.Event) bool

	// OnQuitRequest, if non-nil, runs when CmQuitApp arrives at the
	// main loop. Returning false vetoes the quit (the event is
	// swallowed); returning true lets the program exit normally. Use
	// this to surface "really quit? N running terminals" dialogs
	// without poking around in OnCommand.
	OnQuitRequest func() (proceed bool)

	// OnPanic, if non-nil, runs in the deferred recover inside Run()
	// when a panic propagates out of the main loop. After OnPanic
	// returns, Done() is called to clean up children (PTYs, …) and
	// the panic is re-thrown so the caller still sees it. Hosts can
	// use this to log a crash report before the process dies.
	OnPanic func(recovered any)
}

// Stoppable is implemented by views that own external resources (PTYs,
// file watchers, network connections, …) and need a deterministic
// teardown when the program exits. Application.Done() walks the tree
// and calls Stop() on each descendant that satisfies this — Terminal
// uses it to send SIGHUP to its child shell.
type Stoppable interface {
	Stop()
}

// NewProgram constructs an empty Program. App.Init wires the backend.
func NewProgram(backend term.Backend) *Program {
	cols, rows := backend.Size()
	bounds := geom.NewRect(0, 0, cols, rows)
	p := &Program{backend: backend}
	p.wake = make(chan struct{}, 1)
	views.InitGroup(&p.Group, bounds)
	p.SetSelf(p)
	p.GrowMode = consts.GfGrowAll
	p.queue = drivers.NewQueue()
	views.SetEventQueue(p.queue)
	views.SetPump(p.idle)
	views.SetWait(p.waitOne)
	views.SetMarkDirty(func() {
		p.dirty = true
		// Non-blocking nudge to wake waitOne if it's parked. We don't
		// care about coalescing — a full buffer means a wake is
		// already pending, which is exactly what we want.
		select {
		case p.wake <- struct{}{}:
		default:
		}
	})
	views.SetRootBackend(backend)
	// A theme.Set() during the program's lifetime should repaint.
	// Wired here (not in the theme package) to keep the theme package
	// free of any views dependency.
	theme.SetOnChange(views.MarkDirty)
	return p
}

// GetTypeID for serial registry.
func (p *Program) GetTypeID() string { return "program" }

// HandleEvent overrides Group.HandleEvent to translate the standard
// global shortcuts into commands before walking children. F1 → cmHelp,
// F5 → cmZoom, F6 / Shift+F6 → cmNext / cmPrev, Alt+F3 → cmClose,
// Alt+0..9 → cmSelectWindowNum. After translation we still call the
// embedded Group dispatch so other views can react.
func (p *Program) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvKeyDown {
		// Skip global shortcut translation if the focused view chain
		// claims raw keyboard (Terminal does this so Ctrl+C reaches
		// the embedded shell as SIGINT instead of being eaten as a
		// Copy command). We still let F1/F5/etc. through to be
		// captured if no raw-key view is in focus.
		if p.focusWantsRawKeys() {
			p.Group.HandleEvent(ev)
			return
		}
		var emit uint16
		var info int16
		// F1: if focus advertises a help context, open the help view
		// directly. Otherwise fall through to the legacy CmHelp emit
		// so apps that wire their own help handler still work.
		if ev.KeyCode == consts.KbF1 {
			if ctx := p.helpCtxFromFocus(); ctx != 0 {
				help.Show(&p.Desktop.Group, ctx)
				ev.What = consts.EvNothing
				return
			}
		}
		switch ev.KeyCode {
		case consts.KbF1:
			emit = consts.CmHelp
		case consts.KbF5:
			emit = consts.CmZoom
		case consts.KbF6:
			emit = consts.CmNext
		case consts.KbShiftF6:
			emit = consts.CmPrev
		case consts.KbAltF3:
			emit = consts.CmClose
		case consts.KbCtrlIns:
			emit = consts.CmCopy
		case consts.KbShiftIns:
			emit = consts.CmPaste
		case consts.KbShiftDel:
			emit = consts.CmCut
		}
		// Alt+digit: select Nth window. The reader puts the digit in
		// UnicodeChar when ModAlt is held.
		if emit == 0 && ev.KeyShift&consts.KbAltShift != 0 &&
			ev.UnicodeChar >= '0' && ev.UnicodeChar <= '9' {
			emit = consts.CmSelectWindowNum
			info = int16(ev.UnicodeChar - '0')
		}
		// Ctrl+C / Ctrl+X / Ctrl+V → clipboard commands. We use the
		// scan-coded form so plain typing doesn't trigger them. Note
		// that Cmd+V on macOS goes through the terminal's bracketed
		// paste path independently of these.
		if emit == 0 {
			switch ev.KeyCode {
			case consts.KbCtrlC:
				emit = consts.CmCopy
			case consts.KbCtrlX:
				emit = consts.CmCut
			case consts.KbCtrlV:
				emit = consts.CmPaste
			}
		}
		if emit != 0 {
			cmd := drivers.Event{
				What:    consts.EvCommand,
				Command: emit,
				InfoInt: info,
			}
			p.queue.Put(cmd)
			ev.What = consts.EvNothing
			return
		}
	}
	p.Group.HandleEvent(ev)
}

// focusWantsRawKeys walks the focus chain looking for any view with
// SfRawKeys set in its State. Terminal sets this when it has focus so
// embedded shells get Ctrl+C / Ctrl+X / Ctrl+V / F-keys instead of
// our clipboard and window-management shortcuts.
func (p *Program) focusWantsRawKeys() bool {
	type currenter interface{ Current() views.View }
	var v views.View = p
	for {
		bv := v.BaseView()
		if bv != nil && bv.State&consts.SfRawKeys != 0 {
			return true
		}
		c, ok := v.(currenter)
		if !ok {
			return false
		}
		next := c.Current()
		if next == nil || next == v {
			return false
		}
		v = next
	}
}

// helpCtxFromFocus walks the focus chain to find the nearest non-zero
// HelpCtx. Returns 0 if no focused view advertises a context.
func (p *Program) helpCtxFromFocus() uint16 {
	type currenter interface{ Current() views.View }
	var v views.View = p
	for {
		if bv := v.BaseView(); bv != nil && bv.HelpCtx != 0 {
			return bv.HelpCtx
		}
		c, ok := v.(currenter)
		if !ok {
			return 0
		}
		next := c.Current()
		if next == nil || next == v {
			return 0
		}
		v = next
	}
}

// SetMenuBar inserts a menu bar above the desktop.
func (p *Program) SetMenuBar(v views.View) {
	if p.MenuBar != nil {
		p.Delete(p.MenuBar)
	}
	p.MenuBar = v
	if v != nil {
		p.Insert(v)
	}
}

// SetStatusLine inserts a status line below the desktop.
func (p *Program) SetStatusLine(v views.View) {
	if p.StatusLine != nil {
		p.Delete(p.StatusLine)
	}
	p.StatusLine = v
	if v != nil {
		p.Insert(v)
	}
}

// SetDesktop installs the desktop area between menu bar and status.
func (p *Program) SetDesktop(d *Desktop) {
	if p.Desktop != nil {
		p.Delete(p.Desktop)
	}
	p.Desktop = d
	if d != nil {
		p.Insert(d)
	}
}

// Quit terminates the next pass through the event loop.
func (p *Program) Quit() { p.quit = true }

// Run drives the program loop until Quit() is called or cmQuit fires.
//
// If OnPanic is set, panics propagating out of the loop are caught,
// passed to OnPanic, the program's Done() cleanup runs, and the panic
// is re-thrown so the caller still observes the crash. This is
// belt-and-braces with `defer app.Done()` at the call site — the
// recover ensures PTY children get SIGHUP even if Done was never
// deferred (e.g., during a unit test).
func (p *Program) Run() {
	if p.OnPanic != nil {
		defer func() {
			if r := recover(); r != nil {
				p.OnPanic(r)
				// If we're embedded in an Application, this Done call
				// is redundant with the caller's `defer app.Done()`
				// but harmless — Stop() / Close() are idempotent.
				if a, ok := any(p).(interface{ Done() }); ok {
					a.Done()
				}
				panic(r)
			}
		}()
	}
	p.State |= consts.SfActive | consts.SfVisible | consts.SfExposed
	p.dirty = true // initial paint
	for !p.quit {
		p.idle()
		ev, ok := p.queue.Get()
		if !ok {
			p.waitOne()
			continue
		}
		// Terminal resize: the term backend fires evCommand+cmResizeApp
		// with InfoPtr = geom.Point{cols, rows}. Resize ourselves; the
		// GrowMode plumbing on MenuBar / Desktop / StatusLine carries
		// the change down.
		if ev.What == consts.EvCommand && ev.Command == consts.CmResizeApp {
			if pt, ok := ev.InfoPtr.(geom.Point); ok {
				p.ChangeBounds(geom.NewRect(0, 0, pt.X, pt.Y))
			}
			p.dirty = true
			continue
		}
		if ev.What == consts.EvCommand && ev.Command == consts.CmQuitApp {
			if !p.acceptQuit() {
				p.dirty = true
				continue
			}
			return
		}
		if ev.What == consts.EvCommand && ev.Command == consts.CmQuit {
			return
		}
		p.HandleEvent(&ev)
		if ev.What == consts.EvCommand && p.OnCommand != nil {
			if p.OnCommand(ev.Command, &ev) {
				ev.What = consts.EvNothing
			}
		}
		if ev.What == consts.EvCommand && ev.Command == consts.CmQuitApp {
			if !p.acceptQuit() {
				p.dirty = true
				continue
			}
			return
		}
		// Anything we just handled could have changed visible state.
		p.dirty = true
	}
}

// acceptQuit consults OnQuitRequest. Returns true if the program is
// allowed to exit (no callback or callback says yes), false to veto.
func (p *Program) acceptQuit() bool {
	if p.OnQuitRequest == nil {
		return true
	}
	return p.OnQuitRequest()
}

// PostEvent queues ev for the main loop to consume on its next pass.
// Goroutine-safe; also nudges the wake channel so a parked waitOne
// returns immediately instead of sleeping until the next user input.
// Useful for synthetic command events from background workers and
// from scripted tests.
func (p *Program) PostEvent(ev drivers.Event) {
	if p.queue != nil {
		p.queue.Put(ev)
	}
	p.dirty = true
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// idle drains queued term events into the FV queue, advances any
// registered animation tickers, then redraws the view tree and flushes
// the backend. Called both by Run between events and by modal loops
// (MenuBox.Run, Group.ExecView) that need a redraw while they wait
// for input.
//
// Order of operations:
//
//  1. p.draw()    — populates the cell buffer.
//  2. preFlush()  — lets PreFlusher views (SIXEL) react to the final
//     cell layout: they emit DCS, then mark their own
//     cells "clean" (don't emit) and covering cells
//     "dirty" (force emit, painting over SIXEL pixels).
//  3. Flush()     — emits the cell diff to the terminal.
func (p *Program) idle() {
	pumped := p.pump()
	animDirty := anim.Pulse()
	if !p.dirty && !pumped && !animDirty {
		return
	}
	// Clear the flag BEFORE drawing, not after. If a goroutine (e.g.,
	// the Terminal view's PTY read loop) fires MarkDirty while we're
	// in the middle of a draw, we want the mark to survive into the
	// next idle. Clearing at the end would stomp it, and the wake
	// channel would unblock waitOne only for idle to find dirty=false
	// and skip — manifesting as "typed character only appears after
	// the next keystroke".
	p.dirty = false
	p.draw()
	p.preFlush()
	_ = p.backend.Flush()
}

// preFlush walks the view tree calling PreFlush on any view that
// implements it. The walk is depth-first, post-order so children fire
// before their parents — same shape as Group.Draw.
func (p *Program) preFlush() {
	walkPreFlush(&p.Group, p.backend)
}

func walkPreFlush(g *views.Group, b views.RootBackend) {
	for _, c := range g.Children {
		if !c.BaseView().GetState(consts.SfVisible) {
			continue
		}
		// Group-embedding views (Window, Dialog, Tabs, …) get their
		// embedded Group via the InnerGroup() promotion. Plain leaf
		// views won't satisfy the interface and are skipped.
		if gi, ok := c.(interface{ InnerGroup() *views.Group }); ok {
			if inner := gi.InnerGroup(); inner != nil && inner != g {
				walkPreFlush(inner, b)
			}
		}
		if pf, ok := c.(views.PreFlusher); ok {
			pf.PreFlush(b)
		}
	}
}

// MarkDirty asks the program to repaint on the next idle pass. Handy
// when external code (e.g., async data arriving) mutates a view's
// state outside the event/anim path.
func (p *Program) MarkDirty() { p.dirty = true }

// pump drains term events into the FV queue. Returns true if at least
// one event was pushed — the idle loop uses that to decide whether
// it needs to redraw.
func (p *Program) pump() bool {
	any := false
	for {
		select {
		case te := <-p.backend.Events():
			if e := drivers.FromTermEvent(te); e.What != 0 {
				p.queue.Put(e)
				any = true
			}
		default:
			return any
		}
	}
}

// waitOne blocks until at least one term event is read and pushed onto
// the FV queue, or until the next animation pulse is due. Used by
// ExecView / MenuBox.runIn when their queue is empty between input
// events. With no animations registered the wait is unbounded; with
// at least one ticker it caps at the smallest interval so animations
// stay responsive while idle.
//
// Setting dirty=true on every pumped event is intentional belt-and-
// braces: modal loops + the main Run loop also MarkDirty after they
// hand the event off, but a previous attempt at "dirty only on handle"
// regressed menu interaction (a click → activate → modal-return path
// could land on the user's screen without an intervening repaint of
// the popup state, so the user saw "click twice"). One extra draw per
// input event is a fair trade for never missing one.
//
// The timer is reused across calls to avoid allocating one per wake
// (anim intervals as low as 50ms otherwise mean 20 timers/sec of GC
// churn for a process that's otherwise idle).
func (p *Program) waitOne() {
	if d := anim.MinInterval(); d > 0 {
		if p.waitTimer == nil {
			p.waitTimer = time.NewTimer(d)
		} else {
			if !p.waitTimer.Stop() {
				select {
				case <-p.waitTimer.C:
				default:
				}
			}
			p.waitTimer.Reset(d)
		}
		select {
		case te, alive := <-p.backend.Events():
			if !alive {
				return
			}
			if e := drivers.FromTermEvent(te); e.What != 0 {
				p.queue.Put(e)
				p.dirty = true
			}
		case <-p.waitTimer.C:
		case <-p.wake:
			// Async MarkDirty (e.g., Terminal's PTY read goroutine
			// got new output) — return so the next idle() can draw.
		}
		return
	}
	select {
	case te, alive := <-p.backend.Events():
		if !alive {
			return
		}
		if e := drivers.FromTermEvent(te); e.What != 0 {
			p.queue.Put(e)
			p.dirty = true
		}
	case <-p.wake:
		// See above.
	}
}

func (p *Program) draw() {
	p.backend.Clear(0)
	p.Draw()
	p.placeCursor()
}

// placeCursor walks the focus chain to find the deepest focused view;
// if it has SfCursorVis, the terminal cursor is positioned at that
// view's local Cursor coords (translated to screen space). Otherwise
// the cursor is hidden. Without this, no view ever shows a caret.
//
// The walk uses an interface assertion rather than `*views.Group`
// because Window, Dialog, Program, Tabs, Accordion etc. *embed*
// Group as a value field — they're not *Group themselves but they
// inherit Current() through method promotion.
func (p *Program) placeCursor() {
	type currenter interface {
		Current() views.View
	}
	var leaf views.View = p
	for {
		c, ok := leaf.(currenter)
		if !ok {
			break
		}
		next := c.Current()
		if next == nil {
			break
		}
		leaf = next
	}
	if leaf == nil {
		p.backend.SetCursor(-1, -1)
		p.backend.ShowCursor(false)
		return
	}
	bv := leaf.BaseView()
	if bv == nil || bv.State&consts.SfCursorVis == 0 || bv.State&consts.SfFocused == 0 {
		p.backend.SetCursor(-1, -1)
		p.backend.ShowCursor(false)
		return
	}
	if bv.Cursor.X < 0 || bv.Cursor.Y < 0 ||
		bv.Cursor.X >= bv.Size.X || bv.Cursor.Y >= bv.Size.Y {
		// Caret outside the view's visible bounds — hide.
		p.backend.SetCursor(-1, -1)
		p.backend.ShowCursor(false)
		return
	}
	gx, gy := bv.ScreenOrigin()
	p.backend.SetCursor(gx+bv.Cursor.X, gy+bv.Cursor.Y)
	p.backend.ShowCursor(true)
}

// Application wraps Program with backend lifecycle (Init / Done).
type Application struct {
	*Program
}

// NewApplication initializes the term backend and constructs an empty
// Application. Caller must call Done before exit.
func NewApplication() (*Application, error) {
	be := term.New()
	if err := be.Init(); err != nil {
		return nil, err
	}
	a := &Application{Program: NewProgram(be)}
	cols, rows := be.Size()
	desktopBounds := geom.NewRect(0, 1, cols, rows-1)
	a.SetDesktop(NewDesktop(desktopBounds))
	// Wire clipboard so cut/copy emits OSC 52 to the host terminal.
	clipboard.SetWriter(be.WriteRaw)
	return a, nil
}

// Done shuts down the backend, restoring the terminal. Walks the view
// tree first to call Stop() on any descendant that satisfies the
// Stoppable interface (Terminal does), so child PTYs get SIGHUP
// before the terminal is restored. Idempotent — safe to call twice.
func (a *Application) Done() {
	walkStop(&a.Group)
	_ = a.backend.Close()
}

// walkStop descends the view tree depth-first / post-order, mirroring
// walkPreFlush, and calls Stop() on every view that satisfies the
// Stoppable interface. Used by Application.Done to tear down PTYs.
func walkStop(g *views.Group) {
	for _, c := range g.Children {
		if gi, ok := c.(interface{ InnerGroup() *views.Group }); ok {
			if inner := gi.InnerGroup(); inner != nil && inner != g {
				walkStop(inner)
			}
		}
		if s, ok := c.(Stoppable); ok {
			s.Stop()
		}
	}
}
