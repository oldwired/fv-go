package menus

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// StatusItem is a single hint+keyboard shortcut entry on the status
// line. "F1" + hint "Help" + command cmHelp.
type StatusItem struct {
	Text    string
	KeyCode uint16
	Command uint16
}

// StatusDef is one bracketed range (min..max help context) and the
// items shown when GetHelpCtx is in that range. The Pascal API exposes
// arbitrary nested defs; here we keep one default def for simplicity.
type StatusDef struct {
	Min, Max uint16
	Items    []*StatusItem
	Next     *StatusDef
}

// StatusLine is the bottom-row hint bar.
type StatusLine struct {
	views.Base
	Defs *StatusDef
}

// NewStatusLine builds a one-line status bar with one default def.
func NewStatusLine(bounds geom.Rect, items []*StatusItem) *StatusLine {
	s := &StatusLine{
		Base: views.NewBase(bounds),
		Defs: &StatusDef{Items: items, Min: 0, Max: 0xFFFF},
	}
	s.SetSelf(s)
	s.GrowMode = consts.GfGrowLoY | consts.GfGrowHiX | consts.GfGrowHiY
	s.Options = consts.OfPreProcess
	s.EventMask = consts.EvMouseDown | consts.EvKeyDown | consts.EvBroadcast
	return s
}

// GetTypeID for serial registry.
func (s *StatusLine) GetTypeID() string { return "statusline" }

// Draw paints the items in their declared order. Each item's Text
// is rendered as a CStr (so '~Alt-X~ Exit' becomes "Alt-X Exit" with
// "Alt-X" highlighted as the hotkey).
func (s *StatusLine) Draw() {
	normal := types.MakeAttr(0x00, 0x07) // black on light gray
	hot := types.MakeAttr(0x04, 0x07)    // CGA red on light gray
	buf := screen.MakeDrawBuffer(s.Size.X)
	for x := 0; x < s.Size.X; x++ {
		screen.DrawCell(buf, x, " ", normal)
	}
	x := 1
	if s.Defs == nil {
		s.WriteLine(0, 0, s.Size.X, 1, buf)
		return
	}
	for _, it := range s.Defs.Items {
		if x >= s.Size.X {
			break
		}
		text := it.Text + "  "
		screen.DrawCStr(buf, x, text, normal, hot)
		x += utf8.CStrDisplayWidth(text)
	}
	s.WriteLine(0, 0, s.Size.X, 1, buf)
}

// HandleEvent reacts to keyboard shortcuts that match items in the
// active def.
func (s *StatusLine) HandleEvent(ev *drivers.Event) {
	if ev.What != consts.EvKeyDown || s.Defs == nil {
		return
	}
	for _, it := range s.Defs.Items {
		if it.KeyCode == ev.KeyCode {
			notify := drivers.Event{What: consts.EvCommand, Command: it.Command}
			s.PutEvent(&notify)
			s.ClearEvent(ev)
			return
		}
	}
}

// KeyName returns a short human-friendly name for k. Public so demo
// code can build status hints with the right wording; the StatusLine
// itself doesn't use this anymore (callers embed the key name in the
// Text directly).
func KeyName(k uint16) string {
	switch k {
	case consts.KbF1:
		return "F1"
	case consts.KbF2:
		return "F2"
	case consts.KbF3:
		return "F3"
	case consts.KbF4:
		return "F4"
	case consts.KbF5:
		return "F5"
	case consts.KbF6:
		return "F6"
	case consts.KbF7:
		return "F7"
	case consts.KbF8:
		return "F8"
	case consts.KbF9:
		return "F9"
	case consts.KbF10:
		return "F10"
	case consts.KbAltX:
		return "Alt-X"
	case consts.KbAltF:
		return "Alt-F"
	case consts.KbCtrlF1:
		return "Ctrl-F1"
	case consts.KbEsc:
		return "Esc"
	}
	return "?"
}
