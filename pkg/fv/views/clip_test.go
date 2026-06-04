package views

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
)

// recordBackend is a minimal RootBackend that records the Ch written to
// each cell, so a test can assert where drawing landed.
type recordBackend struct{ cells map[[2]int]string }

func newRecordBackend() *recordBackend { return &recordBackend{cells: map[[2]int]string{}} }

func (r *recordBackend) SetCell(x, y int, c types.DrawCell) { r.cells[[2]int{x, y}] = c.Ch }
func (r *recordBackend) GetCell(x, y int) types.DrawCell {
	return types.DrawCell{Ch: r.cells[[2]int{x, y}]}
}
func (r *recordBackend) Flush() error                 { return nil }
func (r *recordBackend) WriteRaw(string) error        { return nil }
func (r *recordBackend) MarkClean(int, int)           {}
func (r *recordBackend) Invalidate(int, int)          {}
func (r *recordBackend) WasInvalidated(int, int) bool { return false }

// wideChild draws a row far wider than its window, so a clip is observable.
type wideChild struct{ Base }

func newWideChild(r geom.Rect) *wideChild {
	c := &wideChild{Base: NewBase(r)}
	c.SetSelf(c)
	c.State |= consts.SfVisible | consts.SfExposed
	return c
}

func (c *wideChild) Draw() {
	buf := screen.MakeDrawBuffer(40)
	for i := range buf {
		buf[i] = types.DrawCell{Ch: "X"}
	}
	c.WriteLine(0, 0, 40, 1, buf)
}

// TestWriteLineClipsToContainingWindow: a child's output must not escape
// the window that contains it, even when it draws past the window's
// edge. Before the clip, the 40-wide row spilled onto the desktop.
func TestWriteLineClipsToContainingWindow(t *testing.T) {
	rb := newRecordBackend()
	SetRootBackend(rb)
	defer SetRootBackend(nil)

	// Window occupies screen x[2,12), y[2,7).
	w := NewWindow(geom.NewRect(2, 2, 12, 7), "x", 0)
	w.State |= consts.SfVisible | consts.SfExposed
	w.Insert(newWideChild(geom.NewRect(1, 1, 9, 2)))

	w.Draw()

	if got := rb.cells[[2]int{5, 3}]; got != "X" {
		t.Errorf("child content missing inside the window at (5,3): got %q", got)
	}
	if got := rb.cells[[2]int{12, 3}]; got == "X" {
		t.Error("child spilled one cell past the window's right edge (12,3)")
	}
	if got := rb.cells[[2]int{20, 3}]; got == "X" {
		t.Error("child spilled well past the window onto the desktop (20,3)")
	}
}

// floorChild is a leaf used to exercise contentFloor; its GrowMode
// decides whether it pins the window's minimum size.
type floorChild struct{ Base }

func newFloorChild(r geom.Rect, growMode byte) *floorChild {
	c := &floorChild{Base: NewBase(r)}
	c.SetSelf(c)
	c.GrowMode = growMode
	return c
}

func (c *floorChild) Draw() {}

// TestClampResizeHonorsContentFloor: a fixed-offset child whose far edge
// sits at (30,14) must keep the window from shrinking past it (+1 for the
// far frame border).
func TestClampResizeHonorsContentFloor(t *testing.T) {
	w := NewWindow(geom.NewRect(0, 0, 40, 20), "x", 0)
	w.Insert(newFloorChild(geom.NewRect(2, 2, 30, 14), 0))

	gotW, gotH := clampResize(w, 5, 1)
	if gotW != 31 || gotH != 15 {
		t.Errorf("content floor: got (%d,%d), want (31,15)", gotW, gotH)
	}
}

// TestClampResizeIgnoresGrowableChild: a child that grows with the window
// (GfGrowHiX|GfGrowHiY) shrinks with it, so it must not pin any floor —
// the window falls through to the 16x4 hard floor.
func TestClampResizeIgnoresGrowableChild(t *testing.T) {
	w := NewWindow(geom.NewRect(0, 0, 40, 20), "x", 0)
	w.Insert(newFloorChild(geom.NewRect(1, 1, 39, 19), consts.GfGrowHiX|consts.GfGrowHiY))

	gotW, gotH := clampResize(w, 5, 1)
	if gotW != 16 || gotH != 4 {
		t.Errorf("growable child should not pin a floor: got (%d,%d), want (16,4)", gotW, gotH)
	}
}
