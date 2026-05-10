package dialogs

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/validators"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// InputLine is a single-line text editor with optional validator.
type InputLine struct {
	views.Base

	Data      []rune // current value
	MaxLen    int
	CurPos    int // caret position (rune index)
	FirstPos  int // leftmost displayed rune index
	Validator validators.Validator
}

// NewInputLine builds an InputLine that holds up to maxLen runes.
func NewInputLine(bounds geom.Rect, maxLen int) *InputLine {
	il := &InputLine{
		Base:   views.NewBase(bounds),
		MaxLen: maxLen,
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
	il.Draw()
}

// Text returns the current contents.
func (il *InputLine) Text() string { return string(il.Data) }

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

// Draw paints the field.
func (il *InputLine) Draw() {
	// Black on light gray when unfocused; bright white on blue when focused.
	color := types.MakeAttr(0x00, 0x07)
	if il.GetState(consts.SfFocused) {
		color = types.MakeAttr(0x0F, 0x01)
	}
	w := il.Size.X
	buf := screen.MakeDrawBuffer(w)
	for x := 0; x < w; x++ {
		screen.DrawCell(buf, x, " ", color)
	}
	visible := string(il.Data)
	if il.FirstPos > 0 {
		// trim to display starting at FirstPos
		runes := []rune(visible)
		if il.FirstPos < len(runes) {
			visible = string(runes[il.FirstPos:])
		} else {
			visible = ""
		}
	}
	visible = utf8.CopyDisplayCells(visible, 0, w-1)
	screen.DrawStr(buf, 1, visible, color)
	il.WriteLine(0, 0, w, 1, buf)
	il.Cursor = geom.Point{X: 1 + il.CurPos - il.FirstPos, Y: 0}
}

// HandleEvent implements basic single-line editing.
func (il *InputLine) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		// Move caret to clicked position and request focus.
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
		}
		if il.Owner != nil {
			il.Owner.Focus(il)
		}
		il.Draw()
		il.ClearEvent(ev)
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbLeft:
		if il.CurPos > 0 {
			il.CurPos--
		}
	case consts.KbRight:
		if il.CurPos < len(il.Data) {
			il.CurPos++
		}
	case consts.KbHome:
		il.CurPos = 0
	case consts.KbEnd:
		il.CurPos = len(il.Data)
	case consts.KbBack:
		if il.CurPos > 0 {
			il.Data = append(il.Data[:il.CurPos-1], il.Data[il.CurPos:]...)
			il.CurPos--
			il.checkInputValidator()
		}
	case consts.KbDel:
		if il.CurPos < len(il.Data) {
			il.Data = append(il.Data[:il.CurPos], il.Data[il.CurPos+1:]...)
			il.checkInputValidator()
		}
	default:
		if ev.UnicodeChar >= ' ' {
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
