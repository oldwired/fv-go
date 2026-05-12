package terminal

import (
	"os"
	"sync"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Terminal is the FV view. Spawns a child via PTY, owns a parser +
// buffer, and handles I/O on its own goroutine.
//
// Lifecycle: Run() is the typical entry point — it starts the child
// and blocks until exit. For non-blocking use, the caller can invoke
// Start() / Stop() directly; the view will keep painting until Stop()
// is called or the child exits.
type Terminal struct {
	views.Base

	DefaultFG, DefaultBG byte

	// Title is the latest OSC 0/1/2 string the child sent. Empty
	// until the child sets one.
	Title string

	// OnTitle, if non-nil, fires from the reader goroutine whenever
	// Title changes — typically used to update the host window's
	// caption. Goroutine-safe: callbacks should not block.
	OnTitle func(string)

	// OnExit fires once when the child process exits (also from the
	// reader goroutine). Used to auto-close the wrapping window.
	OnExit func(error)

	buf    *buffer
	par    *parser
	pty    *ptyHandle
	mu     sync.Mutex
	closed bool
}

// New constructs a Terminal view at bounds. Buffer size matches the
// view's cell dimensions.
func New(bounds geom.Rect) *Terminal {
	t := &Terminal{
		Base:      views.NewBase(bounds),
		DefaultFG: 0x07,
		DefaultBG: 0x00,
	}
	t.SetSelf(t)
	t.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	t.Options |= consts.OfSelectable
	t.EventMask = consts.EvKeyDown | consts.EvCommand | consts.EvMouseDown | consts.EvMouseUp | consts.EvMouseMove
	// Claim raw keyboard so Program.HandleEvent doesn't fold Ctrl+C
	// into Copy etc. while we're focused. Ctrl+C, Ctrl+X, Ctrl+V,
	// and the F-keys reach the inner shell instead.
	t.State |= consts.SfRawKeys
	t.buf = newBuffer(t.Size.X, t.Size.Y)
	t.par = newParser(t.buf)
	// IMPORTANT: OnTitle fires from inside par.Feed, which is always
	// called with t.mu already held (either by readLoop or by Write).
	// Acquiring t.mu here would self-deadlock — when we tried, zsh
	// hung in tcsetattr (TCSADRAIN waiting on a PTY buffer the reader
	// could no longer drain) and the whole UI froze. Title is set
	// under the caller's lock, no further sync needed.
	t.par.OnTitle = func(title string) {
		t.Title = title
		if t.OnTitle != nil {
			t.OnTitle(title)
		}
		views.MarkDirty()
	}
	return t
}

// GetTypeID for serial registry.
func (t *Terminal) GetTypeID() string { return "terminal" }

// Start spawns the child process. The reader goroutine begins
// immediately; the view will start drawing the first output as soon
// as the child writes anything.
//
// If env is nil, the current process environment is used (with TERM
// patched to "xterm-256color" so curses-based programs negotiate
// reasonable capabilities).
func (t *Terminal) Start(name string, args []string, env []string) error {
	if env == nil {
		env = append(env, os.Environ()...)
		env = append(env, "TERM=xterm-256color")
	}
	p, err := startPTY(name, args, env, t.Size.X, t.Size.Y)
	if err != nil {
		return err
	}
	t.pty = p
	go t.readLoop()
	go t.waitLoop()
	return nil
}

// Stop kills the child and tears down the PTY. Safe to call multiple
// times.
func (t *Terminal) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	if t.pty != nil {
		_ = t.pty.Close()
	}
}

// PID returns the child process ID, or 0 if the PTY isn't running.
// Useful for diagnostic banners ("launched X as PID Y") so a silent
// child can be confirmed alive (or not) from another shell.
func (t *Terminal) PID() int {
	if t.pty == nil || t.pty.cmd == nil || t.pty.cmd.Process == nil {
		return 0
	}
	return t.pty.cmd.Process.Pid
}

// readLoop pumps PTY output into the parser. Runs until the PTY
// closes (typically because the child exited). On exit / error we
// inject a visible "[pty closed: <err>]" line so a silent child
// process doesn't look like a hung terminal.
func (t *Terminal) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := t.pty.Read(buf)
		if n > 0 {
			t.mu.Lock()
			t.par.Feed(buf[:n])
			t.mu.Unlock()
			views.MarkDirty()
		}
		if err != nil {
			msg := "\r\n\x1b[31m[pty closed"
			if err.Error() != "EOF" {
				msg += ": " + err.Error()
			}
			msg += "]\x1b[0m\r\n"
			t.mu.Lock()
			t.par.Feed([]byte(msg))
			t.mu.Unlock()
			views.MarkDirty()
			return
		}
	}
}

// waitLoop watches for child-process exit so we can fire OnExit.
func (t *Terminal) waitLoop() {
	err := t.pty.Wait()
	if t.OnExit != nil {
		t.OnExit(err)
	}
	views.MarkDirty()
}

// HandleEvent dispatches FV events:
//
//   - CmClose triggers Stop() so a window close button reliably tears
//     down the child process instead of leaking a zombie.
//   - Shift+PageUp / Shift+PageDn / Home / End scroll the scrollback
//     view. Plain PageUp/Dn go to the PTY (apps like less expect them).
//   - Mouse wheel scrolls the scrollback when there's history to show;
//     otherwise the wheel falls through to mouse forwarding.
//   - Other key events translate to ANSI bytes and write to the PTY.
//     Any keystroke snaps the viewport back to the live cursor.
//   - Mouse events forward to the PTY as SGR-1006 sequences, but only
//     when the inner program has enabled a mouse-tracking DEC mode.
func (t *Terminal) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvCommand && ev.Command == consts.CmClose {
		t.Stop()
		return
	}
	if t.pty == nil {
		return
	}
	switch ev.What {
	case consts.EvKeyDown:
		t.handleKey(ev)
	case consts.EvMouseDown, consts.EvMouseUp, consts.EvMouseMove:
		t.handleMouse(ev)
	}
}

// handleKey is the keyboard branch of HandleEvent.
//
// Scrollback navigation uses Shift-modified nav keys: Shift+PageUp /
// Shift+PageDn / Shift+Home / Shift+End. Plain nav keys (incl. mouse
// wheel handled separately) are forwarded to the PTY because many TUIs
// genuinely consume them.
func (t *Terminal) handleKey(ev *drivers.Event) {
	shift := ev.KeyShift&consts.KbLeftShift != 0
	if shift {
		switch ev.KeyCode {
		case consts.KbPgUp:
			t.scrollBy(t.Size.Y / 2)
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		case consts.KbPgDn:
			t.scrollBy(-t.Size.Y / 2)
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		case consts.KbHome:
			t.mu.Lock()
			t.buf.scrollOffset = len(t.buf.scrollback)
			t.mu.Unlock()
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		case consts.KbEnd:
			t.mu.Lock()
			t.buf.snapToBottom()
			t.mu.Unlock()
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		}
	}
	// Live-typing event: snap the viewport back so the cursor is
	// visible while the user is typing.
	t.mu.Lock()
	t.buf.snapToBottom()
	t.mu.Unlock()
	bytes := keyToBytes(ev)
	if len(bytes) == 0 {
		return
	}
	_, _ = t.pty.Write(bytes)
	t.ClearEvent(ev)
}

// handleMouse routes mouse events: wheel-up/down scrolls the
// scrollback while there's history to view (so the user can mouse-
// wheel through past output), and any other mouse activity gets
// forwarded to the PTY if the inner program has enabled mouse
// tracking.
func (t *Terminal) handleMouse(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		if ev.Buttons&consts.MbScrollWheelUp != 0 {
			t.scrollBy(3)
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		}
		if ev.Buttons&consts.MbScrollWheelDown != 0 {
			t.mu.Lock()
			atBottom := t.buf.scrollOffset == 0
			t.mu.Unlock()
			if !atBottom {
				t.scrollBy(-3)
				views.MarkDirty()
				t.ClearEvent(ev)
				return
			}
			// At bottom already — let the wheel pass through as a
			// mouse event so apps like less / vim can use it.
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.buf.mouseX10 && !t.buf.mouseBtnEv && !t.buf.mouseAnyEv {
		return
	}
	// Drop motion events the inner app didn't ask for.
	if ev.What == consts.EvMouseMove && !t.buf.mouseBtnEv && !t.buf.mouseAnyEv {
		return
	}
	local := t.MakeLocal(ev.Where)
	if local.X < 0 || local.Y < 0 || local.X >= t.Size.X || local.Y >= t.Size.Y {
		return
	}
	seq := encodeMouseSGR(ev, local.X, local.Y, t.buf.mouseSGR)
	if seq == "" {
		return
	}
	_, _ = t.pty.Write([]byte(seq))
	t.ClearEvent(ev)
}

// scrollBy delegates to buffer under the lock and marks dirty.
func (t *Terminal) scrollBy(delta int) {
	t.mu.Lock()
	t.buf.scrollByLines(delta)
	t.mu.Unlock()
}

// ChangeBounds rezises the buffer + the underlying PTY.
func (t *Terminal) ChangeBounds(r geom.Rect) {
	t.SetBounds(r)
	t.mu.Lock()
	t.buf.resize(t.Size.X, t.Size.Y)
	t.mu.Unlock()
	if t.pty != nil {
		_ = t.pty.Resize(t.Size.X, t.Size.Y)
	}
}

// Draw paints either the live buffer or a scrollback view, depending
// on the buffer's scrollOffset. When viewing scrollback the cursor is
// hidden — its position is in the live state and showing it inside
// historical content would be misleading.
func (t *Terminal) Draw() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for y := 0; y < t.Size.Y; y++ {
		row := screen.MakeDrawBuffer(t.Size.X)
		src := t.buf.rowAt(y)
		for x := 0; x < t.Size.X; x++ {
			var cl cell
			if x < len(src) {
				cl = src[x]
			} else {
				cl = blankCell()
			}
			row[x] = cl.toDrawCell(t.DefaultFG, t.DefaultBG)
			if row[x].Ch == "" {
				row[x].Ch = " "
			}
		}
		t.WriteLine(0, y, t.Size.X, 1, row)
	}
	if t.buf.cursorVisible && t.buf.scrollOffset == 0 {
		t.Cursor = geom.Point{X: t.buf.cursorC, Y: t.buf.cursorR}
		t.State |= consts.SfCursorVis
	} else {
		t.State &^= consts.SfCursorVis
	}
}

// Write injects bytes into the parser as if they came from the PTY.
// Useful for tests and for replaying a captured session — not used by
// the live view.
func (t *Terminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.par.Feed(p)
	return len(p), nil
}

// Buffer returns the underlying buffer for tests/inspection. Not
// goroutine-safe: callers should hold no lock and assume the buffer
// can mutate from the read loop concurrently.
func (t *Terminal) Buffer() *buffer { return t.buf }

// Ensure DrawCell satisfies the basic Ch-set assumption.
var _ = types.DrawCell{}
