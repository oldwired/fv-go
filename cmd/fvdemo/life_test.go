package main

import "testing"

// snapshot of live coordinates for stable comparison.
func liveSet(b *board) map[[2]int]bool {
	m := map[[2]int]bool{}
	for y := 0; y < b.h; y++ {
		for x := 0; x < b.w; x++ {
			if b.at(x, y) {
				m[[2]int{x, y}] = true
			}
		}
	}
	return m
}

func setCells(b *board, coords [][2]int) {
	b.clear()
	for _, c := range coords {
		b.set(c[0], c[1], true)
	}
}

func TestBlinkerOscillates(t *testing.T) {
	for _, mode := range []boundaryMode{bmWrap, bmDead, bmDeflect} {
		b := newBoard(9, 9, mode)
		// horizontal blinker centered well away from edges
		setCells(b, [][2]int{{3, 4}, {4, 4}, {5, 4}})
		start := liveSet(b)
		b.step()
		vertical := liveSet(b)
		wantVert := map[[2]int]bool{{4, 3}: true, {4, 4}: true, {4, 5}: true}
		if !sameSet(vertical, wantVert) {
			t.Fatalf("mode %v: blinker did not become vertical: %v", mode, vertical)
		}
		b.step()
		if !sameSet(liveSet(b), start) {
			t.Fatalf("mode %v: blinker did not return to start after period 2", mode)
		}
	}
}

func TestBlockIsStill(t *testing.T) {
	for _, mode := range []boundaryMode{bmWrap, bmDead, bmDeflect, bmEliminate} {
		b := newBoard(10, 10, mode)
		block := [][2]int{{4, 4}, {5, 4}, {4, 5}, {5, 5}}
		setCells(b, block)
		b.step()
		if !sameSet(liveSet(b), map[[2]int]bool{{4, 4}: true, {5, 4}: true, {4, 5}: true, {5, 5}: true}) {
			t.Fatalf("mode %v: 2x2 block was not still: %v", mode, liveSet(b))
		}
	}
}

func TestWrapReentersGlider(t *testing.T) {
	// A blinker straddling the wrap seam stays alive under Wrap but dies
	// under Dead. Place a vertical blinker across the top edge (y = h-1,
	// 0, 1) so wrap is required to keep all three neighbors.
	wrap := newBoard(7, 7, bmWrap)
	setCells(wrap, [][2]int{{3, 6}, {3, 0}, {3, 1}})
	wrap.step()
	if len(liveSet(wrap)) == 0 {
		t.Fatalf("wrap: seam-straddling blinker died, expected survival")
	}

	dead := newBoard(7, 7, bmDead)
	setCells(dead, [][2]int{{3, 6}, {3, 0}, {3, 1}})
	dead.step()
	// Under Dead the column ends are isolated; the pattern should not
	// reproduce a wrapped row.
	if dead.at(3, 6) && dead.at(3, 0) && dead.at(3, 1) {
		t.Fatalf("dead: pattern unexpectedly behaved like wrap")
	}
}

func TestDeflectNeighborEqualsEdge(t *testing.T) {
	b := newBoard(5, 5, bmDeflect)
	b.set(0, 0, true)
	// For an off-grid neighbor of (0,0), e.g. (-1,0), Deflect clamps to
	// (0,0), so the live corner cell counts as its own neighbor.
	if !b.neighborAlive(-1, 0) {
		t.Fatalf("deflect: off-grid (-1,0) should reflect live edge cell (0,0)")
	}
	b.set(0, 0, false)
	if b.neighborAlive(-1, 0) {
		t.Fatalf("deflect: off-grid (-1,0) should reflect dead edge cell")
	}
}

func TestEliminateKillsBorderPattern(t *testing.T) {
	b := newBoard(12, 12, bmEliminate)
	// A 2x2 block flush against the left/top border (a still life) must
	// be wiped because it touches the border.
	border := [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
	// A 2x2 block in the interior must survive untouched.
	interior := [][2]int{{6, 6}, {7, 6}, {6, 7}, {7, 7}}
	setCells(b, append(append([][2]int{}, border...), interior...))
	b.step()
	for _, c := range border {
		if b.at(c[0], c[1]) {
			t.Fatalf("eliminate: border cell %v survived", c)
		}
	}
	for _, c := range interior {
		if !b.at(c[0], c[1]) {
			t.Fatalf("eliminate: interior cell %v was wrongly removed", c)
		}
	}
}

func TestResizePreservesOverlap(t *testing.T) {
	b := newBoard(6, 6, bmDead)
	setCells(b, [][2]int{{0, 0}, {2, 3}, {5, 5}})
	b.resize(4, 4)
	if !b.at(0, 0) || !b.at(2, 3) {
		t.Fatalf("resize: lost cells inside the overlap region")
	}
	if b.w != 4 || b.h != 4 {
		t.Fatalf("resize: dims = %dx%d, want 4x4", b.w, b.h)
	}
	// (5,5) was outside the new bounds and must be dropped.
	if b.at(5, 5) {
		t.Fatalf("resize: out-of-range cell unexpectedly present")
	}
}

func sameSet(a, b map[[2]int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
