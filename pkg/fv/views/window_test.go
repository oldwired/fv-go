package views

import (
	"testing"

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
