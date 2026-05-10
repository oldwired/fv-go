package dialogs

import (
	"github.com/oldwired/fv-go/pkg/fv/clipboard"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/validators"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// InputLine is a single-line text editor with selection, an optional
// validator, and clipboard integration.
//
// Selection: SelAnchor is the rune index where the current selection
// begins; CurPos is its other end. SelAnchor < 0 means "no selection".
// Selection grows when the user holds Shift while moving the caret;
// any non-Shift navigation collapses it.
type InputLine struct {
	views.Base

	Data      []rune // current value
	MaxLen    int
	CurPos    int // caret position (rune index)
	FirstPos  int // leftmost displayed rune index
	SelAnchor int // -1 = no active selection
	Validator validators.Validator
}

// NewInputLine builds an InputLine that holds up to maxLen runes.
func NewInputLine(bounds geom.Rect, maxLen int) *InputLine {
	il := &InputLine{
		Base:      views.NewBase(bounds),
		MaxLen:    maxLen,
		SelAnchor: -1,
	}
	il.SetSelf(il)
	il.Options |= consts.OfSelectable | consts.OfFirstClick
	il.State |= consts.SfCursorVis
	return il
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
	bg := types.MakeAttr(0x00, 0x07)       // black on light gray (unfocused)
	fg := types.MakeAttr(0x0F, 0x01)       // white on blue (focused)
	selColor := types.MakeAttr(0x0F, 0x06) // bright white on yellow (selection)
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
			il.Owner.Focus(il)
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
func (il *InputLine) insertText(s string) {
	il.deleteSelection()
	for _, r := range s {
		if r == '\r' || r == '\n' || r == '\t' {
			continue
		}
		if il.MaxLen != 0 && len(il.Data) >= il.MaxLen {
			break
		}
		il.Data = append(il.Data[:il.CurPos], append([]rune{r}, il.Data[il.CurPos:]...)...)
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
