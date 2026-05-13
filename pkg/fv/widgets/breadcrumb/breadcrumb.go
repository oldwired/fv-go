// Package breadcrumb provides Breadcrumb — a clickable path-style
// navigation element ("Home › Apps › Settings ›").
package breadcrumb

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Segment is one clickable label.
type Segment struct {
	Label string
	Data  any
}

// Breadcrumb paints a row of segments separated by chevrons. Clicking
// a segment broadcasts cmBreadcrumbSelect with InfoInt = segment index.
type Breadcrumb struct {
	views.Base

	Segments  []Segment
	Separator string

	Color, ActiveColor uint16
}

// New constructs a breadcrumb.
func New(bounds geom.Rect, segments []Segment) *Breadcrumb {
	b := &Breadcrumb{
		Base:        views.NewBase(bounds),
		Segments:    segments,
		Separator:   " › ",
		Color:       theme.Get().TreeNormal,
		ActiveColor: theme.Get().StatHeader,
	}
	b.SetSelf(b)
	b.GrowMode = consts.GfGrowHiX
	return b
}

// GetTypeID for serial registry.
func (b *Breadcrumb) GetTypeID() string { return "breadcrumb" }

// SetSegments replaces the path.
func (b *Breadcrumb) SetSegments(s []Segment) { b.Segments = s }

// Draw paints "A › B › C".
func (b *Breadcrumb) Draw() {
	w := b.Size.X
	buf := screen.MakeDrawBuffer(w)
	for x := 0; x < w; x++ {
		screen.DrawCell(buf, x, " ", b.Color)
	}
	x := 0
	for i, seg := range b.Segments {
		c := b.Color
		if i == len(b.Segments)-1 {
			c = b.ActiveColor
		}
		screen.DrawStr(buf, x, seg.Label, c)
		x += utf8.StringDisplayWidth(seg.Label)
		if i < len(b.Segments)-1 {
			screen.DrawStr(buf, x, b.Separator, b.Color)
			x += utf8.StringDisplayWidth(b.Separator)
		}
		if x >= w {
			break
		}
	}
	b.WriteLine(0, 0, w, 1, buf)
}

// HandleEvent: click on a segment fires a broadcast.
func (b *Breadcrumb) HandleEvent(ev *drivers.Event) {
	if ev.What != consts.EvMouseDown {
		return
	}
	local := b.MakeLocal(ev.Where)
	if local.Y != 0 {
		return
	}
	x := 0
	for i, seg := range b.Segments {
		w := utf8.StringDisplayWidth(seg.Label)
		if local.X >= x && local.X < x+w {
			notify := drivers.Event{
				What:    consts.EvBroadcast,
				Command: 710, // cmBreadcrumbSelect
				InfoInt: int16(i),
			}
			b.PutEvent(&notify)
			b.ClearEvent(ev)
			return
		}
		x += w + utf8.StringDisplayWidth(b.Separator)
	}
}
