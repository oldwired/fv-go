package views

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// With gfGrowRel the selected corners scale proportionally to the
// parent's size change rather than translating by the raw delta.
func TestCalcBoundsGrowRelScalesProportionally(t *testing.T) {
	b := NewBase(geom.NewRect(10, 0, 20, 10)) // x in [10,20]
	b.GrowMode = consts.GfGrowAll | consts.GfGrowRel

	// Owner width 40 → 80 (delta.X = 40); height unchanged at 10.
	r := b.CalcBounds(geom.Point{X: 40, Y: 0}, geom.Point{X: 80, Y: 10})

	// X scales by 80/40 = 2: [10,20] → [20,40]. Y unchanged.
	if r.A.X != 20 || r.B.X != 40 {
		t.Errorf("GrowRel X = [%d,%d], want [20,40]", r.A.X, r.B.X)
	}
	if r.A.Y != 0 || r.B.Y != 10 {
		t.Errorf("GrowRel Y = [%d,%d], want [0,10] (no vertical change)", r.A.Y, r.B.Y)
	}
}

// Without gfGrowRel, the directional bits still translate by the delta
// (regression guard for the absolute path after the signature change).
func TestCalcBoundsAbsoluteGrowUnchanged(t *testing.T) {
	b := NewBase(geom.NewRect(0, 0, 20, 10))
	b.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY

	r := b.CalcBounds(geom.Point{X: 5, Y: 3}, geom.Point{X: 45, Y: 13})

	if r.A.X != 0 || r.A.Y != 0 {
		t.Errorf("origin moved: A=%v, want (0,0)", r.A)
	}
	if r.B.X != 25 || r.B.Y != 13 {
		t.Errorf("far corner = (%d,%d), want (25,13)", r.B.X, r.B.Y)
	}
}
