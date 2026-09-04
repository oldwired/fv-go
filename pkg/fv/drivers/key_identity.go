package drivers

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/term"
)

// KeyIdentity is the normalized, lossless interpretation of a keyboard event.
// Valid is necessary because the zero values are meaningful: Ctrl-Space is
// represented by Rune == 0 and Mods containing term.ModCtrl.
type KeyIdentity struct {
	Key   term.Key
	Rune  rune
	Mods  term.ModBits
	Valid bool
}

// EffectiveKey returns a normalized base key or rune plus independent
// modifiers. It reconstructs the identity from the additive combination of
// Turbo Vision KeyCode, the full UnicodeChar, and every KeyShift bit. Keeping
// normalization in a helper avoids changing Event's public record layout, so
// even positional composite literals used by existing consumers still build.
func (e *Event) EffectiveKey() KeyIdentity {
	if e == nil || e.What != consts.EvKeyDown {
		return KeyIdentity{}
	}

	id := KeyIdentity{Rune: e.UnicodeChar, Mods: modsFromKeyShift(e.KeyShift)}
	setKey := func(key term.Key, implied term.ModBits) KeyIdentity {
		id.Key = key
		id.Mods |= implied
		id.Valid = true
		return id
	}

	switch e.KeyCode {
	case consts.KbEnter:
		return setKey(term.KeyEnter, 0)
	case consts.KbCtrlEnter:
		return setKey(term.KeyEnter, term.ModCtrl)
	case consts.KbTab:
		return setKey(term.KeyTab, 0)
	case consts.KbShiftTab:
		return setKey(term.KeyTab, term.ModShift)
	case consts.KbCtrlTab:
		return setKey(term.KeyTab, term.ModCtrl)
	case consts.KbAltTab:
		return setKey(term.KeyTab, term.ModAlt)
	case consts.KbBack:
		return setKey(term.KeyBackspace, 0)
	case consts.KbCtrlBack:
		return setKey(term.KeyBackspace, term.ModCtrl)
	case consts.KbAltBack:
		return setKey(term.KeyBackspace, term.ModAlt)
	case consts.KbAltShiftBack:
		return setKey(term.KeyBackspace, term.ModAlt|term.ModShift)
	case consts.KbEsc:
		return setKey(term.KeyEsc, 0)
	case consts.KbAltEsc:
		return setKey(term.KeyEsc, term.ModAlt)
	case consts.KbSpaceBar:
		return setKey(term.KeySpace, 0)
	case consts.KbAltSpace:
		return setKey(term.KeySpace, term.ModAlt)
	case consts.KbHome:
		return setKey(term.KeyHome, 0)
	case consts.KbCtrlHome:
		return setKey(term.KeyHome, term.ModCtrl)
	case consts.KbAltHome:
		return setKey(term.KeyHome, term.ModAlt)
	case consts.KbEnd:
		return setKey(term.KeyEnd, 0)
	case consts.KbCtrlEnd:
		return setKey(term.KeyEnd, term.ModCtrl)
	case consts.KbAltEnd:
		return setKey(term.KeyEnd, term.ModAlt)
	case consts.KbPgUp:
		return setKey(term.KeyPgUp, 0)
	case consts.KbCtrlPgUp:
		return setKey(term.KeyPgUp, term.ModCtrl)
	case consts.KbAltPgUp:
		return setKey(term.KeyPgUp, term.ModAlt)
	case consts.KbPgDn:
		return setKey(term.KeyPgDn, 0)
	case consts.KbCtrlPgDn:
		return setKey(term.KeyPgDn, term.ModCtrl)
	case consts.KbAltPgDn:
		return setKey(term.KeyPgDn, term.ModAlt)
	case consts.KbIns:
		return setKey(term.KeyIns, 0)
	case consts.KbCtrlIns:
		return setKey(term.KeyIns, term.ModCtrl)
	case consts.KbAltIns:
		return setKey(term.KeyIns, term.ModAlt)
	case consts.KbShiftIns:
		return setKey(term.KeyIns, term.ModShift)
	case consts.KbDel:
		return setKey(term.KeyDel, 0)
	case consts.KbCtrlDel:
		return setKey(term.KeyDel, term.ModCtrl)
	case consts.KbAltDel:
		return setKey(term.KeyDel, term.ModAlt)
	case consts.KbShiftDel:
		return setKey(term.KeyDel, term.ModShift)
	case consts.KbUp:
		return setKey(term.KeyUp, 0)
	case consts.KbCtrlUp:
		return setKey(term.KeyUp, term.ModCtrl)
	case consts.KbAltUp:
		return setKey(term.KeyUp, term.ModAlt)
	case consts.KbDown:
		return setKey(term.KeyDown, 0)
	case consts.KbCtrlDown:
		return setKey(term.KeyDown, term.ModCtrl)
	case consts.KbAltDown:
		return setKey(term.KeyDown, term.ModAlt)
	case consts.KbLeft:
		return setKey(term.KeyLeft, 0)
	case consts.KbCtrlLeft:
		return setKey(term.KeyLeft, term.ModCtrl)
	case consts.KbAltLeft:
		return setKey(term.KeyLeft, term.ModAlt)
	case consts.KbRight:
		return setKey(term.KeyRight, 0)
	case consts.KbCtrlRight:
		return setKey(term.KeyRight, term.ModCtrl)
	case consts.KbAltRight:
		return setKey(term.KeyRight, term.ModAlt)
	case consts.KbF11:
		return setKey(term.KeyF11, 0)
	case consts.KbF12:
		return setKey(term.KeyF12, 0)
	}

	// The F1-F10 families are contiguous within each legacy modifier
	// bank, so normalize them without another per-key switch.
	if e.KeyCode >= consts.KbF1 && e.KeyCode <= consts.KbF10 {
		return setKey(term.KeyF1+term.Key(e.KeyCode-consts.KbF1)/0x100, 0)
	}
	if e.KeyCode >= consts.KbShiftF1 && e.KeyCode <= consts.KbShiftF10 {
		return setKey(term.KeyF1+term.Key(e.KeyCode-consts.KbShiftF1)/0x100, term.ModShift)
	}
	if e.KeyCode >= consts.KbCtrlF1 && e.KeyCode <= consts.KbCtrlF10 {
		return setKey(term.KeyF1+term.Key(e.KeyCode-consts.KbCtrlF1)/0x100, term.ModCtrl)
	}
	if e.KeyCode >= consts.KbAltF1 && e.KeyCode <= consts.KbAltF10 {
		return setKey(term.KeyF1+term.Key(e.KeyCode-consts.KbAltF1)/0x100, term.ModAlt)
	}

	if id.Rune != 0 {
		id.Valid = true
		return id
	}
	if e.KeyCode == consts.KbNoKey && id.Mods.Has(term.ModCtrl) {
		// The terminal reader explicitly projects byte 0 as Ctrl-Space.
		// Valid cannot be inferred from Rune alone because NUL is zero.
		id.Valid = true
		return id
	}
	if r, ok := legacyAltRune(e.KeyCode); ok {
		id.Rune = r
		id.Mods |= term.ModAlt
		id.Valid = true
		return id
	}
	// Legacy Ctrl-letter codes carry the C0 byte in their low byte.
	// Ctrl-I/Tab and Ctrl-M/Enter are intrinsically ambiguous in classic
	// terminal protocols; named Tab/Enter codes were handled above.
	if c, ok := legacyCtrlByte(e.KeyCode); ok {
		id.Rune = rune(c)
		id.Mods |= term.ModCtrl
		id.Valid = true
		return id
	}
	if c := rune(e.KeyCode & 0xff); c != 0 {
		id.Rune = c
		id.Valid = true
	}
	return id
}

func legacyCtrlByte(k uint16) (byte, bool) {
	switch k {
	case consts.KbCtrlA, consts.KbCtrlB, consts.KbCtrlC, consts.KbCtrlD,
		consts.KbCtrlE, consts.KbCtrlF, consts.KbCtrlG, consts.KbCtrlH,
		consts.KbCtrlI, consts.KbCtrlJ, consts.KbCtrlK, consts.KbCtrlL,
		consts.KbCtrlM, consts.KbCtrlN, consts.KbCtrlO, consts.KbCtrlP,
		consts.KbCtrlQ, consts.KbCtrlR, consts.KbCtrlS, consts.KbCtrlT,
		consts.KbCtrlU, consts.KbCtrlV, consts.KbCtrlW, consts.KbCtrlX,
		consts.KbCtrlY, consts.KbCtrlZ:
		return byte(k & 0xff), true
	}
	return 0, false
}

func modsFromKeyShift(shift uint16) term.ModBits {
	var mods term.ModBits
	if shift&consts.KbBothShifts != 0 {
		mods |= term.ModShift
	}
	if shift&consts.KbAltShift != 0 {
		mods |= term.ModAlt
	}
	if shift&consts.KbCtrlShift != 0 {
		mods |= term.ModCtrl
	}
	return mods
}

func legacyAltRune(k uint16) (rune, bool) {
	switch k {
	case consts.KbAltQ:
		return 'q', true
	case consts.KbAltW:
		return 'w', true
	case consts.KbAltE:
		return 'e', true
	case consts.KbAltR:
		return 'r', true
	case consts.KbAltT:
		return 't', true
	case consts.KbAltY:
		return 'y', true
	case consts.KbAltU:
		return 'u', true
	case consts.KbAltI:
		return 'i', true
	case consts.KbAltO:
		return 'o', true
	case consts.KbAltP:
		return 'p', true
	case consts.KbAltA:
		return 'a', true
	case consts.KbAltS:
		return 's', true
	case consts.KbAltD:
		return 'd', true
	case consts.KbAltF:
		return 'f', true
	case consts.KbAltG:
		return 'g', true
	case consts.KbAltH:
		return 'h', true
	case consts.KbAltJ:
		return 'j', true
	case consts.KbAltK:
		return 'k', true
	case consts.KbAltL:
		return 'l', true
	case consts.KbAltZ:
		return 'z', true
	case consts.KbAltX:
		return 'x', true
	case consts.KbAltC:
		return 'c', true
	case consts.KbAltV:
		return 'v', true
	case consts.KbAltB:
		return 'b', true
	case consts.KbAltN:
		return 'n', true
	case consts.KbAltM:
		return 'm', true
	case consts.KbAlt0:
		return '0', true
	case consts.KbAlt1:
		return '1', true
	case consts.KbAlt2:
		return '2', true
	case consts.KbAlt3:
		return '3', true
	case consts.KbAlt4:
		return '4', true
	case consts.KbAlt5:
		return '5', true
	case consts.KbAlt6:
		return '6', true
	case consts.KbAlt7:
		return '7', true
	case consts.KbAlt8:
		return '8', true
	case consts.KbAlt9:
		return '9', true
	case consts.KbAltMinus:
		return '-', true
	case consts.KbAltEqual:
		return '=', true
	}
	return 0, false
}
