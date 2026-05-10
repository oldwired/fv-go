package dialogs

import (
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
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

// History is a stub: in TV it's a popup of past entries linked to an
// InputLine. We keep the type so dialog descriptors compile, but the
// dropdown is left for follow-on work.
type History struct {
	views.Base
	Link      *InputLine
	HistoryID int
	Items     []string
}

// NewHistory builds a History glyph next to inp.
func NewHistory(bounds geom.Rect, inp *InputLine, id int) *History {
	h := &History{Base: views.NewBase(bounds), Link: inp, HistoryID: id}
	h.SetSelf(h)
	return h
}

// GetTypeID for serial registry.
func (h *History) GetTypeID() string { return "history" }
