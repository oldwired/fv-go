package dialogs

import (
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Button-flag bits, mirroring TButton.Flags.
const (
	BfNormal    byte = 0
	BfDefault   byte = 0x01
	BfLeftJust  byte = 0x02
	BfBroadcast byte = 0x04
	BfGrabFocus byte = 0x08
)

// Button is a clickable button with a hotkey letter.
type Button struct {
	views.Base

	Title     string
	Command   uint16
	Flags     byte
	AmDefault bool
	pressed   bool
}

// NewButton builds a Button. The title can include '~' to mark a hotkey.
func NewButton(bounds geom.Rect, title string, command uint16, flags byte) *Button {
	b := &Button{
		Base:    views.NewBase(bounds),
		Title:   title,
		Command: command,
		Flags:   flags,
	}
	b.SetSelf(b)
	b.Options |= consts.OfSelectable | consts.OfFirstClick | consts.OfPreProcess | consts.OfPostProcess
	b.EventMask |= consts.EvBroadcast
	if flags&BfDefault != 0 {
		b.AmDefault = true
		b.State |= consts.SfDefault
	}
	return b
}

// GetTypeID for serial registry.
func (b *Button) GetTypeID() string { return "button" }

// Draw paints the button. Default buttons have a leading ▶, focused
// buttons reverse-video the background, pressed buttons flash, and
// every button casts a one-cell shadow.
func (b *Button) Draw() {
	// TV cButton palette: black-on-green normal, bright-white when
	// focused or default, white-on-blue while pressed.
	normal := types.MakeAttr(0x00, 0x02) // black on green
	hot := types.MakeAttr(0x0E, 0x02)    // yellow hot
	if b.AmDefault || b.GetState(consts.SfDefault) {
		normal = types.MakeAttr(0x0F, 0x02)
		hot = types.MakeAttr(0x0E, 0x02)
	}
	if b.GetState(consts.SfFocused) {
		// Brighter green for the focused button.
		normal = types.MakeAttr(0x0F, 0x0A) // bright white on light green
		hot = types.MakeAttr(0x0E, 0x0A)
	}
	if b.pressed {
		normal = types.MakeAttr(0x0F, 0x01)
		hot = types.MakeAttr(0x0E, 0x01)
	}
	w := b.Size.X
	buf := screen.MakeDrawBuffer(w)
	for x := 0; x < w; x++ {
		screen.DrawCell(buf, x, " ", normal)
	}
	tw := utf8.CStrDisplayWidth(b.Title)
	startX := (w - tw) / 2
	if startX < 1 {
		startX = 1
	}
	screen.DrawCStr(buf, startX, b.Title, normal, hot)
	if b.AmDefault {
		screen.DrawStr(buf, 0, "▶", normal)
	}
	b.WriteLine(0, 0, w, 1, buf)

	// Shadow: a half-height row (▀ upper-half block) below the button,
	// plus one full cell to the right. The half-block gives a thin
	// drop-shadow rather than a solid black bar.
	//
	// The cell uses FG=dark-gray and BG=dialog-cyan so the upper half
	// renders as shadow and the lower half blends into the dialog.
	if !b.pressed {
		shadow := types.MakeAttr(0x08, 0x03) // dark gray on cyan
		halfRow := make(screen.DrawBuffer, w)
		for i := range halfRow {
			halfRow[i] = types.DrawCell{Ch: "▀", Attr: shadow}
		}
		b.WriteLine(1, 1, w, 1, halfRow)
		b.WriteLine(w, 0, 1, 1, screen.DrawBuffer{{Ch: "▄", Attr: shadow}})
	}
}

// HandleEvent: space, enter, click -> press.
func (b *Button) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		b.press(ev)
		return
	}
	if ev.What == consts.EvKeyDown {
		hot := hotkeyOf(b.Title)
		switch ev.KeyCode {
		case consts.KbEnter:
			if b.GetState(consts.SfFocused) || b.AmDefault {
				b.press(ev)
				return
			}
		case consts.KbSpaceBar:
			if b.GetState(consts.SfFocused) {
				b.press(ev)
				return
			}
		}
		if hot != 0 && (ev.KeyShift&consts.KbAltShift) != 0 {
			letter := byte(ev.UnicodeChar)
			if letter == 0 {
				letter = byte(ev.KeyCode & 0xFF)
			}
			if equalIgnoreCase(letter, hot) {
				b.press(ev)
				return
			}
		}
	}
	if ev.What == consts.EvBroadcast && ev.Command == consts.CmDefault && b.AmDefault {
		b.press(ev)
		return
	}
}

func (b *Button) press(ev *drivers.Event) {
	// Brief pressed-state flash so the click is visually confirmed.
	b.pressed = true
	b.Draw()
	_ = views.Flush()
	time.Sleep(60 * time.Millisecond)
	b.pressed = false
	b.Draw()
	cmd := drivers.Event{What: consts.EvCommand, Command: b.Command}
	b.PutEvent(&cmd)
	b.ClearEvent(ev)
}

func hotkeyOf(s string) byte {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '~' {
			return s[i+1]
		}
	}
	return 0
}

func equalIgnoreCase(a, b byte) bool {
	if a >= 'A' && a <= 'Z' {
		a += 32
	}
	if b >= 'A' && b <= 'Z' {
		b += 32
	}
	return a == b
}
