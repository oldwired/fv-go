//go:build unix

package term

import (
	"errors"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/profile"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"golang.org/x/sys/unix"
	xterm "golang.org/x/term"
)

// terminalWriter is the minimal interface posixBackend.Flush needs.
// Carved out as a separate field (rather than re-typing posixBackend.out)
// so probe / raw-mode paths keep their *os.File for Fd() access.
type terminalWriter interface {
	WriteString(string) (int, error)
}

func newPlatformBackend() Backend { return &posixBackend{} }

type posixBackend struct {
	in       *os.File
	out      *os.File
	writer   terminalWriter // Flush write target; defaults to b.out
	prevTerm *xterm.State
	buf      *cellBuf
	enc      *sgrEncoder
	cursorX  int
	cursorY  int
	cursorOn bool
	events   chan Event
	stop     chan struct{}
	done     chan struct{} // closed by readLoop when it exits
	winch    chan os.Signal
	reader   *reader

	// closeMu guards the closed flag. Close uses it for idempotency
	// (multiple Close calls only run teardown once); Init resets the
	// flag so Suspend → Resume → … → Close still tears down properly.
	// Replaces an earlier sync.Once-based design that left
	// post-Suspend Close calls as no-ops, leaving raw mode / alt
	// screen / mouse modes potentially active on shutdown.
	closeMu sync.Mutex
	closed  bool
}

func (b *posixBackend) Init() error {
	// Re-arm the closed flag so a Suspend → Resume cycle leaves a
	// future Close able to actually run its teardown. Without this,
	// the post-Suspend session would leak raw mode + alt screen on
	// process exit.
	b.closeMu.Lock()
	b.closed = false
	b.closeMu.Unlock()

	b.in = os.Stdin
	b.out = os.Stdout
	b.writer = b.out

	if !xterm.IsTerminal(int(b.in.Fd())) {
		return errors.New("term: stdin is not a tty")
	}
	st, err := xterm.MakeRaw(int(b.in.Fd()))
	if err != nil {
		return err
	}
	b.prevTerm = st

	cols, rows := getSize(int(b.out.Fd()))
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	b.buf = newCellBuf(cols, rows)

	profile.SetVTProbe(true)
	prof := profile.Get()
	b.enc = newSGREncoder(prof.ColorSystem)

	// Switch to alt screen, hide cursor, enable mouse + paste, save originals.
	//
	// OSC 22 ; default ST overrides the host terminal's default mouse-
	// cursor shape (typically I-beam, since terminals are text-first)
	// with the OS default — usually the arrow on macOS / Windows /
	// most Linux DEs. iTerm2, WezTerm, kitty, and Windows Terminal
	// (recent builds) all honor this. Older terminals silently ignore
	// the OSC, which is fine — they keep showing the I-beam.
	io := []string{
		"\x1b[?1049h",         // alt screen
		"\x1b[?25l",           // hide cursor
		"\x1b[?1000h",         // X10 mouse: button down/up
		"\x1b[?1002h",         // cell-motion mouse: motion while a button is held
		"\x1b[?1006h",         // SGR-1006 extended-coordinates encoding
		"\x1b[?2004h",         // bracketed paste
		"\x1b[?1004h",         // focus events
		"\x1b]22;default\x07", // mouse cursor: OS default (arrow)
	}
	if _, err := b.out.WriteString(strings.Join(io, "")); err != nil {
		_ = xterm.Restore(int(b.in.Fd()), b.prevTerm)
		return err
	}

	b.cursorOn = false
	b.events = make(chan Event, 64)
	b.stop = make(chan struct{})
	// done is initialized here, in Init, before the readLoop starts —
	// so Close()'s wait-for-exit branch can always select on it without
	// a nil-channel guard.
	b.done = make(chan struct{})
	b.winch = make(chan os.Signal, 1)
	signal.Notify(b.winch, syscall.SIGWINCH)
	b.reader = newReader(b.in)

	go b.readLoop()
	go b.signalLoop()

	// Wire the cell-pixel-size probe through the reader's CSI parser.
	// Send the CSI 16t query, give the reader up to 200ms to forward a
	// response via OnCellSize, then proceed regardless. Terminals that
	// don't support the query (macOS Terminal, legacy ConHost, dumb
	// tty, …) just don't reply and we fall back to the env-var
	// override or sixel package default.
	probeCellPixelSize(b.reader, b.out)

	return nil
}

// Close is idempotent — sync.Once guarantees a double call doesn't
// re-close channels or panic. After a partial Init failure Close cannot
// be retried; re-initializing from a half-set-up tty is unsafe, so the
// once-and-done semantics are what we want.
//
// Close stops the read loop, signals it via the stop channel, and waits
// briefly for the reader goroutine to exit via the done channel. The
// blocking read inside the goroutine cannot be canceled directly;
// restoring cooked mode typically unblocks it within milliseconds.
// Worst case the wait times out and the goroutine exits after the next
// byte arrives — by which point the terminal is back to a sane state
// and the leftover goroutine is harmless.
func (b *posixBackend) Close() error {
	b.closeMu.Lock()
	if b.closed {
		b.closeMu.Unlock()
		return nil
	}
	b.closed = true
	b.closeMu.Unlock()

	if b.stop != nil {
		close(b.stop)
		// Leave b.stop non-nil. Reading from a closed channel returns
		// immediately, which is exactly what consumers want.
	}
	if b.winch != nil {
		signal.Stop(b.winch)
		b.winch = nil
	}
	// restore terminal state (best effort, in order)
	_, _ = b.out.WriteString(strings.Join([]string{
		"\x1b]22;\x07", // mouse cursor: clear override (restores terminal default)
		"\x1b[?1004l",  // focus events off
		"\x1b[?2004l",  // bracketed paste off
		"\x1b[?1006l",
		"\x1b[?1002l", // cell-motion off
		"\x1b[?1000l",
		"\x1b[?25h",   // show cursor
		"\x1b[0m",     // SGR reset
		"\x1b[?1049l", // exit alt screen
	}, ""))
	if b.prevTerm != nil && b.in != nil {
		_ = xterm.Restore(int(b.in.Fd()), b.prevTerm)
		b.prevTerm = nil
	}
	// Best-effort wait for the read loop to finish. Restoring cooked
	// mode usually unblocks the pending Read promptly.
	if b.done != nil {
		select {
		case <-b.done:
		case <-time.After(100 * time.Millisecond):
		}
	}
	return nil
}

func (b *posixBackend) Size() (cols, rows int) {
	return b.buf.Size()
}

func (b *posixBackend) SetCell(x, y int, c types.DrawCell) { b.buf.Set(x, y, c) }

func (b *posixBackend) GetCell(x, y int) types.DrawCell { return b.buf.Get(x, y) }

func (b *posixBackend) WriteRaw(s string) error {
	if b.out == nil {
		return nil
	}
	_, err := b.out.WriteString(s)
	return err
}

func (b *posixBackend) Clear(attr uint16) { b.buf.Clear(attr) }

func (b *posixBackend) MarkClean(x, y int) { b.buf.markClean(x, y) }

func (b *posixBackend) Invalidate(x, y int) { b.buf.invalidate(x, y) }

func (b *posixBackend) Flush() error {
	spans := b.buf.dirty()
	if len(spans) == 0 && b.cursorOn {
		// at minimum reposition cursor if it should be on
		_, err := b.writer.WriteString(cursorMove(b.cursorX, b.cursorY) + "\x1b[?25h")
		return err
	}

	var sb strings.Builder
	// Hide cursor while painting.
	sb.WriteString("\x1b[?25l")
	for _, s := range spans {
		sb.WriteString(cursorMove(s.x, s.y))
		fg := types.FG(s.attr)
		bg := types.BG(s.attr)
		b.enc.transition(&sb, sgrState{
			fg: fg, bg: bg, fgRGB: s.fg, bgRGB: s.bg, ext: s.ext,
		})
		if s.url != "" {
			sb.WriteString("\x1b]8;;")
			sb.WriteString(s.url)
			sb.WriteString("\x1b\\")
		}
		sb.WriteString(s.text)
		if s.url != "" {
			sb.WriteString("\x1b]8;;\x1b\\")
		}
	}
	sb.WriteString("\x1b[0m")
	if b.cursorOn {
		sb.WriteString(cursorMove(b.cursorX, b.cursorY))
		sb.WriteString("\x1b[?25h")
	}
	_, err := b.writer.WriteString(sb.String())
	// Only commit the front buffer after a successful write. If the
	// write failed or was partial, the diff for the next Flush still
	// reflects what the user actually sees on screen.
	if err == nil {
		b.buf.commit()
	}
	b.enc.hasLast = false
	return err
}

func (b *posixBackend) SetCursor(x, y int) {
	b.cursorX, b.cursorY = x, y
	b.cursorOn = x >= 0 && y >= 0
}

func (b *posixBackend) ShowCursor(visible bool) { b.cursorOn = visible }

func (b *posixBackend) Events() <-chan Event { return b.events }

func (b *posixBackend) Suspend() error {
	_ = b.Close()
	_ = syscall.Kill(0, syscall.SIGTSTP)
	return b.Init()
}

func (b *posixBackend) Resume() error { return nil }

func (b *posixBackend) signalLoop() {
	for {
		select {
		case <-b.stop:
			return
		case <-b.winch:
			cols, rows := getSize(int(b.out.Fd()))
			if cols < 1 || rows < 1 {
				continue
			}
			b.buf.Resize(cols, rows)
			// Wipe the terminal so any cells from the previous size
			// disappear before the next idle redraw fills the whole
			// new viewport. Without this, stale cells from rows /
			// columns that no longer exist linger during a drag.
			_, _ = b.out.WriteString("\x1b[2J")
			select {
			case b.events <- Event{Kind: EventResize, Resize: geom.Point{X: cols, Y: rows}}:
			case <-b.stop:
				return
			}
		}
	}
}

func (b *posixBackend) readLoop() {
	defer close(b.done)
	for {
		select {
		case <-b.stop:
			return
		default:
		}
		evs, err := b.reader.Next()
		if err != nil {
			return
		}
		for _, ev := range evs {
			select {
			case b.events <- ev:
			case <-b.stop:
				return
			}
		}
	}
}

func cursorMove(x, y int) string {
	// ANSI is 1-based.
	var sb strings.Builder
	sb.WriteString("\x1b[")
	sb.WriteString(itoa(y + 1))
	sb.WriteString(";")
	sb.WriteString(itoa(x + 1))
	sb.WriteString("H")
	return sb.String()
}

// itoa is a small dependency-free int->string for hot loops.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func getSize(fd int) (cols, rows int) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0
	}
	return int(ws.Col), int(ws.Row)
}
