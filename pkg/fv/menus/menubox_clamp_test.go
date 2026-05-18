package menus

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// TestNestedSubmenuOriginFlipsAtRightEdge regression: a nested
// submenu opened from a parent menubox near the host's right edge
// used to anchor at parent.X+parent.Width-1 unconditionally, pushing
// the child past the screen. The backend silently drops out-of-bounds
// cells, so the user sees a partially-invisible or completely missing
// popup. The fix flips the child to the left of the parent when the
// natural placement would overflow.
func TestNestedSubmenuOriginFlipsAtRightEdge(t *testing.T) {
	host := geom.NewRect(0, 0, 80, 24)
	parentOrigin := geom.Point{X: 70, Y: 5}
	parentSize := geom.Point{X: 10, Y: 8}
	childSize := geom.Point{X: 16, Y: 6}

	got := nestedSubmenuOrigin(parentOrigin, parentSize, 0, childSize, host)
	// Natural: x = 70 + 10 - 1 = 79, child would run to x=94 (host
	// ends at 80). Flipped: x = 70 - 16 + 1 = 55.
	if got.X != 55 {
		t.Errorf("right-edge flip: X=%d, want 55", got.X)
	}
}

// TestNestedSubmenuOriginRaisesAtBottomEdge: vertical-overflow case
// raises the child so its bottom sits on the host's bottom edge.
func TestNestedSubmenuOriginRaisesAtBottomEdge(t *testing.T) {
	host := geom.NewRect(0, 0, 80, 24)
	parentOrigin := geom.Point{X: 10, Y: 18}
	parentSize := geom.Point{X: 10, Y: 5}
	childSize := geom.Point{X: 16, Y: 10}

	got := nestedSubmenuOrigin(parentOrigin, parentSize, 4, childSize, host)
	// Natural Y = 18 + 4 + 1 = 23. With height 10 that overflows.
	// Raised Y = host.B.Y - childSize.Y = 14.
	if got.Y != 14 {
		t.Errorf("bottom-edge raise: Y=%d, want 14", got.Y)
	}
}

// TestNestedSubmenuOriginNoClampWhenFits: the common case — child
// fits where it naturally lands. No clamp applied.
func TestNestedSubmenuOriginNoClampWhenFits(t *testing.T) {
	host := geom.NewRect(0, 0, 80, 24)
	parentOrigin := geom.Point{X: 10, Y: 5}
	parentSize := geom.Point{X: 10, Y: 5}
	childSize := geom.Point{X: 16, Y: 6}

	got := nestedSubmenuOrigin(parentOrigin, parentSize, 2, childSize, host)
	wantX := 10 + 10 - 1
	wantY := 5 + 2 + 1
	if got.X != wantX || got.Y != wantY {
		t.Errorf("no-clamp case: got (%d,%d), want (%d,%d)", got.X, got.Y, wantX, wantY)
	}
}
