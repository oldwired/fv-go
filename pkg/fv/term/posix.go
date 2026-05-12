//go:build unix

package term

import (
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/profile"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"golang.org/x/sys/unix"
	xterm "golang.org/x/term"
)

func newPlatformBackend() Backend { return &posixBackend{} }

type posixBackend struct {
	in       *os.File
	out      *os.File
	prevTerm *xterm.State
	buf      *cellBuf
	enc      *sgrEncoder
	cursorX  int
	cursorY  int
	cursorOn bool
	events   chan Event
	stop     chan struct{}
	winch    chan os.Signal
	reader   *reader
}

func (b *posixBackend) Init() error {
	b.in = os.Stdin
	b.out = os.Stdout

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

func (b *posixBackend) Close() error {
	if b.stop != nil {
		close(b.stop)
		b.stop = nil
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
	return nil
}

func (b *posixBackend) Size() (cols, rows int) {
	return b.buf.cols, b.buf.rows
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
		_, err := b.out.WriteString(cursorMove(b.cursorX, b.cursorY) + "\x1b[?25h")
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
	_, err := b.out.WriteString(sb.String())
	b.buf.commit()
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
