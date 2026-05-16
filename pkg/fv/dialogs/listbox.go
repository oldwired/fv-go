package dialogs

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/history"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/popupmenu"
)

// ListBox is the abstract list view from Dialogs.pas. Concrete uses
// supply a slice of items and a getText function.
type ListBox struct {
	views.ListViewer
}

// NewListBox builds a ListBox.
func NewListBox(bounds geom.Rect, cols int, vScroll *views.ScrollBar) *ListBox {
	l := &ListBox{ListViewer: *views.NewListViewer(bounds, cols, nil, vScroll)}
	l.SetSelf(l)
	return l
}

// GetTypeID for serial registry.
func (l *ListBox) GetTypeID() string { return "listbox" }

// StringListBox is a ListBox backed by a slice of strings.
type StringListBox struct {
	ListBox
	Items []string
}

// NewStringListBox builds a StringListBox.
func NewStringListBox(bounds geom.Rect, vScroll *views.ScrollBar, items []string) *StringListBox {
	s := &StringListBox{ListBox: *NewListBox(bounds, 1, vScroll), Items: items}
	s.SetSelf(s)
	s.GetText = func(i int) string {
		if i < 0 || i >= len(s.Items) {
			return ""
		}
		return s.Items[i]
	}
	s.SetRange(len(items))
	return s
}

// GetTypeID for serial registry.
func (s *StringListBox) GetTypeID() string { return "stringlistbox" }

// SetItems replaces the contents and resets focus to 0.
func (s *StringListBox) SetItems(items []string) {
	s.Items = items
	s.SetRange(len(items))
	s.Focused = 0
	s.Draw()
}

// History is the dropdown-arrow affordance next to an InputLine.
// Clicking the glyph (or pressing Alt+Down on the linked input) opens
// a popup list of past entries from the history store; the selected
// entry replaces the linked InputLine's text.
//
// Items is overridden by the package-level history store when
// HistoryID != 0; callers can also set it directly for ad-hoc lists.
type History struct {
	views.Base
	Link      *InputLine
	HistoryID int
	Items     []string
}

// NewHistory builds a History glyph next to inp. Render the glyph in
// a one-cell-wide bounds; the popup positions itself underneath the
// linked InputLine on click.
func NewHistory(bounds geom.Rect, inp *InputLine, id int) *History {
	h := &History{Base: views.NewBase(bounds), Link: inp, HistoryID: id}
	h.SetSelf(h)
	h.Options |= consts.OfSelectable | consts.OfFirstClick
	return h
}

// GetTypeID for serial registry.
func (h *History) GetTypeID() string { return "history" }

// Draw renders the down-arrow affordance. Matches the ComboBox glyph
// (▾) so the UI vocabulary stays consistent.
func (h *History) Draw() {
	pal := theme.Get()
	attr := pal.HistoryArrow
	for y := 0; y < h.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(h.Size.X)
		for x := 0; x < h.Size.X; x++ {
			screen.DrawCell(buf, x, " ", attr)
		}
		if y == h.Size.Y/2 && h.Size.X >= 1 {
			screen.DrawCell(buf, 0, "▾", attr)
		}
		h.WriteLine(0, y, h.Size.X, 1, buf)
	}
}

// HandleEvent opens the popup on mouse click. Other events fall
// through to Base (which ignores them).
func (h *History) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		h.openPopup()
		h.ClearEvent(ev)
		return
	}
	h.Base.HandleEvent(ev)
}

// items returns the current candidate list. If HistoryID is non-zero
// we consult the package-level history store; otherwise the explicit
// Items slice wins.
func (h *History) items() []string {
	if h.HistoryID != 0 {
		return history.Get(byte(h.HistoryID))
	}
	return h.Items
}

// openPopup builds a PopupMenu under the linked InputLine and
// transfers the selection back when the user picks an entry. No-op
// when the history list is empty — clicking a dropdown with nothing
// to drop down would be confusing.
func (h *History) openPopup() {
	items := h.items()
	if len(items) == 0 || h.Owner == nil {
		return
	}
	host := topLevelGroup(h.Owner)
	if host == nil {
		return
	}
	// Anchor under the linked InputLine when available; fall back to
	// directly under the glyph for ad-hoc placements.
	var anchorX, anchorY int
	if h.Link != nil && h.Link.Owner != nil {
		ax, ay := h.Link.ScreenOrigin()
		anchorX, anchorY = ax, ay+h.Link.Size.Y
	} else {
		gx, gy := h.ScreenOrigin()
		anchorX, anchorY = gx, gy+1
	}
	hx, hy := host.ScreenOrigin()
	width := 24
	if h.Link != nil {
		width = h.Link.Size.X
	}
	pop := popupmenu.New(
		geom.Point{X: anchorX - hx, Y: anchorY - hy},
		items,
		width+8,
	)
	idx := pop.Run(host)
	if idx >= 0 && idx < len(items) && h.Link != nil {
		h.Link.SetText(items[idx])
	}
}

// topLevelGroup walks up from g to the outermost Group (the Program).
// PopupMenu.Run wants to insert into the program so it can paint
// over everything else.
func topLevelGroup(g *views.Group) *views.Group {
	for g != nil && g.Owner != nil {
		g = g.Owner
	}
	return g
}
