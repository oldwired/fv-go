package views

import (
	"testing"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// TestWindowOnCloseFiresBeforeDelete confirms two things at once:
//
//  1. OnClose runs when Close() is invoked on a non-modal window.
//  2. It runs BEFORE the parent's Children list drops the window —
//     so a host walking the subtree from inside OnClose still sees
//     descendants (Terminals, sub-windows, …) and can clean them up.
//
// This is the closure of the "host bookkeeping lingers until something
// trips over Owner == nil" gap that fvmux currently papers over with
// polling.
func TestWindowOnCloseFiresBeforeDelete(t *testing.T) {
	parent := &Group{}
	InitGroup(parent, geom.NewRect(0, 0, 80, 24))
	parent.SetSelf(parent)

	w := NewWindow(geom.NewRect(2, 2, 30, 12), "child", 0)
	parent.Insert(w)

	var fired bool
	var stillAttached bool
	var childrenCountAtFire int
	w.OnClose = func() {
		fired = true
		stillAttached = w.Owner != nil
		// w should still be in parent.Children at this moment.
		if w.Owner != nil {
			childrenCountAtFire = len(w.Owner.Children)
		}
	}
	w.Close()

	if !fired {
		t.Fatal("OnClose did not fire")
	}
	if !stillAttached {
		t.Error("OnClose fired AFTER Owner was nilled — host can't walk descendants")
	}
	if childrenCountAtFire == 0 {
		t.Error("window was already removed from parent.Children when OnClose fired")
	}
	// And the actual removal still happened after the callback returned.
	for _, c := range parent.Children {
		if c == w.self {
			t.Error("window not removed from parent after Close()")
		}
	}
}

// TestFlashTitleBarFlagging confirms the flash window opens for the
// requested duration and closes once it elapses.
func TestFlashTitleBarFlagging(t *testing.T) {
	w := NewWindow(geom.NewRect(0, 0, 20, 5), "x", 0)
	if w.Flashing() {
		t.Fatal("Flashing() should be false initially")
	}
	w.FlashTitleBar(40 * time.Millisecond)
	if !w.Flashing() {
		t.Error("Flashing() not set after FlashTitleBar")
	}
	// Wait past the flash window.
	time.Sleep(80 * time.Millisecond)
	if w.Flashing() {
		t.Error("Flashing() did not clear after duration elapsed")
	}
}

// TestFlashTitleBarDebounceDropsSpam: a second FlashTitleBar within
// 500ms of the first is dropped wholesale — no extension, no
// re-flash. This is the "BEL spam must not strobe the chrome" rule.
func TestFlashTitleBarDebounceDropsSpam(t *testing.T) {
	w := NewWindow(geom.NewRect(0, 0, 20, 5), "x", 0)
	w.FlashTitleBar(40 * time.Millisecond)
	firstUntil := w.flashUntil
	// Tight follow-up: same call, well under 500ms since the first.
	w.FlashTitleBar(1 * time.Second)
	if !w.flashUntil.Equal(firstUntil) {
		t.Errorf("debounced call mutated flashUntil: was %v, now %v",
			firstUntil, w.flashUntil)
	}
}

// TestWindowOnCloseFiresOnModalClose covers the modal branch — Close()
// on a modal window goes through EndModal, but OnClose must still
// fire so the host's bookkeeping runs.
func TestWindowOnCloseFiresOnModalClose(t *testing.T) {
	w := NewWindow(geom.NewRect(0, 0, 30, 10), "modal", 0)
	// EndModal sets SfModal as a side effect of marking the loop
	// terminator; clear the captured cmd so future inspection of
	// endState isn't misleading.
	w.EndModal(0)
	w.ClearEndState()

	var fired bool
	w.OnClose = func() { fired = true }
	w.Close()
	if !fired {
		t.Error("OnClose did not fire for modal-window Close()")
	}
}

// TestClampResizeHardFloor regression for the fvmux bug: a vanilla
// Window with no SizeLimits override must still get the (16, 4) floor
// so the user can't drag the corner past the point where children
// would have negative widths.
func TestClampResizeHardFloor(t *testing.T) {
	w := NewWindow(geom.NewRect(0, 0, 40, 20), "x", 0)
	gotW, gotH := clampResize(w, 5, 1)
	if gotW != 16 || gotH != 4 {
		t.Errorf("hard floor: got (%d, %d), want (16, 4)", gotW, gotH)
	}
}

// TestClampResizeAbovePreservedRequest confirms the clamp is a floor,
// not a snap: a request larger than the floor passes through.
func TestClampResizeAbovePreservedRequest(t *testing.T) {
	w := NewWindow(geom.NewRect(0, 0, 40, 20), "x", 0)
	gotW, gotH := clampResize(w, 50, 25)
	if gotW != 50 || gotH != 25 {
		t.Errorf("above-floor request: got (%d, %d), want (50, 25)", gotW, gotH)
	}
}

// resizableTestWindow is a Window subclass that overrides SizeLimits
// — the pattern fvmux's SFTP browser dialog uses. The clamp must
// honor the override even when it's larger than the (16, 4) baseline.
type resizableTestWindow struct {
	Window
	minW, minH int
}

func newResizableTestWindow(min geom.Point) *resizableTestWindow {
	w := &resizableTestWindow{minW: min.X, minH: min.Y}
	InitWindow(&w.Window, geom.NewRect(0, 0, 80, 24), "x", 0)
	w.SetSelf(w)
	return w
}

func (w *resizableTestWindow) SizeLimits() (geom.Point, geom.Point) {
	return geom.Point{X: w.minW, Y: w.minH}, geom.Point{X: 1 << 14, Y: 1 << 14}
}

// TestClampResizeHonorsSizeLimitsOverride: a Window that overrides
// SizeLimits to (60, 12) must not shrink below those dimensions.
// This is the actual fvmux ask: dialogs that contain fixed-bound
// children declare their minimum, and the framework respects it.
func TestClampResizeHonorsSizeLimitsOverride(t *testing.T) {
	w := newResizableTestWindow(geom.Point{X: 60, Y: 12})
	gotW, gotH := clampResize(&w.Window, 20, 5)
	if gotW != 60 || gotH != 12 {
		t.Errorf("SizeLimits override: got (%d, %d), want (60, 12)", gotW, gotH)
	}
	// A request between the framework floor and the override should
	// still clamp to the override.
	gotW, gotH = clampResize(&w.Window, 40, 8)
	if gotW != 60 || gotH != 12 {
		t.Errorf("between-floor-and-override: got (%d, %d), want (60, 12)", gotW, gotH)
	}
	// And a generous request passes through.
	gotW, gotH = clampResize(&w.Window, 100, 30)
	if gotW != 100 || gotH != 30 {
		t.Errorf("above-override request: got (%d, %d), want (100, 30)", gotW, gotH)
	}
}
