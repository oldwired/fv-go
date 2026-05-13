// Package tooltip provides Tooltip — a small focus-driven hint that
// appears after a delay over the focused view, reading its TipText if
// it implements the Tipper interface.
package tooltip

import (
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Tipper is implemented by views that have hover help text.
type Tipper interface {
	TipText() string
}

// Tooltip is a single-line popup positioned next to the focused view.
type Tooltip struct {
	views.Base

	Host  *views.Group
	Delay time.Duration
	Color uint16

	armed time.Time
	last  string
}

// New constructs a Tooltip pinned to host (typically the Desktop).
// The tooltip is initially invisible; it appears when the focused
// child of the program tree advertises a TipText() and the user has
// kept it focused for at least Delay.
func New(host *views.Group) *Tooltip {
	t := &Tooltip{
		Base:  views.NewBase(geom.Rect{}),
		Host:  host,
		Delay: 700 * time.Millisecond,
		Color: theme.Get().TooltipNormal,
	}
	t.SetSelf(t)
	t.State &^= 0
	t.State |= 0
	anim.Register(t, 200*time.Millisecond)
	return t
}

// GetTypeID for serial registry.
func (t *Tooltip) GetTypeID() string { return "tooltip" }

// Tick is the polling callback: find the focused tipper, manage the
// delay, set/clear our visibility and bounds.
func (t *Tooltip) Tick(now time.Time) bool {
	tip, where := findFocusedTip(t.Host)
	if tip == "" {
		if t.last != "" {
			t.last = ""
			t.Hide()
			return true
		}
		t.armed = time.Time{}
		return false
	}
	if tip != t.last {
		t.last = tip
		t.armed = now
		t.Hide()
		return true
	}
	if !t.armed.IsZero() && now.Sub(t.armed) >= t.Delay {
		// Promote to visible.
		w := len(tip) + 2
		t.Origin = geom.Point{X: where.X, Y: where.Y + 1}
		t.Size = geom.Point{X: w, Y: 1}
		t.Show()
		return true
	}
	return false
}

// Draw paints the tooltip if visible.
func (t *Tooltip) Draw() {
	if t.Size.X <= 0 || t.last == "" {
		return
	}
	buf := screen.MakeDrawBuffer(t.Size.X)
	for x := 0; x < t.Size.X; x++ {
		screen.DrawCell(buf, x, " ", t.Color)
	}
	screen.DrawStr(buf, 1, t.last, t.Color)
	t.WriteLine(0, 0, t.Size.X, 1, buf)
}

// findFocusedTip walks the program tree, returning the focused leaf's
// TipText (if any) and a screen-space anchor for the tooltip.
func findFocusedTip(g *views.Group) (string, geom.Point) {
	var cur views.View = g
	for {
		gg, ok := cur.(*views.Group)
		if !ok {
			break
		}
		next := gg.Current()
		if next == nil {
			break
		}
		cur = next
	}
	if t, ok := cur.(Tipper); ok {
		text := t.TipText()
		if text != "" {
			x, y := cur.BaseView().ScreenOrigin()
			return text, geom.Point{X: x, Y: y}
		}
	}
	return "", geom.Point{}
}
