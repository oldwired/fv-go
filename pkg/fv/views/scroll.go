package views

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
)

// ScrollBar is a one-cell-wide vertical or horizontal scroll bar.
type ScrollBar struct {
	Base

	Min, Max, Value int
	PgStep, ArStep  int

	horizontal bool
}

// NewScrollBar constructs a ScrollBar. If bounds.Width <= 1 it's vertical;
// otherwise horizontal.
func NewScrollBar(bounds geom.Rect) *ScrollBar {
	s := &ScrollBar{
		Base:       NewBase(bounds),
		Min:        0,
		Max:        100,
		Value:      0,
		PgStep:     10,
		ArStep:     1,
		horizontal: bounds.Width() > 1,
	}
	s.SetSelf(s)
	if s.horizontal {
		s.GrowMode = consts.GfGrowLoY | consts.GfGrowHiX | consts.GfGrowHiY
	} else {
		s.GrowMode = consts.GfGrowLoX | consts.GfGrowHiX | consts.GfGrowHiY
	}
	return s
}

// GetTypeID for serial registry.
func (s *ScrollBar) GetTypeID() string { return "scrollbar" }

// SetRange clamps value to [min, max] and stores them.
func (s *ScrollBar) SetRange(min, max int) {
	s.Min = min
	s.Max = max
	if s.Value < min {
		s.Value = min
	}
	if s.Value > max {
		s.Value = max
	}
}

// SetValue clamps and stores v, then redraws.
func (s *ScrollBar) SetValue(v int) {
	if v < s.Min {
		v = s.Min
	}
	if v > s.Max {
		v = s.Max
	}
	if v != s.Value {
		s.Value = v
		s.Draw()
	}
}

// Draw paints the track + thumb.
func (s *ScrollBar) Draw() {
	// Track ▒ is light-gray on cyan (close to TV's classic scrollbar
	// palette); the thumb stands out as bright white on the same bg.
	// (The earlier MakeAttr(0x70, 0x70) was a literal copy of a Pascal
	// packed-attribute byte and rendered as black-on-black after the
	// low-nibble mask in legacyFGCode.)
	color := types.MakeAttr(0x07, 0x03)
	thumbColor := types.MakeAttr(0x0F, 0x03)
	if s.horizontal {
		buf := screen.MakeDrawBuffer(s.Size.X)
		screen.DrawChar(buf, 0, '◄', color, 1)
		for i := 1; i < s.Size.X-1; i++ {
			screen.DrawCell(buf, i, "▒", color)
		}
		screen.DrawChar(buf, s.Size.X-1, '►', color, 1)
		thumbX := s.thumbPos(s.Size.X - 2)
		if thumbX >= 0 {
			screen.DrawChar(buf, 1+thumbX, '█', thumbColor, 1)
		}
		s.WriteLine(0, 0, s.Size.X, 1, buf)
	} else {
		// Vertical
		topRow := screen.MakeDrawBuffer(1)
		screen.DrawChar(topRow, 0, '▲', color, 1)
		s.WriteLine(0, 0, 1, 1, topRow)
		for y := 1; y < s.Size.Y-1; y++ {
			row := screen.MakeDrawBuffer(1)
			screen.DrawCell(row, 0, "▒", color)
			s.WriteLine(0, y, 1, 1, row)
		}
		bottomRow := screen.MakeDrawBuffer(1)
		screen.DrawChar(bottomRow, 0, '▼', color, 1)
		s.WriteLine(0, s.Size.Y-1, 1, 1, bottomRow)
		thumbY := s.thumbPos(s.Size.Y - 2)
		if thumbY >= 0 {
			thumbRow := screen.MakeDrawBuffer(1)
			screen.DrawChar(thumbRow, 0, '█', thumbColor, 1)
			s.WriteLine(0, 1+thumbY, 1, 1, thumbRow)
		}
	}
}

func (s *ScrollBar) thumbPos(track int) int {
	if track <= 0 || s.Max == s.Min {
		return -1
	}
	off := (s.Value - s.Min) * (track - 1) / (s.Max - s.Min)
	if off < 0 {
		off = 0
	}
	if off > track-1 {
		off = track - 1
	}
	return off
}

// HandleEvent: arrows step by ArStep, page-up/down by PgStep, home/end
// jump to ends.
func (s *ScrollBar) HandleEvent(ev *drivers.Event) {
	s.Base.HandleEvent(ev)
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbUp, consts.KbLeft:
		s.SetValue(s.Value - s.ArStep)
	case consts.KbDown, consts.KbRight:
		s.SetValue(s.Value + s.ArStep)
	case consts.KbPgUp:
		s.SetValue(s.Value - s.PgStep)
	case consts.KbPgDn:
		s.SetValue(s.Value + s.PgStep)
	case consts.KbHome:
		s.SetValue(s.Min)
	case consts.KbEnd:
		s.SetValue(s.Max)
	default:
		return
	}
	s.ClearEvent(ev)
	notify := drivers.Event{What: consts.EvBroadcast, Command: consts.CmScrollBarChanged, InfoPtr: s}
	s.PutEvent(&notify)
}

// Scroller is an abstract scrollable area. Concrete subtypes (a text
// list, an editor) override Draw to paint Delta-aligned content.
type Scroller struct {
	Group

	HScrollBar *ScrollBar
	VScrollBar *ScrollBar
	Delta      geom.Point
	Limit      geom.Point
}

// NewScroller constructs an empty Scroller.
func NewScroller(bounds geom.Rect, h, v *ScrollBar) *Scroller {
	s := &Scroller{HScrollBar: h, VScrollBar: v}
	InitGroup(&s.Group, bounds)
	s.SetSelf(s)
	s.Options |= consts.OfSelectable
	return s
}

// GetTypeID for serial registry.
func (s *Scroller) GetTypeID() string { return "scroller" }

// SetLimit installs the maximum scroll extent.
func (s *Scroller) SetLimit(x, y int) {
	s.Limit = geom.Point{X: x, Y: y}
	if s.HScrollBar != nil {
		s.HScrollBar.SetRange(0, max(0, x-s.Size.X))
	}
	if s.VScrollBar != nil {
		s.VScrollBar.SetRange(0, max(0, y-s.Size.Y))
	}
}

// HandleEvent listens for scroll-bar changed broadcasts to update Delta.
func (s *Scroller) HandleEvent(ev *drivers.Event) {
	s.Group.HandleEvent(ev)
	if ev.What == consts.EvBroadcast && ev.Command == consts.CmScrollBarChanged {
		dx, dy := s.Delta.X, s.Delta.Y
		if s.HScrollBar != nil {
			dx = s.HScrollBar.Value
		}
		if s.VScrollBar != nil {
			dy = s.VScrollBar.Value
		}
		if dx != s.Delta.X || dy != s.Delta.Y {
			s.Delta = geom.Point{X: dx, Y: dy}
			s.Draw()
		}
	}
}

// ListViewer is the abstract single-column list. Concrete versions
// (ListBox, StringListBox) provide GetText.
type ListViewer struct {
	Group

	Range   int // total item count
	Focused int // currently focused row
	NumCols int // number of visible columns (for grid lists)
	HScroll *ScrollBar
	VScroll *ScrollBar
	GetText func(item int) string // populated by subclasses

	// SingleClickSelects, when true, fires cmListItemSelected on a
	// single click instead of waiting for a double-click. Useful for
	// directory lists in file dialogs and similar "pick instantly"
	// patterns.
	SingleClickSelects bool
}

// NewListViewer constructs a ListViewer. cols is the column layout; for
// a one-column list, pass 1.
func NewListViewer(bounds geom.Rect, cols int, h, v *ScrollBar) *ListViewer {
	if cols < 1 {
		cols = 1
	}
	l := &ListViewer{NumCols: cols, HScroll: h, VScroll: v}
	InitGroup(&l.Group, bounds)
	l.SetSelf(l)
	l.Options |= consts.OfSelectable | consts.OfFirstClick
	l.EventMask |= consts.EvBroadcast
	return l
}

// GetTypeID for serial registry.
func (l *ListViewer) GetTypeID() string { return "listviewer" }

// SetRange installs total item count and clamps Focused.
func (l *ListViewer) SetRange(n int) {
	l.Range = n
	if l.Focused >= n {
		l.Focused = n - 1
	}
	if l.Focused < 0 && n > 0 {
		l.Focused = 0
	}
	if l.VScroll != nil {
		rows := l.Size.Y
		if rows < 1 {
			rows = 1
		}
		l.VScroll.SetRange(0, max(0, n-rows))
	}
}

// FocusItem moves focus to item i and scrolls into view.
func (l *ListViewer) FocusItem(i int) {
	if i < 0 {
		i = 0
	}
	if i >= l.Range {
		i = l.Range - 1
	}
	l.Focused = i
	rows := l.Size.Y
	if rows < 1 {
		rows = 1
	}
	if l.VScroll != nil {
		if i < l.VScroll.Value {
			l.VScroll.SetValue(i)
		} else if i >= l.VScroll.Value+rows {
			l.VScroll.SetValue(i - rows + 1)
		}
	}
	l.Draw()
}

// Draw renders the visible items via GetText (if set).
func (l *ListViewer) Draw() {
	if l.GetText == nil {
		l.Group.Draw()
		return
	}
	rows := l.Size.Y
	cols := l.Size.X
	top := 0
	if l.VScroll != nil {
		top = l.VScroll.Value
	}
	for r := 0; r < rows; r++ {
		buf := screen.MakeDrawBuffer(cols)
		idx := top + r
		if idx < 0 || idx >= l.Range {
			color := types.MakeAttr(0x07, 0x00)
			for x := 0; x < cols; x++ {
				screen.DrawCell(buf, x, " ", color)
			}
		} else {
			color := types.MakeAttr(0x07, 0x00)
			if idx == l.Focused {
				color = types.MakeAttr(0x0F, 0x07)
			}
			for x := 0; x < cols; x++ {
				screen.DrawCell(buf, x, " ", color)
			}
			screen.DrawStr(buf, 1, l.GetText(idx), color)
		}
		l.WriteLine(0, r, cols, 1, buf)
	}
}

// HandleEvent: arrows / pageup / pagedown / home / end / Enter, plus
// click-to-focus, double-click to select, and mouse-wheel to scroll.
func (l *ListViewer) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		// Wheel events come through as mouse-down with the wheel
		// button bits set (consts.MbScrollWheelUp / Down). Translate
		// to focus movement without firing cmListItemSelected, even
		// when SingleClickSelects is on — otherwise scrolling a
		// directory list would navigate into whatever ends up under
		// the cursor.
		if ev.Buttons&(consts.MbScrollWheelUp|consts.MbScrollWheelDown) != 0 {
			step := 3
			if ev.Buttons&consts.MbScrollWheelUp != 0 {
				l.FocusItem(l.Focused - step)
			} else {
				l.FocusItem(l.Focused + step)
			}
			l.ClearEvent(ev)
			return
		}
		local := l.MakeLocal(ev.Where)
		top := 0
		if l.VScroll != nil {
			top = l.VScroll.Value
		}
		idx := top + local.Y
		if idx >= 0 && idx < l.Range {
			l.FocusItem(idx)
			if l.Owner != nil {
				l.Owner.Focus(l.self)
			}
			if ev.DoubleClk || l.SingleClickSelects {
				notify := drivers.Event{What: consts.EvBroadcast, Command: consts.CmListItemSelected, InfoPtr: l.Self()}
				l.PutEvent(&notify)
			}
			l.ClearEvent(ev)
		}
		return
	}
	if ev.What == consts.EvKeyDown {
		switch ev.KeyCode {
		case consts.KbUp:
			l.FocusItem(l.Focused - 1)
			l.ClearEvent(ev)
			return
		case consts.KbDown:
			l.FocusItem(l.Focused + 1)
			l.ClearEvent(ev)
			return
		case consts.KbPgUp:
			l.FocusItem(l.Focused - l.Size.Y)
			l.ClearEvent(ev)
			return
		case consts.KbPgDn:
			l.FocusItem(l.Focused + l.Size.Y)
			l.ClearEvent(ev)
			return
		case consts.KbHome:
			l.FocusItem(0)
			l.ClearEvent(ev)
			return
		case consts.KbEnd:
			l.FocusItem(l.Range - 1)
			l.ClearEvent(ev)
			return
		case consts.KbEnter:
			notify := drivers.Event{What: consts.EvBroadcast, Command: consts.CmListItemSelected, InfoPtr: l.Self()}
			l.PutEvent(&notify)
			l.ClearEvent(ev)
			return
		}
	}
	l.Group.HandleEvent(ev)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
