//go:build windows

package term

import (
	"errors"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/profile"
	"github.com/oldwired/fv-go/pkg/fv/sixel"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"golang.org/x/sys/windows"
)

// terminalWriter is the minimal interface winBackend.Flush needs.
// Carved out as a separate field so probe / console-API paths keep
// using *os.File / Handle directly.
type terminalWriter interface {
	WriteString(string) (int, error)
}

func newPlatformBackend() Backend { return &winBackend{} }

type winBackend struct {
	stdin     windows.Handle
	stdout    windows.Handle
	prevIn    uint32
	prevOut   uint32
	prevCP    uint32
	buf       *cellBuf
	enc       *sgrEncoder
	out       *os.File
	writer    terminalWriter // Flush write target; defaults to b.out
	in        *os.File
	cursorX   int
	cursorY   int
	cursorOn  bool
	events    chan Event
	stop      chan struct{}
	done      chan struct{} // closed by readLoop when it exits
	reader    *reader
	closeOnce sync.Once
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
	b.writer = b.out
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
	// done initialized before readLoop starts; see posix.go for rationale.
	b.done = make(chan struct{})
	b.reader = newReader(b.in)
	go b.readLoop()

	// Cell pixel size for the SIXEL pipeline, most-reliable-first. Seed
	// from the current console font, then send the CSI 16t query — a
	// reply refines (or confirms) the value. Windows Terminal (≥ v1.22)
	// supports CSI 16t and SIXEL graphics; legacy ConHost doesn't reply,
	// so the console-font seed stands.
	updateCellSizeFromConsoleFont(b.stdout)
	probeCellPixelSize(b.reader, b.out)

	return nil
}

// Close is idempotent — sync.Once guarantees a double call doesn't
// re-close channels or panic. After a partial Init failure Close cannot
// be retried; see posix.go for the same rationale.
func (b *winBackend) Close() error {
	b.closeOnce.Do(func() {
		if b.stop != nil {
			close(b.stop)
			// Leave b.stop non-nil; receivers on a closed channel are fine.
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
		// Bounded wait for the read loop. Restoring console modes
		// usually wakes any blocking ReadConsoleInput.
		if b.done != nil {
			select {
			case <-b.done:
			case <-time.After(100 * time.Millisecond):
			}
		}
	})
	return nil
}

func (b *winBackend) Size() (int, int) { return b.buf.Size() }

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

func (b *winBackend) WasInvalidated(x, y int) bool { return b.buf.wasInvalidated(x, y) }

func (b *winBackend) Flush() error {
	spans := b.buf.dirty()
	if len(spans) == 0 && b.cursorOn {
		_, err := b.writer.WriteString(cursorMove(b.cursorX, b.cursorY) + "\x1b[?25h")
		return err
	}
	var sb strings.Builder
	sb.WriteString("\x1b[?25l")
	for _, s := range spans {
		sb.WriteString(cursorMove(s.x, s.y))
		fg := types.FG(s.attr)
		bg := types.BG(s.attr)
		b.enc.transition(&sb, sgrState{fg: fg, bg: bg, fgRGB: s.fg, bgRGB: s.bg, ext: s.ext})
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
	// Only commit after a successful write; see posix.go for rationale.
	if err == nil {
		b.buf.commit()
	}
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
	defer close(b.done)
	for {
		select {
		case <-b.stop:
			return
		default:
		}
		evs, err := b.reader.nextUntil(b.stop)
		if err != nil {
			return
		}
		for _, ev := range evs {
			// detect resize via embedded resize event from parser? Windows
			// posts buffer-size events through INPUT_RECORDs that the
			// VT-input mode normally swallows. As a fallback we poll size
			// after each event burst.
			cols, rows := winGetSize(b.stdout)
			curCols, curRows := b.buf.Size()
			if cols != curCols || rows != curRows {
				// A resize may be a font-size change, which alters the
				// per-cell pixel size; refresh it from the console font.
				updateCellSizeFromConsoleFont(b.stdout)
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

// GetCurrentConsoleFontEx is not exposed by x/sys/windows, so bind it
// from kernel32 directly.
var (
	modkernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetCurrentConsoleFontEx = modkernel32.NewProc("GetCurrentConsoleFontEx")
)

// consoleFontInfoEx mirrors the Win32 CONSOLE_FONT_INFOEX. dwFontSize
// carries the character-cell size in pixels.
type consoleFontInfoEx struct {
	cbSize     uint32
	nFont      uint32
	dwFontSize windows.Coord
	fontFamily uint32
	fontWeight uint32
	faceName   [32]uint16 // LF_FACESIZE
}

// updateCellSizeFromConsoleFont derives the per-cell pixel size from the
// current console font and records it via sixel.SetCellSize. This is the
// Windows counterpart to the Unix winsize-pixel path: a fallback for when
// the terminal doesn't answer the CSI 16t query. Windows Terminal answers
// CSI 16t (and we let that override this), while legacy ConHost reports a
// usable font size here. No-op on failure, leaving the sixel default or
// env-var override to stand.
func updateCellSizeFromConsoleFont(h windows.Handle) {
	var info consoleFontInfoEx
	info.cbSize = uint32(unsafe.Sizeof(info))
	r, _, _ := procGetCurrentConsoleFontEx.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return
	}
	sixel.SetCellSize(int(info.dwFontSize.X), int(info.dwFontSize.Y))
}
