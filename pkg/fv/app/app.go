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
	"github.com/oldwired/fv-go/pkg/fv/term"
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
	dirty     bool        // set when an event/anim/etc. has changed state and a redraw is owed
	waitTimer *time.Timer // reused across waitOne() calls to skip per-wake allocation

	// OnCommand, if non-nil, is invoked for every EvCommand event the
	// view tree didn't consume. Returning true marks the command as
	// handled and prevents further propagation.
	OnCommand func(cmd uint16, ev *drivers.Event) bool
}

// NewProgram constructs an empty Program. App.Init wires the backend.
func NewProgram(backend term.Backend) *Program {
	cols, rows := backend.Size()
	bounds := geom.NewRect(0, 0, cols, rows)
	p := &Program{backend: backend}
	views.InitGroup(&p.Group, bounds)
	p.SetSelf(p)
	p.GrowMode = consts.GfGrowAll
	p.queue = drivers.NewQueue()
	views.SetEventQueue(p.queue)
	views.SetPump(p.idle)
	views.SetWait(p.waitOne)
	views.SetMarkDirty(func() { p.dirty = true })
	views.SetRootBackend(backend)
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
		var emit uint16
		var info int16
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
func (p *Program) Run() {
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
			return
		}
		// Anything we just handled could have changed visible state.
		p.dirty = true
	}
}

// idle drains queued term events into the FV queue, advances any
// registered animation tickers, then redraws the view tree and flushes
// the backend. Called both by Run between events and by modal loops
// (MenuBox.Run, Group.ExecView) that need a redraw while they wait
// for input.
func (p *Program) idle() {
	pumped := p.pump()
	animDirty := anim.Pulse()
	if !p.dirty && !pumped && !animDirty {
		return
	}
	p.draw()
	_ = p.backend.Flush()
	p.dirty = false
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
// Pumping an event does NOT set dirty here — the dirty flag is set by
// the caller after it actually handles the event (Run sets it directly,
// modal loops call views.MarkDirty()). Setting it here too would cause
// a stale extra draw between waitOne returning and the event being
// dispatched.
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
			}
		case <-p.waitTimer.C:
		}
		return
	}
	te, alive := <-p.backend.Events()
	if !alive {
		return
	}
	if e := drivers.FromTermEvent(te); e.What != 0 {
		p.queue.Put(e)
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

// Done shuts down the backend, restoring the terminal.
func (a *Application) Done() {
	_ = a.backend.Close()
}
