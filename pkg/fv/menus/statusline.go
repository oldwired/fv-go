package menus

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// StatusItem is a single hint+keyboard shortcut entry on the status
// line. "F1" + hint "Help" + command cmHelp.
type StatusItem struct {
	// Text is the displayed hint, e.g. "~F1~ Help" — '~' marks the
	// hotkey letter for hot-letter highlighting.
	Text string
	// KeyCode is the keypress that fires Command, e.g. consts.KbF1.
	KeyCode uint16
	// Command is the EvCommand emitted when the user presses KeyCode
	// or clicks the item.
	Command uint16
}

// StatusDef is one bracketed range (min..max help context) and the
// items shown when GetHelpCtx is in that range. The Pascal API exposes
// arbitrary nested defs; here we keep one default def for simplicity.
//
// Items / LeftItems render at the left edge growing right; RightItems
// renders right-justified at the right edge. Items is the legacy slot
// kept for backward compatibility; new callers should prefer
// LeftItems + RightItems. A blank middle gap separates the two sides.
type StatusDef struct {
	// Min, Max define the help-context range this def applies to.
	// 0..0xFFFF for an always-active def.
	Min, Max uint16
	// Items is the legacy single-slot, left-aligned item list. New
	// callers should prefer LeftItems + RightItems.
	Items []*StatusItem
	// LeftItems renders left-aligned starting at column 1, growing
	// right. Drawn after Items so legacy + modern layouts compose.
	LeftItems []*StatusItem
	// RightItems renders right-justified at the right edge in
	// declared order. Useful for "Alt-X Exit" / clock / status
	// indicators.
	RightItems []*StatusItem
	// Next chains another def for a different help-context range.
	// The first def whose Min..Max bracket includes the focus's
	// HelpCtx is the active one.
	Next *StatusDef
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

// Draw paints the items: legacy Items + LeftItems left-aligned at
// x=1, RightItems right-justified, a blank middle gap. Each item's
// Text is rendered as a CStr (so '~Alt-X~ Exit' becomes "Alt-X Exit"
// with "Alt-X" highlighted as the hotkey).
func (s *StatusLine) Draw() {
	pal := theme.Get()
	normal := pal.StatusBarNormal
	hot := pal.StatusBarHot
	buf := screen.MakeDrawBuffer(s.Size.X)
	for x := 0; x < s.Size.X; x++ {
		screen.DrawCell(buf, x, " ", normal)
	}
	if s.Defs == nil {
		s.WriteLine(0, 0, s.Size.X, 1, buf)
		return
	}
	// Left-aligned slot. Items is the legacy field, LeftItems is the
	// modern one; render Items first so old code keeps working, then
	// LeftItems flowing after.
	x := 1
	drawSlot := func(items []*StatusItem) {
		for _, it := range items {
			if x >= s.Size.X {
				break
			}
			text := it.Text + "  "
			screen.DrawCStr(buf, x, text, normal, hot)
			x += utf8.CStrDisplayWidth(text)
		}
	}
	drawSlot(s.Defs.Items)
	drawSlot(s.Defs.LeftItems)
	// Right-aligned slot. Measure the total width of every right
	// item, then draw starting at Size.X - total. Right items render
	// in declared order (so the first RightItem is leftmost of the
	// right group).
	if len(s.Defs.RightItems) > 0 {
		rightW := 0
		for _, it := range s.Defs.RightItems {
			rightW += utf8.CStrDisplayWidth(it.Text + "  ")
		}
		rx := s.Size.X - rightW
		if rx < x+1 {
			// No room for both sides — clip right onto whatever
			// follows left. Better than overlapping.
			rx = x + 1
		}
		for _, it := range s.Defs.RightItems {
			if rx >= s.Size.X {
				break
			}
			text := it.Text + "  "
			screen.DrawCStr(buf, rx, text, normal, hot)
			rx += utf8.CStrDisplayWidth(text)
		}
	}
	s.WriteLine(0, 0, s.Size.X, 1, buf)
}

// HandleEvent reacts to keyboard shortcuts that match items in any
// slot of the active def.
func (s *StatusLine) HandleEvent(ev *drivers.Event) {
	if ev.What != consts.EvKeyDown || s.Defs == nil {
		return
	}
	for _, slot := range [][]*StatusItem{s.Defs.Items, s.Defs.LeftItems, s.Defs.RightItems} {
		for _, it := range slot {
			if it.KeyCode == ev.KeyCode {
				notify := drivers.Event{What: consts.EvCommand, Command: it.Command}
				s.PutEvent(&notify)
				s.ClearEvent(ev)
				return
			}
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
