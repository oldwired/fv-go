package terminal

import (
	"strconv"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/term"
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

// keyToBytes translates one FV key event into the byte sequence the PTY child
// expects. applicationCursor is the child-controlled DECCKM state.
// EffectiveKey supplies one normalized identity while preserving Event's
// legacy public record layout.
func keyToBytes(ev *drivers.Event, applicationCursor bool) []byte {
	id := ev.EffectiveKey()
	if !id.Valid {
		return nil
	}

	if id.Key == term.KeyNone {
		return encodeRune(id.Rune, id.Mods)
	}

	// These keys use C0/classic encodings. Classic input cannot
	// distinguish Ctrl-I from Tab or Ctrl-M from Enter; preserve the named
	// identity consistently and only add an Alt prefix where meaningful.
	switch id.Key {
	case term.KeyEnter:
		b := byte('\r')
		if id.Mods.Has(term.ModCtrl) {
			b = '\n'
		}
		return withAlt([]byte{b}, id.Mods)
	case term.KeyTab:
		if id.Mods.Has(term.ModShift) {
			return []byte("\x1b[Z")
		}
		return withAlt([]byte{'\t'}, id.Mods)
	case term.KeyBackspace:
		b := byte(0x7f)
		if id.Mods.Has(term.ModCtrl) {
			b = 0x08
		}
		return withAlt([]byte{b}, id.Mods)
	case term.KeyEsc:
		return withAlt([]byte{0x1b}, id.Mods)
	case term.KeySpace:
		if id.Mods.Has(term.ModCtrl) {
			return withAlt([]byte{0}, id.Mods)
		}
		return withAlt([]byte{' '}, id.Mods)
	}

	if final, ok := cursorFinal(id.Key); ok {
		if id.Mods == 0 {
			if applicationCursor {
				return []byte{0x1b, 'O', final}
			}
			return []byte{0x1b, '[', final}
		}
		return modifiedCSI("1", id.Mods, final)
	}
	if number, ok := tildeKeyNumber(id.Key); ok {
		if id.Mods == 0 {
			return []byte("\x1b[" + number + "~")
		}
		return modifiedCSI(number, id.Mods, '~')
	}
	if final, ok := ss3FunctionFinal(id.Key); ok {
		if id.Mods == 0 {
			return []byte{0x1b, 'O', final}
		}
		return modifiedCSI("1", id.Mods, final)
	}
	return nil
}

func encodeRune(r rune, mods term.ModBits) []byte {
	var out []byte
	if mods.Has(term.ModCtrl) {
		if c, ok := controlRuneByte(r); ok {
			out = []byte{c}
		}
	}
	if out == nil {
		out = []byte(string(r))
	}
	return withAlt(out, mods)
}

func controlRuneByte(r rune) (byte, bool) {
	switch {
	case r >= 0 && r <= 0x1f:
		return byte(r), true
	case r >= 'a' && r <= 'z':
		return byte(r-'a') + 1, true
	case r >= 'A' && r <= 'Z':
		return byte(r-'A') + 1, true
	}
	switch r {
	case ' ', '@':
		return 0, true
	case '[':
		return 0x1b, true
	case '\\':
		return 0x1c, true
	case ']':
		return 0x1d, true
	case '^':
		return 0x1e, true
	case '_':
		return 0x1f, true
	case '?':
		return 0x7f, true
	}
	return 0, false
}

func withAlt(payload []byte, mods term.ModBits) []byte {
	if !mods.Has(term.ModAlt) {
		return payload
	}
	out := make([]byte, 1, len(payload)+1)
	out[0] = 0x1b
	return append(out, payload...)
}

func modifierParam(mods term.ModBits) int {
	m := 1
	if mods.Has(term.ModShift) {
		m++
	}
	if mods.Has(term.ModAlt) {
		m += 2
	}
	if mods.Has(term.ModCtrl) {
		m += 4
	}
	return m
}

func modifiedCSI(first string, mods term.ModBits, final byte) []byte {
	return []byte("\x1b[" + first + ";" + strconv.Itoa(modifierParam(mods)) + string(final))
}

func cursorFinal(key term.Key) (byte, bool) {
	switch key {
	case term.KeyUp:
		return 'A', true
	case term.KeyDown:
		return 'B', true
	case term.KeyLeft:
		return 'D', true
	case term.KeyRight:
		return 'C', true
	case term.KeyHome:
		return 'H', true
	case term.KeyEnd:
		return 'F', true
	}
	return 0, false
}

func tildeKeyNumber(key term.Key) (string, bool) {
	switch key {
	case term.KeyPgUp:
		return "5", true
	case term.KeyPgDn:
		return "6", true
	case term.KeyIns:
		return "2", true
	case term.KeyDel:
		return "3", true
	case term.KeyF5:
		return "15", true
	case term.KeyF6:
		return "17", true
	case term.KeyF7:
		return "18", true
	case term.KeyF8:
		return "19", true
	case term.KeyF9:
		return "20", true
	case term.KeyF10:
		return "21", true
	case term.KeyF11:
		return "23", true
	case term.KeyF12:
		return "24", true
	}
	return "", false
}

func ss3FunctionFinal(key term.Key) (byte, bool) {
	switch key {
	case term.KeyF1:
		return 'P', true
	case term.KeyF2:
		return 'Q', true
	case term.KeyF3:
		return 'R', true
	case term.KeyF4:
		return 'S', true
	}
	return 0, false
}
