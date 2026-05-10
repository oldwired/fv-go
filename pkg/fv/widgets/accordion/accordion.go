// Package accordion provides Accordion — a vertical stack of
// expandable / collapsible sections.
package accordion

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Section describes one accordion section: a clickable header row
// and a body view that's shown when the section is expanded.
type Section struct {
	Title    string
	Body     views.View
	BodyRows int // height when expanded
	Expanded bool
}

// Accordion is a Group of stacked sections. The Exclusive flag means
// at most one section can be expanded at a time.
type Accordion struct {
	views.Group

	Sections  []*Section
	Exclusive bool

	HeaderColor uint16
	OpenColor   uint16
}

// New constructs an empty Accordion.
func New(bounds geom.Rect) *Accordion {
	a := &Accordion{
		HeaderColor: types.MakeAttr(0x0F, 0x01),
		OpenColor:   types.MakeAttr(0x0E, 0x02),
	}
	views.InitGroup(&a.Group, bounds)
	a.SetSelf(a)
	a.Options |= consts.OfSelectable
	a.GrowMode = consts.GfGrowAll
	return a
}

// GetTypeID for serial registry.
func (a *Accordion) GetTypeID() string { return "accordion" }

// AddSection appends a section with the given title and body. The
// body's bounds are managed by the accordion; pass any rect.
func (a *Accordion) AddSection(title string, body views.View, bodyRows int) {
	s := &Section{Title: title, Body: body, BodyRows: bodyRows}
	a.Sections = append(a.Sections, s)
	if body != nil {
		a.Insert(body)
	}
	a.layout()
}

// Toggle expands or collapses the i-th section.
func (a *Accordion) Toggle(i int) {
	if i < 0 || i >= len(a.Sections) {
		return
	}
	if a.Exclusive {
		for j, s := range a.Sections {
			if j != i {
				s.Expanded = false
			}
		}
	}
	a.Sections[i].Expanded = !a.Sections[i].Expanded
	a.layout()
}

// layout positions every header and body inside the group.
func (a *Accordion) layout() {
	y := 0
	for _, s := range a.Sections {
		// Header always takes 1 row at y.
		// Body lives at y+1 with BodyRows height when expanded.
		if s.Body != nil {
			if s.Expanded && y+1 < a.Size.Y {
				h := s.BodyRows
				if y+1+h > a.Size.Y {
					h = a.Size.Y - (y + 1)
				}
				s.Body.BaseView().State |= consts.SfVisible
				s.Body.ChangeBounds(geom.NewRect(1, y+1, a.Size.X-1, y+1+h))
			} else {
				s.Body.BaseView().State &^= consts.SfVisible
			}
		}
		y++
		if s.Expanded {
			y += s.BodyRows
		}
		if y >= a.Size.Y {
			break
		}
	}
}

// Draw paints headers; bodies draw themselves via Group.Draw.
func (a *Accordion) Draw() {
	a.Group.Draw()
	y := 0
	for _, s := range a.Sections {
		if y >= a.Size.Y {
			break
		}
		buf := screen.MakeDrawBuffer(a.Size.X)
		c := a.HeaderColor
		marker := "▶"
		if s.Expanded {
			c = a.OpenColor
			marker = "▼"
		}
		for x := 0; x < a.Size.X; x++ {
			screen.DrawCell(buf, x, " ", c)
		}
		screen.DrawStr(buf, 0, " "+marker+" "+s.Title, c)
		a.WriteLine(0, y, a.Size.X, 1, buf)
		y++
		if s.Expanded {
			y += s.BodyRows
		}
	}
}

// HandleEvent routes header clicks into Toggle.
func (a *Accordion) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		local := a.MakeLocal(ev.Where)
		y := 0
		for i, s := range a.Sections {
			if local.Y == y {
				a.Toggle(i)
				a.ClearEvent(ev)
				return
			}
			y++
			if s.Expanded {
				y += s.BodyRows
			}
		}
	}
	a.Group.HandleEvent(ev)
}
