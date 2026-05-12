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
	t.EventMask = consts.EvKeyDown | consts.EvCommand
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

// HandleEvent forwards key events to the PTY as ANSI byte sequences.
// CmClose triggers Stop() so a window close button reliably tears
// down the child process instead of leaking a zombie.
func (t *Terminal) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvCommand && ev.Command == consts.CmClose {
		t.Stop()
		// Don't ClearEvent — let the parent Window close as normal.
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	if t.pty == nil {
		return
	}
	bytes := keyToBytes(ev)
	if len(bytes) == 0 {
		return
	}
	_, _ = t.pty.Write(bytes)
	t.ClearEvent(ev)
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

// Draw paints the buffer's cells into the view.
func (t *Terminal) Draw() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for y := 0; y < t.Size.Y; y++ {
		row := screen.MakeDrawBuffer(t.Size.X)
		for x := 0; x < t.Size.X; x++ {
			cl := t.buf.CellAt(x, y)
			row[x] = cl.toDrawCell(t.DefaultFG, t.DefaultBG)
			if row[x].Ch == "" {
				row[x].Ch = " "
			}
		}
		t.WriteLine(0, y, t.Size.X, 1, row)
	}
	// Cursor placement.
	if t.buf.cursorVisible {
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
