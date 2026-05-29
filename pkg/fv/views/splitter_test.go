package views

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// TestSplitGroupRatio round-trips GetRatio / SetRatio via a vertical
// splitter and verifies SetMinPanel propagates to the embedded
// Splitter.
func TestSplitGroupRatio(t *testing.T) {
	g := NewSplitGroup(geom.NewRect(0, 0, 100, 40), SplitVertical, 50)
	left := newDummy(geom.Rect{})
	right := newDummy(geom.Rect{})
	g.SetPanels(left, right)

	// At construction, ratio should reflect SplitPos / Size.X = 50/100 = 0.5.
	if r := g.GetRatio(); r < 0.49 || r > 0.51 {
		t.Errorf("GetRatio = %v, want ~0.5", r)
	}

	// SetRatio(0.25) → SplitPos = 25.
	g.SetRatio(0.25)
	if g.SplitPos != 25 {
		t.Errorf("after SetRatio(0.25), SplitPos = %d, want 25", g.SplitPos)
	}

	// Clamping: SetRatio(2.0) clamps to 1.0 → SplitPos = Size.X, then
	// recalc() clamps to Size.X - 2 = 98.
	g.SetRatio(2.0)
	if g.SplitPos != 98 {
		t.Errorf("after SetRatio(2.0), SplitPos = %d, want 98 (clamped)", g.SplitPos)
	}

	// SetMinPanel updates the underlying Splitter.
	g.SetMinPanel(10, 20)
	if g.Splitter == nil {
		t.Fatal("Splitter is nil after SetPanels")
	}
	if g.Splitter.MinPanel1 != 10 || g.Splitter.MinPanel2 != 20 {
		t.Errorf("SetMinPanel did not propagate: got (%d, %d), want (10, 20)",
			g.Splitter.MinPanel1, g.Splitter.MinPanel2)
	}
}

// TestSplitGroupDegenerateSizeNoNegativePanels: at sizes too small to
// hold both panels plus the splitter, recalc must not produce a negative
// panel rect (which propagated junk Size.X/Y into child layout).
func TestSplitGroupDegenerateSizeNoNegativePanels(t *testing.T) {
	for _, w := range []int{1, 2, 3} {
		g := NewSplitGroup(geom.NewRect(0, 0, w, 10), SplitVertical, 1)
		p1, p2 := newDummy(geom.Rect{}), newDummy(geom.Rect{})
		g.SetPanels(p1, p2)
		if p1.Size.X < 0 || p2.Size.X < 0 {
			t.Errorf("vertical w=%d: negative panel width p1=%d p2=%d", w, p1.Size.X, p2.Size.X)
		}
	}
	for _, h := range []int{1, 2, 3} {
		g := NewSplitGroup(geom.NewRect(0, 0, 10, h), SplitHorizontal, 1)
		p1, p2 := newDummy(geom.Rect{}), newDummy(geom.Rect{})
		g.SetPanels(p1, p2)
		if p1.Size.Y < 0 || p2.Size.Y < 0 {
			t.Errorf("horizontal h=%d: negative panel height p1=%d p2=%d", h, p1.Size.Y, p2.Size.Y)
		}
	}
}

// dummy is a minimal View for use in splitter tests.
type dummy struct{ Base }

func newDummy(b geom.Rect) *dummy {
	d := &dummy{Base: NewBase(b)}
	d.SetSelf(d)
	return d
}

func (d *dummy) GetTypeID() string { return "dummy" }
