package dialogs

import (
	"github.com/oldwired/fv-go/pkg/fv/clipboard"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/history"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/validators"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// InputLine is a single-line text editor with selection, an optional
// validator, optional history recall, and clipboard integration.
//
// Selection: SelAnchor is the rune index where the current selection
// begins; CurPos is its other end. SelAnchor < 0 means "no selection".
// Selection grows when the user holds Shift while moving the caret;
// any non-Shift navigation collapses it.
//
// History: when HistoryID is non-zero, Up/Down recall past entries
// from the package-level history store, and Commit() pushes the
// current value as a new entry.
type InputLine struct {
	views.Base

	Data []rune // current value (rune slice for O(1) Cursor moves)
	// MaxLen caps Data length in runes. Typing past this is dropped.
	MaxLen int
	// CurPos is the caret position as a rune index into Data.
	CurPos int
	// FirstPos is the rune index of the leftmost on-screen character;
	// updated as the caret scrolls horizontally past the field width.
	FirstPos int
	// SelAnchor is the rune index where the selection began, or -1
	// when there's no active selection. CurPos is the other end.
	SelAnchor int
	// Validator, when non-nil, screens individual keystrokes (Format)
	// and the final value (IsValidInput / IsValid). See
	// pkg/fv/validators for the built-in pickers/filters.
	Validator validators.Validator
	// HistoryID, when non-zero, names a slot in the package-level
	// pkg/fv/history store: Up/Down recalls past values and Commit
	// pushes the current value as a fresh entry.
	HistoryID byte
	histPos   int // -1 = at the live edit; >=0 = browsing
}

// NewInputLine builds an InputLine that holds up to maxLen runes.
func NewInputLine(bounds geom.Rect, maxLen int) *InputLine {
	il := &InputLine{
		Base:      views.NewBase(bounds),
		MaxLen:    maxLen,
		SelAnchor: -1,
		histPos:   -1,
	}
	il.SetSelf(il)
	il.Options |= consts.OfSelectable | consts.OfFirstClick
	il.State |= consts.SfCursorVis
	return il
}

// Commit pushes the current text to history (if HistoryID != 0). Call
// from the parent dialog's OK handler to record the entry.
func (il *InputLine) Commit() {
	if il.HistoryID != 0 {
		history.Add(il.HistoryID, string(il.Data))
	}
}

// recallFromHistory replaces the buffer with the n-th entry of the
// history list. n=-1 returns to the live edit (an empty string).
func (il *InputLine) recallFromHistory(step int) {
	if il.HistoryID == 0 {
		return
	}
	list := history.Get(il.HistoryID)
	if len(list) == 0 {
		return
	}
	il.histPos += step
	if il.histPos < 0 {
		il.histPos = -1
		il.SetText("")
		return
	}
	if il.histPos >= len(list) {
		il.histPos = len(list) - 1
	}
	il.SetText(list[il.histPos])
}

// GetTypeID for serial registry.
func (il *InputLine) GetTypeID() string { return "inputline" }

// SetText replaces the contents.
func (il *InputLine) SetText(s string) {
	il.Data = []rune(s)
	if il.MaxLen > 0 && len(il.Data) > il.MaxLen {
		il.Data = il.Data[:il.MaxLen]
	}
	il.CurPos = len(il.Data)
	il.FirstPos = 0
	il.SelAnchor = -1
	il.Draw()
}

// Text returns the current contents.
func (il *InputLine) Text() string { return string(il.Data) }

// SelectAll marks the entire field as selected.
func (il *InputLine) SelectAll() {
	il.SelAnchor = 0
	il.CurPos = len(il.Data)
}

// hasSelection reports whether SelAnchor differs from CurPos.
func (il *InputLine) hasSelection() bool {
	return il.SelAnchor >= 0 && il.SelAnchor != il.CurPos
}

// selectionRange returns the [low, high) rune-index range. Caller must
// have confirmed hasSelection.
func (il *InputLine) selectionRange() (int, int) {
	a, b := il.SelAnchor, il.CurPos
	if a > b {
		a, b = b, a
	}
	return a, b
}

// deleteSelection removes the selected runes (if any) and moves the
// caret to the start of where the selection used to be.
func (il *InputLine) deleteSelection() bool {
	if !il.hasSelection() {
		return false
	}
	lo, hi := il.selectionRange()
	il.Data = append(il.Data[:lo], il.Data[hi:]...)
	il.CurPos = lo
	il.SelAnchor = -1
	return true
}

// DataSize / GetData / SetData implement IFVDataAware (just text bytes).
func (il *InputLine) DataSize() int { return il.MaxLen + 1 }

func (il *InputLine) GetData(buf []byte) {
	src := []byte(string(il.Data))
	copy(buf, src)
	for i := len(src); i < len(buf); i++ {
		buf[i] = 0
	}
}

func (il *InputLine) SetData(buf []byte) {
	end := len(buf)
	for i, b := range buf {
		if b == 0 {
			end = i
			break
		}
	}
	il.SetText(string(buf[:end]))
}

// Draw paints the field, highlighting any active selection range.
func (il *InputLine) Draw() {
	pal := theme.Get()
	bg := pal.InputUnfocused
	fg := pal.InputFocused
	selColor := pal.InputSelected
	color := bg
	if il.GetState(consts.SfFocused) {
		color = fg
	}
	w := il.Size.X
	buf := screen.MakeDrawBuffer(w)
	for x := 0; x < w; x++ {
		screen.DrawCell(buf, x, " ", color)
	}

	// Render runes from FirstPos through end-of-field, applying the
	// selection color to indices inside [selLo, selHi).
	selLo, selHi := -1, -1
	if il.hasSelection() {
		selLo, selHi = il.selectionRange()
	}
	x := 1
	for i := il.FirstPos; i < len(il.Data) && x < w; i++ {
		c := color
		if i >= selLo && i < selHi {
			c = selColor
		}
		buf[x] = types.DrawCell{Ch: string(il.Data[i]), Attr: c}
		x++
	}
	il.WriteLine(0, 0, w, 1, buf)
	il.Cursor = geom.Point{X: 1 + il.CurPos - il.FirstPos, Y: 0}
}

// HandleEvent implements editing, navigation, selection, and clipboard.
func (il *InputLine) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		local := il.MakeLocal(ev.Where)
		if local.Y == 0 {
			target := local.X - 1 + il.FirstPos
			if target < 0 {
				target = 0
			}
			if target > len(il.Data) {
				target = len(il.Data)
			}
			il.CurPos = target
			il.SelAnchor = -1
		}
		if il.Owner != nil {
			// Self() returns the concrete subclass pointer (e.g.
			// *InputLong) the parent group actually stores.
			il.Owner.Focus(il.Self())
		}
		il.Draw()
		il.ClearEvent(ev)
		return
	}

	// Clipboard / edit commands. cmPaste arrives both from bracketed
	// paste (with InfoPtr=string) and from menu shortcuts (no info,
	// so we fall back to the package-local clipboard).
	if ev.What == consts.EvCommand {
		switch ev.Command {
		case consts.CmCopy:
			il.copySelection()
			il.ClearEvent(ev)
			return
		case consts.CmCut:
			il.copySelection()
			if il.deleteSelection() {
				il.adjustScroll()
				il.Draw()
			}
			il.ClearEvent(ev)
			return
		case consts.CmClear:
			if il.deleteSelection() {
				il.adjustScroll()
				il.Draw()
			}
			il.ClearEvent(ev)
			return
		case consts.CmPaste:
			text, _ := ev.InfoPtr.(string)
			if text == "" {
				text = clipboard.GetText()
			}
			if text != "" {
				il.insertText(text)
			}
			il.ClearEvent(ev)
			return
		}
	}

	if ev.What != consts.EvKeyDown {
		return
	}
	shift := ev.KeyShift&(consts.KbLeftShift|consts.KbRightShift) != 0
	switch ev.KeyCode {
	case consts.KbLeft:
		il.startOrClearSelection(shift)
		if il.CurPos > 0 {
			il.CurPos--
		}
	case consts.KbRight:
		il.startOrClearSelection(shift)
		if il.CurPos < len(il.Data) {
			il.CurPos++
		}
	case consts.KbHome:
		il.startOrClearSelection(shift)
		il.CurPos = 0
	case consts.KbEnd:
		il.startOrClearSelection(shift)
		il.CurPos = len(il.Data)
	case consts.KbCtrlA:
		il.SelectAll()
	case consts.KbUp:
		if il.HistoryID != 0 {
			il.recallFromHistory(+1)
			il.ClearEvent(ev)
			return
		}
	case consts.KbDown:
		if il.HistoryID != 0 {
			il.recallFromHistory(-1)
			il.ClearEvent(ev)
			return
		}
	case consts.KbBack:
		if !il.deleteSelection() && il.CurPos > 0 {
			il.Data = append(il.Data[:il.CurPos-1], il.Data[il.CurPos:]...)
			il.CurPos--
		}
		il.checkInputValidator()
	case consts.KbDel:
		if !il.deleteSelection() && il.CurPos < len(il.Data) {
			il.Data = append(il.Data[:il.CurPos], il.Data[il.CurPos+1:]...)
		}
		il.checkInputValidator()
	default:
		if ev.UnicodeChar >= ' ' {
			il.deleteSelection()
			if il.MaxLen == 0 || len(il.Data) < il.MaxLen {
				newData := make([]rune, 0, len(il.Data)+1)
				newData = append(newData, il.Data[:il.CurPos]...)
				newData = append(newData, ev.UnicodeChar)
				newData = append(newData, il.Data[il.CurPos:]...)
				if il.passesInputValidator(string(newData)) {
					il.Data = newData
					il.CurPos++
				}
			}
		} else {
			return
		}
	}
	il.adjustScroll()
	il.Draw()
	il.ClearEvent(ev)
}

// startOrClearSelection arms the selection anchor before a navigation
// keystroke if Shift is held; otherwise drops any existing selection.
func (il *InputLine) startOrClearSelection(shift bool) {
	if shift {
		if il.SelAnchor < 0 {
			il.SelAnchor = il.CurPos
		}
	} else {
		il.SelAnchor = -1
	}
}

// copySelection writes the current selection (or the whole field if
// no selection) to the clipboard.
func (il *InputLine) copySelection() {
	var s string
	if il.hasSelection() {
		lo, hi := il.selectionRange()
		s = string(il.Data[lo:hi])
	} else {
		s = string(il.Data)
	}
	clipboard.SetText(s)
}

// insertText replaces the selection (or inserts at the caret) with s.
// Newlines and tabs are dropped — InputLine is single-line.
//
// Each candidate rune is screened by passesInputValidator before it's
// committed, mirroring the keystroke path. Without this, a paste or
// other programmatic insert could place characters into the field
// that the validator would reject from typing — surprising for the
// user and a real bug for fields like numeric or restricted-charset
// inputs.
func (il *InputLine) insertText(s string) {
	il.deleteSelection()
	for _, r := range s {
		if r == '\r' || r == '\n' || r == '\t' {
			continue
		}
		if il.MaxLen != 0 && len(il.Data) >= il.MaxLen {
			break
		}
		candidate := make([]rune, 0, len(il.Data)+1)
		candidate = append(candidate, il.Data[:il.CurPos]...)
		candidate = append(candidate, r)
		candidate = append(candidate, il.Data[il.CurPos:]...)
		if !il.passesInputValidator(string(candidate)) {
			// Skip this rune; keep going so a paste with one bad
			// character still lands the good ones.
			continue
		}
		il.Data = candidate
		il.CurPos++
	}
	il.adjustScroll()
	il.Draw()
}

func (il *InputLine) adjustScroll() {
	visW := il.Size.X - 1
	if visW < 1 {
		visW = 1
	}
	if il.CurPos < il.FirstPos {
		il.FirstPos = il.CurPos
	} else if il.CurPos-il.FirstPos >= visW {
		il.FirstPos = il.CurPos - visW + 1
	}
}

func (il *InputLine) passesInputValidator(s string) bool {
	if il.Validator == nil {
		return true
	}
	return il.Validator.IsValidInput(&s, true)
}

func (il *InputLine) checkInputValidator() {
	if il.Validator == nil {
		return
	}
	s := string(il.Data)
	il.Validator.IsValidInput(&s, true)
	il.Data = []rune(s)
}

// Valid is called before the dialog accepts; returns true iff the
// validator (if any) accepts the current text.
func (il *InputLine) Valid(command uint16) bool {
	if command == consts.CmCancel {
		return true
	}
	if il.Validator == nil {
		return true
	}
	return il.Validator.IsValid(string(il.Data))
}
