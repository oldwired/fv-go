// Package term is the hand-rolled cross-platform terminal backend for
// fv-go. It owns the tty: raw mode, alt-screen entry/exit, cursor
// visibility, mouse tracking, bracketed paste, the diff-based cell
// flush, and the input event loop.
//
// Two platform-specific files set up the tty:
//   - tty_unix.go    (//go:build unix)    termios + TIOCGWINSZ + SIGWINCH
//   - tty_windows.go (//go:build windows) console-mode + buffer events
//
// Higher layers (drivers, screen, views) talk only to the Backend
// interface defined here, never directly to platform code.
package term

import (
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/types"
)

// Event is a single input event coming up from the tty reader.
type Event struct {
	Kind   EventKind
	Rune   rune
	Key    Key // for KeyEvent — high-level key identity
	Mods   ModBits
	Mouse  MouseState
	Resize geom.Point // Cols/Rows (X=cols, Y=rows)
	Paste  string     // for PasteEvent

	// Truncated, when Kind is EventPaste, signals that the paste
	// payload was cut off at the bracketed-paste size cap. Hosts
	// that care can surface this; most can ignore it.
	Truncated bool
}

// EventKind discriminates the Event union.
type EventKind int

const (
	EventNone EventKind = iota
	EventKey
	EventMouse
	EventResize
	EventPaste
	EventFocusIn
	EventFocusOut
)

// Key identifies a non-character key. Character input uses Rune+Mods
// instead with Key=KeyNone.
type Key int

const (
	KeyNone Key = iota
	KeyEnter
	KeyTab
	KeyBackspace
	KeyEsc
	KeySpace
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPgUp
	KeyPgDn
	KeyIns
	KeyDel
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
)

// ModBits is a bitmask of held modifiers when the key was pressed.
type ModBits uint8

const (
	ModShift ModBits = 1 << iota
	ModAlt
	ModCtrl
)

// Has reports whether m contains the modifier flag.
func (m ModBits) Has(flag ModBits) bool { return m&flag != 0 }

// MouseState captures one mouse update.
type MouseState struct {
	Where    geom.Point // cell coords, 0-based
	Buttons  byte       // OR of Mb* flags from consts
	Pressed  bool
	Released bool
	Motion   bool
	// Double is true when this press follows a press of the same
	// button at the same cell within the double-click window. Set
	// only on the second press of a pair.
	Double bool
}

// Backend is the contract between the term layer and the rest of fv-go.
//
// Lifecycle: Init()/Close() bracket the entire app session. After Init,
// the implementation owns stdin/stdout — callers must not read or
// write them directly.
type Backend interface {
	Init() error
	Close() error

	// Size returns the current viewport (cols, rows).
	Size() (cols, rows int)

	// SetCell updates one cell in the back buffer. Coordinates outside
	// the viewport are silently dropped.
	SetCell(x, y int, c types.DrawCell)

	// GetCell reads one cell from the back buffer. Returns the zero
	// DrawCell for out-of-range coordinates. Used by the shadow render
	// path so it can preserve the underlying character.
	GetCell(x, y int) types.DrawCell

	// Clear fills the back buffer with empty cells using attr.
	Clear(attr uint16)

	// Flush diffs the back buffer against the front buffer and emits
	// the minimum SGR + cursor moves to bring the screen into sync.
	Flush() error

	// SetCursor moves the visible cursor. Negative coordinates hide it.
	SetCursor(x, y int)

	// WriteRaw emits an arbitrary byte sequence to the terminal.
	// Used by the clipboard package to send OSC 52 sequences that
	// populate the host clipboard when supported.
	WriteRaw(s string) error

	// MarkClean tells the diff that cell (x,y) didn't change since the
	// last flush, even if cur and prev compare unequal. Used by the
	// SIXEL pre-flush hook to suppress emission of sentinel cells —
	// without this they'd render as spaces and overwrite the graphics.
	MarkClean(x, y int)

	// Invalidate forces cell (x,y) to re-emit on the next flush even if
	// cur and prev are equal. Used by SIXEL views to keep covering
	// cells layered on top of their freshly-emitted graphics, and by
	// view tear-down to clear residual SIXEL pixels at deleted regions.
	Invalidate(x, y int)

	// WasInvalidated reports whether cell (x,y)'s prev is the zero cell —
	// i.e. it was Invalidate()d (or zeroed by a resize) and not yet
	// re-committed. SIXEL pre-flush uses it to decide whether its region
	// was disturbed since the last frame and must be re-emitted.
	WasInvalidated(x, y int) bool

	// ShowCursor toggles cursor visibility independent of position.
	ShowCursor(visible bool)

	// Events returns a receive-only channel of input events.
	Events() <-chan Event

	// Suspend/Resume restore the terminal for a foreground process
	// (Ctrl+Z on Unix). No-op on Windows.
	Suspend() error
	Resume() error
}

// New returns a new platform-appropriate Backend.
func New() Backend { return newPlatformBackend() }
