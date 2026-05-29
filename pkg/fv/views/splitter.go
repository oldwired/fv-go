package views

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
)

// SplitOrientation picks the axis the splitter sits on.
type SplitOrientation int

const (
	// SplitHorizontal: splitter is a horizontal bar; Panel1 sits above
	// Panel2.
	SplitHorizontal SplitOrientation = iota
	// SplitVertical: splitter is a vertical bar; Panel1 sits to the
	// left of Panel2.
	SplitVertical
)

// Splitter is a draggable bar that lives between two sibling views.
// Dragging it changes the split position and re-issues ChangeBounds on
// each panel. The splitter itself is one cell thick.
type Splitter struct {
	Base

	Orientation          SplitOrientation
	Panel1, Panel2       View
	MinPanel1, MinPanel2 int
}

// NewSplitter constructs a splitter between two pre-existing panels.
// Mininmums clamp how thin each side can shrink.
func NewSplitter(bounds geom.Rect, orientation SplitOrientation, p1, p2 View, minA, minB int) *Splitter {
	s := &Splitter{
		Base:        NewBase(bounds),
		Orientation: orientation,
		Panel1:      p1,
		Panel2:      p2,
		MinPanel1:   minA,
		MinPanel2:   minB,
	}
	s.SetSelf(s)
	s.Options |= consts.OfSelectable
	if orientation == SplitVertical {
		s.GrowMode = consts.GfGrowLoY | consts.GfGrowHiY
	} else {
		s.GrowMode = consts.GfGrowLoX | consts.GfGrowHiX
	}
	return s
}

// GetTypeID for serial registry.
func (s *Splitter) GetTypeID() string { return "splitter" }

// Draw paints the splitter bar with a small handle in the middle.
func (s *Splitter) Draw() {
	pal := theme.Get()
	color := pal.SplitterBar
	handle := pal.SplitterHandle
	if s.Orientation == SplitVertical {
		mid := s.Size.Y / 2
		for y := 0; y < s.Size.Y; y++ {
			ch := "│"
			attr := color
			if y == mid {
				ch = "║"
				attr = handle
			}
			row := screen.DrawBuffer{{Ch: ch, Attr: attr}}
			s.WriteLine(0, y, 1, 1, row)
		}
	} else {
		mid := s.Size.X / 2
		buf := screen.MakeDrawBuffer(s.Size.X)
		for x := 0; x < s.Size.X; x++ {
			ch := "─"
			attr := color
			if x == mid {
				ch = "═"
				attr = handle
			}
			screen.DrawCell(buf, x, ch, attr)
		}
		s.WriteLine(0, 0, s.Size.X, 1, buf)
	}
}

// HandleEvent runs a drag loop when the user clicks the splitter.
// Scroll-wheel ticks now arrive as EvMouseWheel (not EvMouseDown), so
// no per-site filter is needed — this handler simply ignores
// non-EvMouseDown events.
func (s *Splitter) HandleEvent(ev *drivers.Event) {
	if ev.What != consts.EvMouseDown {
		return
	}
	q := globalQueue.Load()
	if q == nil {
		return
	}
	prev := ev.Where
	for {
		if pumpFn != nil {
			pumpFn()
		}
		next, ok := q.Get()
		if !ok {
			if waitFn != nil {
				waitFn()
			}
			continue
		}
		switch next.What {
		case consts.EvMouseUp:
			s.ClearEvent(ev)
			return
		case consts.EvMouseMove, consts.EvMouseDown:
			var d int
			if s.Orientation == SplitVertical {
				d = next.Where.X - prev.X
			} else {
				d = next.Where.Y - prev.Y
			}
			if d != 0 {
				s.move(d)
				prev = next.Where
			}
		}
	}
}

// move shifts the splitter by d cells along its axis, respecting the
// per-panel minimums.
func (s *Splitter) move(d int) {
	if s.Panel1 == nil || s.Panel2 == nil {
		return
	}
	p1 := s.Panel1.BaseView()
	p2 := s.Panel2.BaseView()
	if s.Orientation == SplitVertical {
		newWidth1 := p1.Size.X + d
		newWidth2 := p2.Size.X - d
		if newWidth1 < s.MinPanel1 || newWidth2 < s.MinPanel2 {
			return
		}
		p1.Self().ChangeBounds(geom.NewRect(p1.Origin.X, p1.Origin.Y, p1.Origin.X+newWidth1, p1.Origin.Y+p1.Size.Y))
		s.MoveTo(s.Origin.X+d, s.Origin.Y)
		p2.Self().ChangeBounds(geom.NewRect(p2.Origin.X+d, p2.Origin.Y, p2.Origin.X+d+newWidth2, p2.Origin.Y+p2.Size.Y))
	} else {
		newHeight1 := p1.Size.Y + d
		newHeight2 := p2.Size.Y - d
		if newHeight1 < s.MinPanel1 || newHeight2 < s.MinPanel2 {
			return
		}
		p1.Self().ChangeBounds(geom.NewRect(p1.Origin.X, p1.Origin.Y, p1.Origin.X+p1.Size.X, p1.Origin.Y+newHeight1))
		s.MoveTo(s.Origin.X, s.Origin.Y+d)
		p2.Self().ChangeBounds(geom.NewRect(p2.Origin.X, p2.Origin.Y+d, p2.Origin.X+p2.Size.X, p2.Origin.Y+d+newHeight2))
	}
}

// SplitGroup is a convenience container that owns Panel1, a Splitter,
// and Panel2 with automatic layout. RecalcLayout repositions all three
// when the group's bounds change.
type SplitGroup struct {
	Group

	Orientation SplitOrientation
	SplitPos    int // distance from the left/top edge to the splitter
	Splitter    *Splitter
	Panel1      View
	Panel2      View
}

// NewSplitGroup builds an empty split group; SetPanels installs the
// children.
func NewSplitGroup(bounds geom.Rect, orientation SplitOrientation, splitPos int) *SplitGroup {
	g := &SplitGroup{Orientation: orientation, SplitPos: splitPos}
	InitGroup(&g.Group, bounds)
	g.SetSelf(g)
	g.GrowMode = consts.GfGrowAll
	return g
}

// GetTypeID for serial registry.
func (g *SplitGroup) GetTypeID() string { return "splitgroup" }

// SetPanels installs the two panel views and creates the splitter.
// The panels' bounds are ignored — SplitGroup positions them.
func (g *SplitGroup) SetPanels(p1, p2 View) {
	g.Panel1 = p1
	g.Panel2 = p2
	g.recalc()
	g.Insert(p1)
	g.Splitter = NewSplitter(geom.Rect{}, g.Orientation, p1, p2, 4, 4)
	g.Insert(g.Splitter)
	g.Insert(p2)
	g.recalc()
}

// ChangeBounds re-lays out children when the container resizes.
func (g *SplitGroup) ChangeBounds(r geom.Rect) {
	g.Group.ChangeBounds(r)
	g.recalc()
}

// GetRatio returns SplitPos as a fraction of the group's size on the
// split axis. Useful for persisting layouts across resizes — restore
// with SetRatio.
func (g *SplitGroup) GetRatio() float64 {
	total := g.Size.X
	if g.Orientation == SplitHorizontal {
		total = g.Size.Y
	}
	if total <= 0 {
		return 0
	}
	return float64(g.SplitPos) / float64(total)
}

// SetRatio places the splitter at r * total (clamped to [0,1]) and
// re-lays out. Drives fvmux's keyboard resize mode (Ctrl-G R H/J/K/L)
// without simulating mouse drags.
func (g *SplitGroup) SetRatio(r float64) {
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	total := g.Size.X
	if g.Orientation == SplitHorizontal {
		total = g.Size.Y
	}
	g.SplitPos = int(r * float64(total))
	g.recalc()
	MarkDirty()
}

// SetMinPanel updates the per-panel minimum sizes on the Splitter and
// re-lays out so the new constraints take effect immediately.
func (g *SplitGroup) SetMinPanel(min1, min2 int) {
	if g.Splitter == nil {
		return
	}
	g.Splitter.MinPanel1 = min1
	g.Splitter.MinPanel2 = min2
	g.recalc()
}

// recalc positions the panels + splitter based on SplitPos.
func (g *SplitGroup) recalc() {
	if g.Panel1 == nil || g.Panel2 == nil {
		return
	}
	w, h := g.Size.X, g.Size.Y
	if g.Orientation == SplitVertical {
		if w < 3 {
			// Too narrow for Panel1 + splitter + Panel2. Give Panel1 the
			// width and collapse the splitter/Panel2 to zero width at the
			// right edge — clamping SplitPos here would still yield a
			// negative Panel2 rect.
			if w < 0 {
				w = 0
			}
			g.Panel1.ChangeBounds(geom.NewRect(0, 0, w, h))
			if g.Splitter != nil {
				g.Splitter.ChangeBounds(geom.NewRect(w, 0, w, h))
			}
			g.Panel2.ChangeBounds(geom.NewRect(w, 0, w, h))
			return
		}
		if g.SplitPos < 1 {
			g.SplitPos = 1
		}
		if g.SplitPos > w-2 {
			g.SplitPos = w - 2
		}
		g.Panel1.ChangeBounds(geom.NewRect(0, 0, g.SplitPos, h))
		if g.Splitter != nil {
			g.Splitter.ChangeBounds(geom.NewRect(g.SplitPos, 0, g.SplitPos+1, h))
		}
		g.Panel2.ChangeBounds(geom.NewRect(g.SplitPos+1, 0, w, h))
	} else {
		if h < 3 {
			if h < 0 {
				h = 0
			}
			g.Panel1.ChangeBounds(geom.NewRect(0, 0, w, h))
			if g.Splitter != nil {
				g.Splitter.ChangeBounds(geom.NewRect(0, h, w, h))
			}
			g.Panel2.ChangeBounds(geom.NewRect(0, h, w, h))
			return
		}
		if g.SplitPos < 1 {
			g.SplitPos = 1
		}
		if g.SplitPos > h-2 {
			g.SplitPos = h - 2
		}
		g.Panel1.ChangeBounds(geom.NewRect(0, 0, w, g.SplitPos))
		if g.Splitter != nil {
			g.Splitter.ChangeBounds(geom.NewRect(0, g.SplitPos, w, g.SplitPos+1))
		}
		g.Panel2.ChangeBounds(geom.NewRect(0, g.SplitPos+1, w, h))
	}
}
