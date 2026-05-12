package terminal

import (
	"strconv"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
)

// encodeMouseSGR formats one FV mouse event as an SGR-1006 escape
// sequence: "\x1b[<b;x;y M" for press / motion, "\x1b[<b;x;y m" for
// release. b is the xterm button code (0=left, 1=middle, 2=right,
// 32 added for motion, 64+ for wheel). Coordinates are 1-based.
//
// Returns "" when the event isn't a meaningful mouse signal (e.g.,
// motion without any button held when only ?1002 is enabled — but we
// filter those out at the call site, so we never see them here).
//
// We always emit SGR-1006 format even when the inner program asked
// for the older X10 / extended encodings; modern shells universally
// understand 1006, and supporting the legacy fixed-width-byte format
// would mean failing on coords > 223 anyway. If a program insists on
// X10 (sgr1006=false), we still emit 1006 — empirically it's the
// more reliable choice.
func encodeMouseSGR(ev *drivers.Event, x, y int, _ bool) string {
	b := mouseButtonCode(ev)
	if b < 0 {
		return ""
	}
	final := byte('M')
	if ev.What == consts.EvMouseUp {
		final = 'm'
	}
	var sb [32]byte
	out := sb[:0]
	out = append(out, '\x1b', '[', '<')
	out = strconv.AppendInt(out, int64(b), 10)
	out = append(out, ';')
	out = strconv.AppendInt(out, int64(x+1), 10)
	out = append(out, ';')
	out = strconv.AppendInt(out, int64(y+1), 10)
	out = append(out, final)
	return string(out)
}

// mouseButtonCode returns the xterm button-code byte for an event, or
// -1 if no button bit is set on a press/release. Motion events use
// the bottom-most button currently considered "pressed" by FV.
func mouseButtonCode(ev *drivers.Event) int {
	// Wheel events get distinct codes regardless of motion.
	if ev.Buttons&consts.MbScrollWheelUp != 0 {
		return 64
	}
	if ev.Buttons&consts.MbScrollWheelDown != 0 {
		return 65
	}
	b := -1
	switch {
	case ev.Buttons&consts.MbLeftButton != 0:
		b = 0
	case ev.Buttons&consts.MbMiddleButton != 0:
		b = 1
	case ev.Buttons&consts.MbRightButton != 0:
		b = 2
	}
	if b < 0 && ev.What == consts.EvMouseUp {
		// Release with no specific button still reports "release of
		// whatever was held" — code 0 is the conventional default.
		b = 0
	}
	if b < 0 && ev.What == consts.EvMouseMove {
		b = 3 // "motion with no button"
	}
	if b < 0 {
		return -1
	}
	if ev.What == consts.EvMouseMove {
		b |= 32 // xterm motion bit
	}
	// Modifier bits.
	if ev.KeyShift&consts.KbLeftShift != 0 {
		b |= 4
	}
	if ev.KeyShift&consts.KbAltShift != 0 {
		b |= 8
	}
	if ev.KeyShift&consts.KbCtrlShift != 0 {
		b |= 16
	}
	return b
}

// keyToBytes translates one FV key event into the byte sequence the
// PTY child expects. Returns nil for events that shouldn't be sent
// (focus changes, mouse, …).
//
// The mapping is the standard "xterm-like" set: navigation keys go out
// as the appropriate CSI escape, ASCII passes through, and Alt+letter
// gets the ESC prefix that most modern shells expect.
func keyToBytes(ev *drivers.Event) []byte {
	if ev.What != consts.EvKeyDown {
		return nil
	}
	// Navigation / function keys via KeyCode.
	switch ev.KeyCode {
	case consts.KbEnter:
		return []byte{'\r'}
	case consts.KbTab:
		return []byte{'\t'}
	case consts.KbShiftTab:
		return []byte("\x1b[Z")
	case consts.KbBack:
		return []byte{0x7F}
	case consts.KbEsc:
		return []byte{0x1B}
	case consts.KbSpaceBar:
		// Space is emitted by the reader as a named Key (not a Rune),
		// so the UnicodeChar fall-through below doesn't fire. Without
		// this case, the space bar is silently dropped.
		return []byte{' '}
	case consts.KbUp:
		return []byte("\x1b[A")
	case consts.KbDown:
		return []byte("\x1b[B")
	case consts.KbRight:
		return []byte("\x1b[C")
	case consts.KbLeft:
		return []byte("\x1b[D")
	case consts.KbHome:
		return []byte("\x1b[H")
	case consts.KbEnd:
		return []byte("\x1b[F")
	case consts.KbPgUp:
		return []byte("\x1b[5~")
	case consts.KbPgDn:
		return []byte("\x1b[6~")
	case consts.KbDel:
		return []byte("\x1b[3~")
	case consts.KbIns:
		return []byte("\x1b[2~")
	case consts.KbF1:
		return []byte("\x1bOP")
	case consts.KbF2:
		return []byte("\x1bOQ")
	case consts.KbF3:
		return []byte("\x1bOR")
	case consts.KbF4:
		return []byte("\x1bOS")
	case consts.KbF5:
		return []byte("\x1b[15~")
	case consts.KbF6:
		return []byte("\x1b[17~")
	case consts.KbF7:
		return []byte("\x1b[18~")
	case consts.KbF8:
		return []byte("\x1b[19~")
	case consts.KbF9:
		return []byte("\x1b[20~")
	case consts.KbF10:
		return []byte("\x1b[21~")
	case consts.KbF11:
		return []byte("\x1b[23~")
	case consts.KbF12:
		return []byte("\x1b[24~")
	case consts.KbCtrlBack:
		return []byte{0x08}
	}
	// Ctrl+letter from our reader arrives via KeyCode = consts.KbCtrlA..Z;
	// map back to the raw Ctrl-modified byte.
	if c := ctrlByte(ev.KeyCode); c != 0 {
		return []byte{c}
	}
	// Alt+letter: ESC + char.
	if ev.KeyShift&consts.KbAltShift != 0 && ev.UnicodeChar >= ' ' {
		return []byte{0x1B, byte(ev.UnicodeChar)}
	}
	// Plain printable rune.
	if ev.UnicodeChar > 0 {
		return []byte(string(ev.UnicodeChar))
	}
	return nil
}

// ctrlByte returns the raw Ctrl-letter byte for KbCtrlA..KbCtrlZ, or 0
// if the keycode isn't one of those. Keeps the mapping table out of
// the main switch for readability.
func ctrlByte(kc uint16) byte {
	switch kc {
	case consts.KbCtrlA:
		return 0x01
	case consts.KbCtrlB:
		return 0x02
	case consts.KbCtrlC:
		return 0x03
	case consts.KbCtrlD:
		return 0x04
	case consts.KbCtrlE:
		return 0x05
	case consts.KbCtrlF:
		return 0x06
	case consts.KbCtrlG:
		return 0x07
	case consts.KbCtrlH:
		return 0x08
	case consts.KbCtrlI:
		return 0x09
	case consts.KbCtrlJ:
		return 0x0A
	case consts.KbCtrlK:
		return 0x0B
	case consts.KbCtrlL:
		return 0x0C
	case consts.KbCtrlM:
		return 0x0D
	case consts.KbCtrlN:
		return 0x0E
	case consts.KbCtrlO:
		return 0x0F
	case consts.KbCtrlP:
		return 0x10
	case consts.KbCtrlQ:
		return 0x11
	case consts.KbCtrlR:
		return 0x12
	case consts.KbCtrlS:
		return 0x13
	case consts.KbCtrlT:
		return 0x14
	case consts.KbCtrlU:
		return 0x15
	case consts.KbCtrlV:
		return 0x16
	case consts.KbCtrlW:
		return 0x17
	case consts.KbCtrlX:
		return 0x18
	case consts.KbCtrlY:
		return 0x19
	case consts.KbCtrlZ:
		return 0x1A
	}
	return 0
}
