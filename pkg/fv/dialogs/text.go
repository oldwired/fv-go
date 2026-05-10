package dialogs

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// StaticText displays a non-interactive multi-line string. Text may
// contain '\n' for line breaks.
type StaticText struct {
	views.Base
	Text string
}

// NewStaticText builds a StaticText.
func NewStaticText(bounds geom.Rect, text string) *StaticText {
	s := &StaticText{Base: views.NewBase(bounds), Text: text}
	s.SetSelf(s)
	return s
}

// GetTypeID for serial registry.
func (s *StaticText) GetTypeID() string { return "statictext" }

// Draw paints the text wrapped to bounds.
func (s *StaticText) Draw() {
	color := types.MakeAttr(0x00, 0x03) // black on dialog cyan
	y := 0
	start := 0
	for i := 0; i <= len(s.Text); i++ {
		if i == len(s.Text) || s.Text[i] == '\n' {
			line := s.Text[start:i]
			buf := screen.MakeDrawBuffer(s.Size.X)
			for x := 0; x < s.Size.X; x++ {
				screen.DrawCell(buf, x, " ", color)
			}
			screen.DrawStr(buf, 0, line, color)
			s.WriteLine(0, y, s.Size.X, 1, buf)
			y++
			start = i + 1
			if y >= s.Size.Y {
				return
			}
		}
	}
	for ; y < s.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(s.Size.X)
		for x := 0; x < s.Size.X; x++ {
			screen.DrawCell(buf, x, " ", color)
		}
		s.WriteLine(0, y, s.Size.X, 1, buf)
	}
}

// Label is a StaticText linked to a focusable control. Clicking the
// label or pressing its hotkey transfers focus to the linked control.
type Label struct {
	StaticText
	Link views.View
}

// NewLabel builds a Label.
func NewLabel(bounds geom.Rect, text string, link views.View) *Label {
	l := &Label{StaticText: *NewStaticText(bounds, text), Link: link}
	l.SetSelf(l)
	l.Options |= consts.OfPreProcess | consts.OfPostProcess
	return l
}

// GetTypeID for serial registry.
func (l *Label) GetTypeID() string { return "label" }

// Draw renders the label using CStr semantics so '~' marks the hotkey
// letter for highlighting.
func (l *Label) Draw() {
	normal := types.MakeAttr(0x00, 0x03) // black on cyan to match dialog
	hot := types.MakeAttr(0x0F, 0x03)    // bright white hotkey
	if l.Link != nil && l.Link.BaseView().GetState(consts.SfFocused) {
		// Subtle "this label's target is focused" cue.
		normal = types.MakeAttr(0x0F, 0x03)
		hot = types.MakeAttr(0x0E, 0x03)
	}
	buf := screen.MakeDrawBuffer(l.Size.X)
	for x := 0; x < l.Size.X; x++ {
		screen.DrawCell(buf, x, " ", normal)
	}
	screen.DrawCStr(buf, 0, l.Text, normal, hot)
	l.WriteLine(0, 0, l.Size.X, 1, buf)
}

// HandleEvent: Alt-letter focuses linked control.
func (l *Label) HandleEvent(ev *drivers.Event) {
	if ev.What != consts.EvKeyDown || l.Link == nil {
		return
	}
	if ev.KeyShift&consts.KbAltShift == 0 {
		return
	}
	hot := hotkeyOf(l.Text)
	letter := byte(ev.UnicodeChar)
	if letter == 0 {
		letter = byte(ev.KeyCode & 0xFF)
	}
	if hot != 0 && equalIgnoreCase(letter, hot) {
		// focus link
		if l.Owner != nil {
			l.Owner.Focus(l.Link)
		}
		l.ClearEvent(ev)
	}
}

// ParamText is a StaticText with sprintf-style fill at draw time.
type ParamText struct {
	StaticText
	Args []any
}

// NewParamText builds a ParamText. Caller can update Args after
// construction; next Draw will reflect the new values.
func NewParamText(bounds geom.Rect, format string, args ...any) *ParamText {
	p := &ParamText{StaticText: *NewStaticText(bounds, format), Args: args}
	p.SetSelf(p)
	return p
}

// GetTypeID for serial registry.
func (p *ParamText) GetTypeID() string { return "paramtext" }
