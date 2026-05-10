// Package toggle provides ToggleSwitch — an interactive on/off control
// with three visual styles (slider, checkbox, brackets) and an optional
// label whose ~hotkey~ gets highlighted.
package toggle

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Style picks the visual rendering of the switch body.
type Style int

const (
	StyleSlider   Style = iota // [───●] / [●───]
	StyleCheckbox              // [ ] / [✓]
	StyleBrackets              // [OFF] / [ON ]
)

// ToggleSwitch is a focusable boolean toggle.
type ToggleSwitch struct {
	views.Base

	Value   bool
	Command uint16
	Style   Style
	Label   string // text after the switch; ~X~ marks a hotkey letter
}

// New constructs a switch.
func New(bounds geom.Rect, label string, command uint16, initial bool) *ToggleSwitch {
	t := &ToggleSwitch{
		Base:    views.NewBase(bounds),
		Value:   initial,
		Command: command,
		Label:   label,
		Style:   StyleSlider,
	}
	t.SetSelf(t)
	t.Options |= consts.OfSelectable | consts.OfFirstClick | consts.OfPreProcess
	t.EventMask |= consts.EvBroadcast
	return t
}

// GetTypeID for serial registry.
func (t *ToggleSwitch) GetTypeID() string { return "toggle" }

// Toggle flips Value and redraws.
func (t *ToggleSwitch) Toggle() {
	t.Value = !t.Value
	t.Draw()
}

// Press toggles + broadcasts cmd.
func (t *ToggleSwitch) Press() {
	t.Toggle()
	if t.Command != 0 {
		ev := drivers.Event{
			What:    consts.EvBroadcast,
			Command: t.Command,
			InfoPtr: t,
		}
		t.PutEvent(&ev)
	}
}

// switchVisual returns the rune sequence that paints the switch body.
func (t *ToggleSwitch) switchVisual() string {
	switch t.Style {
	case StyleCheckbox:
		if t.Value {
			return "[✓]"
		}
		return "[ ]"
	case StyleBrackets:
		if t.Value {
			return "[ON ]"
		}
		return "[OFF]"
	}
	if t.Value {
		return "[───●]"
	}
	return "[●───]"
}

// Draw paints body + label.
func (t *ToggleSwitch) Draw() {
	body := types.MakeAttr(0x07, 0x03)  // gray on cyan
	label := types.MakeAttr(0x00, 0x03) // black on cyan
	hot := types.MakeAttr(0x0F, 0x03)   // bright white hotkey
	on := types.MakeAttr(0x0E, 0x03)    // yellow when ON
	if t.GetState(consts.SfFocused) {
		body = types.MakeAttr(0x0F, 0x06)
		label = types.MakeAttr(0x0F, 0x06)
		hot = types.MakeAttr(0x0E, 0x06)
		on = types.MakeAttr(0x0F, 0x06)
	}

	buf := screen.MakeDrawBuffer(t.Size.X)
	for x := 0; x < t.Size.X; x++ {
		screen.DrawCell(buf, x, " ", label)
	}
	visual := t.switchVisual()
	visualColor := body
	if t.Value {
		visualColor = on
	}
	screen.DrawStr(buf, 0, visual, visualColor)
	x := len([]rune(visual)) + 1
	if t.Label != "" && x < t.Size.X {
		screen.DrawCStr(buf, x, t.Label, label, hot)
	}
	t.WriteLine(0, 0, t.Size.X, 1, buf)
}

// HandleEvent: space / Enter toggle, mouse-down toggles, Alt+hotkey
// activates from anywhere in the dialog.
func (t *ToggleSwitch) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		t.Press()
		t.ClearEvent(ev)
		return
	}
	if ev.What == consts.EvKeyDown {
		if t.GetState(consts.SfFocused) {
			if ev.KeyCode == consts.KbEnter || ev.KeyCode == consts.KbSpaceBar {
				t.Press()
				t.ClearEvent(ev)
				return
			}
		}
		// Alt+hotkey
		if ev.KeyShift&consts.KbAltShift != 0 {
			letter := byte(ev.UnicodeChar)
			if letter == 0 {
				letter = byte(ev.KeyCode & 0xFF)
			}
			if hot := hotkeyOf(t.Label); hot != 0 && eqIgnoreCase(letter, hot) {
				if t.Owner != nil {
					t.Owner.Focus(t.Self())
				}
				t.Press()
				t.ClearEvent(ev)
			}
		}
	}
}

// DataSize / GetData / SetData expose Value as a single byte.
func (t *ToggleSwitch) DataSize() int { return 1 }

func (t *ToggleSwitch) GetData(buf []byte) {
	if len(buf) >= 1 {
		if t.Value {
			buf[0] = 1
		} else {
			buf[0] = 0
		}
	}
}

func (t *ToggleSwitch) SetData(buf []byte) {
	if len(buf) >= 1 {
		t.Value = buf[0] != 0
		t.Draw()
	}
}

func hotkeyOf(s string) byte {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '~' {
			return s[i+1]
		}
	}
	return 0
}

func eqIgnoreCase(a, b byte) bool {
	if a >= 'A' && a <= 'Z' {
		a += 32
	}
	if b >= 'A' && b <= 'Z' {
		b += 32
	}
	return a == b
}
