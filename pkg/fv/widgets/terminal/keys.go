package terminal

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
)

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
