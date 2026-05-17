package dialogs

import (
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
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
//
// Layout convention: a button's drop shadow paints one row BELOW its
// logical bounds and one cell to the RIGHT of them — outside the
// bounds the constructor records, but inside the dialog's body. The
// shadow row and the trailing column are reserved for the button:
// any sibling widget placed in those cells will be overdrawn each
// time the button repaints (focus change, hover, press flash). A
// typical dialog reserves rows h-3 (the button row itself) and h-2
// (its shadow) for the button strip; anything above the strip stops
// at h-4.
//
// The shadow is intentionally NOT reflected in the recorded bounds
// because that would force every existing dialog layout to add a row
// — a breaking change. The convention has to live in the layout
// instead.
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
//
// The shadow is emitted via WriteLine at local coordinates (1, 1) and
// (w, 0), which means the framework writes one row BELOW the button's
// bounds and one cell to the RIGHT of them. The cells outside the
// view's bounds are still legitimate writes — Base.WriteLine doesn't
// clip — so siblings in those positions will be overdrawn. See the
// layout convention note on NewButton.
func (b *Button) Draw() {
	pal := theme.Get()
	// TV cButton palette: black-on-green normal, bright-white when
	// focused or default, white-on-blue while pressed.
	normal := pal.ButtonNormal
	hot := pal.ButtonNormalHot
	if b.AmDefault || b.GetState(consts.SfDefault) {
		normal = pal.ButtonDefault
		hot = pal.ButtonDefaultHot
	}
	if b.GetState(consts.SfFocused) {
		normal = pal.ButtonFocused
		hot = pal.ButtonFocusedHot
	}
	if b.pressed {
		normal = pal.ButtonPressed
		hot = pal.ButtonPressedHot
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
		shadow := pal.ButtonShadow
		halfRow := make(screen.DrawBuffer, w)
		for i := range halfRow {
			halfRow[i] = types.DrawCell{Ch: "▀", Attr: shadow}
		}
		b.WriteLine(1, 1, w, 1, halfRow)
		b.WriteLine(w, 0, 1, 1, screen.DrawBuffer{{Ch: "▄", Attr: shadow}})
	}
}

// HandleEvent dispatches:
//   - mouse-down -> mouseLoop (press-and-hold semantics: depress while
//     the cursor stays over the button, pop back up if it moves away,
//     fire the command only if mouse-up lands while still over)
//   - Enter / Space (when focused) -> brief press flash + fire
//   - Alt+hotkey -> brief press flash + fire
//   - cmDefault broadcast (when this is the default button) -> fire
func (b *Button) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		b.mouseLoop(ev)
		return
	}
	if ev.What == consts.EvKeyDown {
		hot := hotkeyOf(b.Title)
		switch ev.KeyCode {
		case consts.KbEnter:
			if b.GetState(consts.SfFocused) || b.AmDefault {
				b.flashAndFire(ev)
				return
			}
		case consts.KbSpaceBar:
			if b.GetState(consts.SfFocused) {
				b.flashAndFire(ev)
				return
			}
		}
		if hot != 0 && (ev.KeyShift&consts.KbAltShift) != 0 {
			letter := byte(ev.UnicodeChar)
			if letter == 0 {
				letter = byte(ev.KeyCode & 0xFF)
			}
			if equalIgnoreCase(letter, hot) {
				b.flashAndFire(ev)
				return
			}
		}
	}
	if ev.What == consts.EvBroadcast && ev.Command == consts.CmDefault && b.AmDefault {
		b.flashAndFire(ev)
		return
	}
}

// mouseLoop runs from the moment a mouse-down lands on the button.
// It depresses the button visually, follows mouse motion (popping the
// button back up if the cursor leaves the bounds, depressing again if
// it returns), and fires the command on mouse-up only if the cursor
// is still over the button. This matches the behavior of TV's TButton.
func (b *Button) mouseLoop(start *drivers.Event) {
	q := views.GetEventQueue()
	if q == nil {
		b.ClearEvent(start)
		return
	}

	over := true
	b.pressed = true
	b.Draw()
	_ = views.Flush()

	for {
		if pump := views.GetPump(); pump != nil {
			pump()
		}
		ev, ok := q.Get()
		if !ok {
			if wait := views.GetWait(); wait != nil {
				wait()
			}
			continue
		}
		switch ev.What {
		case consts.EvMouseMove, consts.EvMouseDown:
			nowOver := b.MouseInView(ev.Where)
			if nowOver != over {
				over = nowOver
				b.pressed = nowOver
				b.Draw()
				_ = views.Flush()
			}
		case consts.EvMouseUp:
			fire := over
			b.pressed = false
			b.Draw()
			_ = views.Flush()
			if fire {
				cmd := drivers.Event{What: consts.EvCommand, Command: b.Command}
				b.PutEvent(&cmd)
			}
			b.ClearEvent(start)
			return
		}
	}
}

// flashAndFire is the keyboard / hotkey activation: a brief depressed
// flash with no chance for the user to cancel.
func (b *Button) flashAndFire(ev *drivers.Event) {
	b.pressed = true
	b.Draw()
	_ = views.Flush()
	time.Sleep(60 * time.Millisecond)
	b.pressed = false
	b.Draw()
	_ = views.Flush()
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
