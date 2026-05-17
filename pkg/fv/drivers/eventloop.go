package drivers

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/term"
)

// FromTermEvent projects a term.Event into the legacy FV Event shape.
// Returns ev with What == EvNothing if the input is something the
// view layer doesn't care about (focus events, paste, etc.).
func FromTermEvent(t term.Event) Event {
	var e Event
	switch t.Kind {
	case term.EventKey:
		e.What = consts.EvKeyDown
		if t.Mods.Has(term.ModShift) {
			e.KeyShift |= consts.KbLeftShift
		}
		if t.Mods.Has(term.ModAlt) {
			e.KeyShift |= consts.KbAltShift
		}
		if t.Mods.Has(term.ModCtrl) {
			e.KeyShift |= consts.KbCtrlShift
		}
		if t.Key != term.KeyNone {
			e.KeyCode = mapKey(t.Key, t.Mods)
		} else {
			e.UnicodeChar = t.Rune
			if t.Mods.Has(term.ModAlt) {
				// Alt-letter -> kbAlt<X> when known
				if alt := altCodeForRune(t.Rune); alt != 0 {
					e.KeyCode = alt
				} else {
					e.KeyCode = uint16(t.Rune) & 0x00FF
				}
			} else if t.Mods.Has(term.ModCtrl) {
				e.KeyCode = ctrlCodeForRune(t.Rune)
			} else {
				e.KeyCode = uint16(t.Rune) & 0x00FF
			}
		}
	case term.EventMouse:
		// Wheel ticks arrive through the xterm protocol as a "press"
		// with a MbScrollWheel* button bit and no matching release.
		// Project them to EvMouseWheel so widgets that match
		// EvMouseDown for "user clicked" don't misfire on every
		// scroll notch. The button-bit (Up vs Down) stays on
		// e.Buttons so callers can tell scroll direction.
		switch {
		case t.Mouse.Buttons&(consts.MbScrollWheelUp|consts.MbScrollWheelDown) != 0:
			e.What = consts.EvMouseWheel
		case t.Mouse.Motion:
			// Motion-while-button-held arrives with Pressed=true and
			// the xterm "32" motion bit, so check Motion first.
			e.What = consts.EvMouseMove
		case t.Mouse.Released:
			e.What = consts.EvMouseUp
		case t.Mouse.Pressed:
			e.What = consts.EvMouseDown
		}
		e.Buttons = t.Mouse.Buttons
		e.Where = t.Mouse.Where
		e.DoubleClk = t.Mouse.Double
		// Modifier bits travel on every mouse event, so view code
		// can read Shift/Ctrl/Alt+click the same way it reads them
		// off a keyboard event.
		if t.Mods.Has(term.ModShift) {
			e.KeyShift |= consts.KbLeftShift
		}
		if t.Mods.Has(term.ModAlt) {
			e.KeyShift |= consts.KbAltShift
		}
		if t.Mods.Has(term.ModCtrl) {
			e.KeyShift |= consts.KbCtrlShift
		}
	case term.EventResize:
		e.What = consts.EvCommand
		e.Command = consts.CmResizeApp
		e.InfoPtr = t.Resize
	case term.EventFocusIn:
		e.What = consts.EvCommand
		e.Command = consts.CmReceivedFocus
	case term.EventFocusOut:
		e.What = consts.EvCommand
		e.Command = consts.CmReleasedFocus
	case term.EventPaste:
		e.What = consts.EvCommand
		e.Command = consts.CmPaste
		e.InfoPtr = t.Paste
		// Carry the truncation flag in InfoByte. Downstream readers
		// that only need the string keep doing e.InfoPtr.(string);
		// callers who care can check (e.InfoByte & consts.PasteTruncated).
		if t.Truncated {
			e.InfoByte = consts.PasteTruncated
		}
	default:
		// term.EventNone (zero value) and any future kind we haven't
		// taught the view layer to care about — return Event with
		// What == EvNothing, the documented "ignore this" sentinel.
	}
	return e
}

func mapKey(k term.Key, mods term.ModBits) uint16 {
	switch k {
	case term.KeyEnter:
		return consts.KbEnter
	case term.KeyTab:
		if mods.Has(term.ModShift) {
			return consts.KbShiftTab
		}
		return consts.KbTab
	case term.KeyBackspace:
		if mods.Has(term.ModCtrl) {
			return consts.KbCtrlBack
		}
		if mods.Has(term.ModAlt) {
			return consts.KbAltBack
		}
		return consts.KbBack
	case term.KeyEsc:
		return consts.KbEsc
	case term.KeySpace:
		return consts.KbSpaceBar
	case term.KeyUp:
		if mods.Has(term.ModCtrl) {
			return consts.KbCtrlUp
		}
		if mods.Has(term.ModAlt) {
			return consts.KbAltUp
		}
		return consts.KbUp
	case term.KeyDown:
		if mods.Has(term.ModCtrl) {
			return consts.KbCtrlDown
		}
		if mods.Has(term.ModAlt) {
			return consts.KbAltDown
		}
		return consts.KbDown
	case term.KeyLeft:
		if mods.Has(term.ModCtrl) {
			return consts.KbCtrlLeft
		}
		if mods.Has(term.ModAlt) {
			return consts.KbAltLeft
		}
		return consts.KbLeft
	case term.KeyRight:
		if mods.Has(term.ModCtrl) {
			return consts.KbCtrlRight
		}
		if mods.Has(term.ModAlt) {
			return consts.KbAltRight
		}
		return consts.KbRight
	case term.KeyHome:
		if mods.Has(term.ModCtrl) {
			return consts.KbCtrlHome
		}
		return consts.KbHome
	case term.KeyEnd:
		if mods.Has(term.ModCtrl) {
			return consts.KbCtrlEnd
		}
		return consts.KbEnd
	case term.KeyPgUp:
		if mods.Has(term.ModCtrl) {
			return consts.KbCtrlPgUp
		}
		return consts.KbPgUp
	case term.KeyPgDn:
		if mods.Has(term.ModCtrl) {
			return consts.KbCtrlPgDn
		}
		return consts.KbPgDn
	case term.KeyIns:
		return consts.KbIns
	case term.KeyDel:
		return consts.KbDel
	case term.KeyF1:
		return fkey(consts.KbF1, consts.KbShiftF1, consts.KbCtrlF1, consts.KbAltF1, mods)
	case term.KeyF2:
		return fkey(consts.KbF2, consts.KbShiftF2, consts.KbCtrlF2, consts.KbAltF2, mods)
	case term.KeyF3:
		return fkey(consts.KbF3, consts.KbShiftF3, consts.KbCtrlF3, consts.KbAltF3, mods)
	case term.KeyF4:
		return fkey(consts.KbF4, consts.KbShiftF4, consts.KbCtrlF4, consts.KbAltF4, mods)
	case term.KeyF5:
		return fkey(consts.KbF5, consts.KbShiftF5, consts.KbCtrlF5, consts.KbAltF5, mods)
	case term.KeyF6:
		return fkey(consts.KbF6, consts.KbShiftF6, consts.KbCtrlF6, consts.KbAltF6, mods)
	case term.KeyF7:
		return fkey(consts.KbF7, consts.KbShiftF7, consts.KbCtrlF7, consts.KbAltF7, mods)
	case term.KeyF8:
		return fkey(consts.KbF8, consts.KbShiftF8, consts.KbCtrlF8, consts.KbAltF8, mods)
	case term.KeyF9:
		return fkey(consts.KbF9, consts.KbShiftF9, consts.KbCtrlF9, consts.KbAltF9, mods)
	case term.KeyF10:
		return fkey(consts.KbF10, consts.KbShiftF10, consts.KbCtrlF10, consts.KbAltF10, mods)
	case term.KeyF11:
		return consts.KbF11
	case term.KeyF12:
		return consts.KbF12
	default:
		// term.KeyNone (zero value, "character key, see Rune") and
		// anything we don't have a legacy FV scancode for — fall
		// through to the bare-rune dispatch in the caller.
	}
	return 0
}

func fkey(plain, shift, ctrl, alt uint16, mods term.ModBits) uint16 {
	switch {
	case mods.Has(term.ModCtrl):
		return ctrl
	case mods.Has(term.ModAlt):
		return alt
	case mods.Has(term.ModShift):
		return shift
	}
	return plain
}

// altCodeForRune mirrors the AltCodes table in Drivers.pas — maps a
// printable letter or digit to the kbAlt<X> scan-coded value.
func altCodeForRune(r rune) uint16 {
	switch r {
	case 'a', 'A':
		return consts.KbAltA
	case 'b', 'B':
		return consts.KbAltB
	case 'c', 'C':
		return consts.KbAltC
	case 'd', 'D':
		return consts.KbAltD
	case 'e', 'E':
		return consts.KbAltE
	case 'f', 'F':
		return consts.KbAltF
	case 'g', 'G':
		return consts.KbAltG
	case 'h', 'H':
		return consts.KbAltH
	case 'i', 'I':
		return consts.KbAltI
	case 'j', 'J':
		return consts.KbAltJ
	case 'k', 'K':
		return consts.KbAltK
	case 'l', 'L':
		return consts.KbAltL
	case 'm', 'M':
		return consts.KbAltM
	case 'n', 'N':
		return consts.KbAltN
	case 'o', 'O':
		return consts.KbAltO
	case 'p', 'P':
		return consts.KbAltP
	case 'q', 'Q':
		return consts.KbAltQ
	case 'r', 'R':
		return consts.KbAltR
	case 's', 'S':
		return consts.KbAltS
	case 't', 'T':
		return consts.KbAltT
	case 'u', 'U':
		return consts.KbAltU
	case 'v', 'V':
		return consts.KbAltV
	case 'w', 'W':
		return consts.KbAltW
	case 'x', 'X':
		return consts.KbAltX
	case 'y', 'Y':
		return consts.KbAltY
	case 'z', 'Z':
		return consts.KbAltZ
	case '0':
		return consts.KbAlt0
	case '1':
		return consts.KbAlt1
	case '2':
		return consts.KbAlt2
	case '3':
		return consts.KbAlt3
	case '4':
		return consts.KbAlt4
	case '5':
		return consts.KbAlt5
	case '6':
		return consts.KbAlt6
	case '7':
		return consts.KbAlt7
	case '8':
		return consts.KbAlt8
	case '9':
		return consts.KbAlt9
	case '-':
		return consts.KbAltMinus
	case '=':
		return consts.KbAltEqual
	}
	return 0
}

func ctrlCodeForRune(r rune) uint16 {
	if r >= 'a' && r <= 'z' {
		r -= 32
	}
	switch r {
	case 'A':
		return consts.KbCtrlA
	case 'B':
		return consts.KbCtrlB
	case 'C':
		return consts.KbCtrlC
	case 'D':
		return consts.KbCtrlD
	case 'E':
		return consts.KbCtrlE
	case 'F':
		return consts.KbCtrlF
	case 'G':
		return consts.KbCtrlG
	case 'H':
		return consts.KbCtrlH
	case 'I':
		return consts.KbCtrlI
	case 'J':
		return consts.KbCtrlJ
	case 'K':
		return consts.KbCtrlK
	case 'L':
		return consts.KbCtrlL
	case 'M':
		return consts.KbCtrlM
	case 'N':
		return consts.KbCtrlN
	case 'O':
		return consts.KbCtrlO
	case 'P':
		return consts.KbCtrlP
	case 'Q':
		return consts.KbCtrlQ
	case 'R':
		return consts.KbCtrlR
	case 'S':
		return consts.KbCtrlS
	case 'T':
		return consts.KbCtrlT
	case 'U':
		return consts.KbCtrlU
	case 'V':
		return consts.KbCtrlV
	case 'W':
		return consts.KbCtrlW
	case 'X':
		return consts.KbCtrlX
	case 'Y':
		return consts.KbCtrlY
	case 'Z':
		return consts.KbCtrlZ
	}
	return uint16(r) & 0x1F
}
