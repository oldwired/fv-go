package views

import (
	"testing"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
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

func TestWindowAllowedCloseDetachesAndNotifies(t *testing.T) {
	parent := NewGroup(geom.NewRect(0, 0, 80, 24))
	w := NewWindow(geom.NewRect(2, 2, 30, 12), "child", 0)
	parent.Insert(w)
	requests := 0
	closes := 0
	w.OnCloseRequest = func() bool {
		requests++
		return true
	}
	w.OnClose = func() { closes++ }

	w.Close()

	if requests != 1 {
		t.Errorf("OnCloseRequest calls = %d, want 1", requests)
	}
	if closes != 1 {
		t.Errorf("OnClose calls = %d, want 1", closes)
	}
	if w.Owner != nil {
		t.Error("allowed close left window attached")
	}
}

func TestWindowVetoedCloseStaysAttached(t *testing.T) {
	parent := NewGroup(geom.NewRect(0, 0, 80, 24))
	w := NewWindow(geom.NewRect(2, 2, 30, 12), "child", 0)
	parent.Insert(w)
	closes := 0
	w.OnCloseRequest = func() bool { return false }
	w.OnClose = func() { closes++ }

	w.Close()

	if w.Owner != parent {
		t.Error("vetoed close detached window")
	}
	if closes != 0 {
		t.Errorf("OnClose calls = %d, want 0", closes)
	}
}

func TestWindowNilCloseRequestPreservesBehavior(t *testing.T) {
	parent := NewGroup(geom.NewRect(0, 0, 80, 24))
	w := NewWindow(geom.NewRect(2, 2, 30, 12), "child", 0)
	parent.Insert(w)
	closes := 0
	w.OnClose = func() { closes++ }

	w.Close()

	if closes != 1 {
		t.Errorf("OnClose calls = %d, want 1", closes)
	}
	if w.Owner != nil {
		t.Error("nil OnCloseRequest left window attached")
	}
}

func TestWindowInteractiveClosePathsHonorVeto(t *testing.T) {
	tests := []struct {
		name  string
		close func(*Window, *drivers.Event)
	}{
		{
			name: "frame close box",
			close: func(w *Window, ev *drivers.Event) {
				ev.What = consts.EvMouseDown
				ev.Where = geom.Point{X: w.Origin.X + 2, Y: w.Origin.Y}
				w.HandleEvent(ev)
			},
		},
		{
			name: "CmClose",
			close: func(w *Window, ev *drivers.Event) {
				ev.What = consts.EvCommand
				ev.Command = consts.CmClose
				w.HandleEvent(ev)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := NewGroup(geom.NewRect(0, 0, 80, 24))
			w := NewWindow(geom.NewRect(2, 2, 30, 12), "child", 0)
			parent.Insert(w)
			requests := 0
			closes := 0
			w.OnCloseRequest = func() bool {
				requests++
				return false
			}
			w.OnClose = func() { closes++ }
			ev := &drivers.Event{}

			tt.close(w, ev)

			if requests != 1 {
				t.Errorf("OnCloseRequest calls = %d, want 1", requests)
			}
			if closes != 0 {
				t.Errorf("OnClose calls = %d, want 0", closes)
			}
			if w.Owner != parent {
				t.Error("vetoed interactive close detached window")
			}
			if ev.What != consts.EvNothing {
				t.Errorf("close event was not consumed: What = %#x", ev.What)
			}
		})
	}
}

func TestWindowModalCloseHonorsRequest(t *testing.T) {
	parent := NewGroup(geom.NewRect(0, 0, 80, 24))
	w := NewWindow(geom.NewRect(2, 2, 30, 12), "modal", 0)
	parent.Insert(w)
	w.EndModal(0)
	w.ClearEndState()
	allow := false
	closes := 0
	w.OnCloseRequest = func() bool { return allow }
	w.OnClose = func() { closes++ }

	w.Close()

	if w.EndStateValue() != 0 {
		t.Errorf("vetoed modal close set end state to %#x", w.EndStateValue())
	}
	if closes != 0 {
		t.Errorf("OnClose calls after veto = %d, want 0", closes)
	}
	if w.Owner != parent {
		t.Error("vetoed modal close detached window")
	}

	allow = true
	w.Close()

	if w.EndStateValue() != consts.CmCancel {
		t.Errorf("allowed modal close end state = %#x, want CmCancel", w.EndStateValue())
	}
	if closes != 1 {
		t.Errorf("OnClose calls after allowed close = %d, want 1", closes)
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

// TestSetSizeLimitsHonoredByClampResize: callers that just want a
// minimum without subclassing can use SetSizeLimits and the resize
// drag honors it the same way as a subclass override.
func TestSetSizeLimitsHonoredByClampResize(t *testing.T) {
	w := NewWindow(geom.NewRect(0, 0, 80, 24), "x", 0)
	w.SetSizeLimits(geom.Point{X: 50, Y: 10}, geom.Point{})

	gotW, gotH := clampResize(w, 20, 5)
	if gotW != 50 || gotH != 10 {
		t.Errorf("SetSizeLimits(50,10) clamp: got (%d, %d), want (50, 10)", gotW, gotH)
	}

	// Zero on an axis means "no caller limit" — fall through to the
	// 16×4 hard floor.
	w.SetSizeLimits(geom.Point{X: 40, Y: 0}, geom.Point{})
	gotW, gotH = clampResize(w, 5, 1)
	if gotW != 40 || gotH != 4 {
		t.Errorf("partial limits: got (%d, %d), want (40, 4)", gotW, gotH)
	}
}

// TestSetSizeLimitsThroughDialogEmbedding: Dialog embeds Window, so
// SetSizeLimits is callable on a *Dialog without a wrapper type and
// SizeLimits dispatches correctly through Self().
func TestSetSizeLimitsThroughDialogEmbedding(t *testing.T) {
	w := NewWindow(geom.NewRect(0, 0, 80, 24), "x", 0)
	w.SetSizeLimits(geom.Point{X: 60, Y: 12}, geom.Point{X: 100, Y: 30})
	// Dispatch via Self() — same path clampResize uses.
	gotMin, gotMax := w.Self().SizeLimits()
	if gotMin.X != 60 || gotMin.Y != 12 {
		t.Errorf("Self().SizeLimits min: got %v, want (60, 12)", gotMin)
	}
	if gotMax.X != 100 || gotMax.Y != 30 {
		t.Errorf("Self().SizeLimits max: got %v, want (100, 30)", gotMax)
	}
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
