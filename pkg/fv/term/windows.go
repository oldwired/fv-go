//go:build windows

package term

import (
	"errors"
	"os"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/profile"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"golang.org/x/sys/windows"
)

func newPlatformBackend() Backend { return &winBackend{} }

type winBackend struct {
	stdin    windows.Handle
	stdout   windows.Handle
	prevIn   uint32
	prevOut  uint32
	prevCP   uint32
	buf      *cellBuf
	enc      *sgrEncoder
	out      *os.File
	in       *os.File
	cursorX  int
	cursorY  int
	cursorOn bool
	events   chan Event
	stop     chan struct{}
	reader   *reader
}

const (
	enableVTOutput = 0x0004
	enableVTInput  = 0x0200
	enableNoLineNL = 0x0008
	enableWindowIn = 0x0008
	enableMouseIn  = 0x0010
)

func (b *winBackend) Init() error {
	b.in = os.Stdin
	b.out = os.Stdout
	b.stdin = windows.Handle(b.in.Fd())
	b.stdout = windows.Handle(b.out.Fd())

	if err := windows.GetConsoleMode(b.stdout, &b.prevOut); err != nil {
		return err
	}
	if err := windows.GetConsoleMode(b.stdin, &b.prevIn); err != nil {
		return err
	}
	b.prevCP, _ = windows.GetConsoleOutputCP()

	if err := windows.SetConsoleMode(b.stdout, b.prevOut|enableVTOutput|enableNoLineNL); err != nil {
		return err
	}
	if err := windows.SetConsoleMode(b.stdin, enableVTInput|enableWindowIn|enableMouseIn); err != nil {
		_ = windows.SetConsoleMode(b.stdout, b.prevOut)
		return err
	}
	if err := windows.SetConsoleOutputCP(65001); err != nil {
		// non-fatal
	}

	cols, rows := winGetSize(b.stdout)
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

	io := []string{
		"\x1b[?1049h", "\x1b[?25l",
		"\x1b[?1000h", "\x1b[?1002h", "\x1b[?1006h",
		"\x1b[?2004h", "\x1b[?1004h",
		"\x1b]22;default\x07", // mouse cursor: OS default arrow (WT honors OSC 22)
	}
	if _, err := b.out.WriteString(strings.Join(io, "")); err != nil {
		return err
	}

	b.events = make(chan Event, 64)
	b.stop = make(chan struct{})
	b.reader = newReader(b.in)
	go b.readLoop()

	// Probe cell pixel size for the SIXEL pipeline. Windows Terminal
	// (≥ v1.22) supports CSI 16t and SIXEL graphics; legacy ConHost
	// doesn't reply and we fall back to defaults.
	probeCellPixelSize(b.reader, b.out)

	return nil
}

func (b *winBackend) Close() error {
	if b.stop != nil {
		close(b.stop)
		b.stop = nil
	}
	if b.out != nil {
		_, _ = b.out.WriteString(strings.Join([]string{
			"\x1b]22;\x07", // restore mouse cursor default
			"\x1b[?1004l", "\x1b[?2004l",
			"\x1b[?1006l", "\x1b[?1002l", "\x1b[?1000l",
			"\x1b[?25h", "\x1b[0m", "\x1b[?1049l",
		}, ""))
	}
	if b.stdout != 0 {
		_ = windows.SetConsoleMode(b.stdout, b.prevOut)
	}
	if b.stdin != 0 {
		_ = windows.SetConsoleMode(b.stdin, b.prevIn)
	}
	if b.prevCP != 0 {
		_ = windows.SetConsoleOutputCP(b.prevCP)
	}
	return nil
}

func (b *winBackend) Size() (int, int) { return b.buf.cols, b.buf.rows }

func (b *winBackend) SetCell(x, y int, c types.DrawCell) { b.buf.Set(x, y, c) }

func (b *winBackend) GetCell(x, y int) types.DrawCell { return b.buf.Get(x, y) }

func (b *winBackend) WriteRaw(s string) error {
	if b.out == nil {
		return nil
	}
	_, err := b.out.WriteString(s)
	return err
}

func (b *winBackend) Clear(attr uint16) { b.buf.Clear(attr) }

func (b *winBackend) MarkClean(x, y int) { b.buf.markClean(x, y) }

func (b *winBackend) Invalidate(x, y int) { b.buf.invalidate(x, y) }

func (b *winBackend) Flush() error {
	spans := b.buf.dirty()
	if len(spans) == 0 && b.cursorOn {
		_, err := b.out.WriteString(cursorMove(b.cursorX, b.cursorY) + "\x1b[?25h")
		return err
	}
	var sb strings.Builder
	sb.WriteString("\x1b[?25l")
	for _, s := range spans {
		sb.WriteString(cursorMove(s.x, s.y))
		fg := types.FG(s.attr)
		bg := types.BG(s.attr)
		b.enc.transition(&sb, sgrState{fg: fg, bg: bg, fgRGB: s.fg, bgRGB: s.bg, ext: s.ext})
		sb.WriteString(s.text)
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

func (b *winBackend) SetCursor(x, y int) {
	b.cursorX, b.cursorY = x, y
	b.cursorOn = x >= 0 && y >= 0
}

func (b *winBackend) ShowCursor(visible bool) { b.cursorOn = visible }

func (b *winBackend) Events() <-chan Event { return b.events }

func (b *winBackend) Suspend() error { return errors.New("suspend not supported on Windows") }

func (b *winBackend) Resume() error { return nil }

func (b *winBackend) readLoop() {
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
			// detect resize via embedded resize event from parser? Windows
			// posts buffer-size events through INPUT_RECORDs that the
			// VT-input mode normally swallows. As a fallback we poll size
			// after each event burst.
			cols, rows := winGetSize(b.stdout)
			if cols != b.buf.cols || rows != b.buf.rows {
				b.buf.Resize(cols, rows)
				select {
				case b.events <- Event{Kind: EventResize, Resize: geom.Point{X: cols, Y: rows}}:
				case <-b.stop:
					return
				}
			}
			select {
			case b.events <- ev:
			case <-b.stop:
				return
			}
		}
	}
}

func cursorMove(x, y int) string {
	var sb strings.Builder
	sb.WriteString("\x1b[")
	sb.WriteString(itoa(y + 1))
	sb.WriteString(";")
	sb.WriteString(itoa(x + 1))
	sb.WriteString("H")
	return sb.String()
}

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

func winGetSize(h windows.Handle) (cols, rows int) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(h, &info); err != nil {
		return 0, 0
	}
	return int(info.Window.Right - info.Window.Left + 1), int(info.Window.Bottom - info.Window.Top + 1)
}
