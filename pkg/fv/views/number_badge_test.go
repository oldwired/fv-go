package views_test

import (
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// renderWindow renders a window once into a fresh headless backend and
// returns the text snapshot. Used for badge-tracking tests so the
// per-window-number logic is observable without a full golden file.
func renderWindow(num int) string {
	h := term.NewHeadless(20, 6)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)
	w := views.NewWindow(geom.NewRect(0, 0, 18, 5), "x", num)
	w.State |= consts.SfExposed | consts.SfVisible
	w.State &^= consts.SfShadow
	w.Draw()
	_ = h.Flush()
	return h.Snapshot()
}

// TestWindowNumberBadgeSingleDigit covers the existing 1..9 path.
func TestWindowNumberBadgeSingleDigit(t *testing.T) {
	snap := renderWindow(7)
	if !strings.Contains(snap, " 7 ") {
		t.Errorf("expected ' 7 ' badge in snapshot, got:\n%s", snap)
	}
}

// TestWindowNumberBadgeDoubleDigit covers the new n > 9 path. The
// frame's badge slot is one column wide; rendering "10" would overrun
// into the zoom icon, so 10+ renders as a generic "+" marker.
func TestWindowNumberBadgeDoubleDigit(t *testing.T) {
	for _, n := range []int{10, 13, 99} {
		snap := renderWindow(n)
		if !strings.Contains(snap, " + ") {
			t.Errorf("n=%d: expected ' + ' badge, got:\n%s", n, snap)
		}
	}
}

// TestWindowNumberBadgeZero confirms n=0 still renders no badge.
func TestWindowNumberBadgeZero(t *testing.T) {
	snap := renderWindow(0)
	if strings.Contains(snap, " + ") || strings.Contains(snap, " 0 ") {
		t.Errorf("n=0 should render no badge, got:\n%s", snap)
	}
}
