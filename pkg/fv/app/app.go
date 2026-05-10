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

	backend term.Backend
	queue   *drivers.Queue
	quit    bool

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
	}
}

// idle drains queued term events into the FV queue, advances any
// registered animation tickers, then redraws the view tree and flushes
// the backend. Called both by Run between events and by modal loops
// (MenuBox.Run, Group.ExecView) that need a redraw while they wait
// for input.
func (p *Program) idle() {
	p.pump()
	anim.Pulse()
	p.draw()
	_ = p.backend.Flush()
}

// pump drains term events into the FV queue without blocking.
func (p *Program) pump() {
	for {
		select {
		case te := <-p.backend.Events():
			if e := drivers.FromTermEvent(te); e.What != 0 {
				p.queue.Put(e)
			}
		default:
			return
		}
	}
}

// waitOne blocks until at least one term event is read and pushed onto
// the FV queue, or until the next animation pulse is due. Used by
// ExecView / MenuBox.runIn when their queue is empty between input
// events. With no animations registered the wait is unbounded; with
// at least one ticker it caps at the smallest interval so animations
// stay responsive while idle.
func (p *Program) waitOne() {
	if d := anim.MinInterval(); d > 0 {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case te, alive := <-p.backend.Events():
			if !alive {
				return
			}
			if e := drivers.FromTermEvent(te); e.What != 0 {
				p.queue.Put(e)
			}
		case <-timer.C:
			// Fall through; idle() will fire anim.Pulse on the next pass.
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
