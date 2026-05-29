package treeview

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

func manyRoots(n int) []*Node {
	r := make([]*Node, n)
	for i := range r {
		r[i] = &Node{Label: string(rune('a' + i%26))}
	}
	return r
}

// A mouse-wheel scroll moves the viewport (top) without changing the
// selection (Focused) or firing OnSelect.
func TestWheelScrollDoesNotMoveSelection(t *testing.T) {
	tv := New(geom.NewRect(0, 0, 20, 5), manyRoots(40))
	var selects int
	tv.OnSelect = func(*Node) { selects++ }

	startFocused := tv.Focused

	tv.HandleEvent(&drivers.Event{What: consts.EvMouseWheel, Buttons: consts.MbScrollWheelDown})

	if tv.Focused != startFocused {
		t.Errorf("wheel changed Focused from %d to %d (should not move selection)", startFocused, tv.Focused)
	}
	if tv.top == 0 {
		t.Errorf("wheel did not scroll the viewport (top still 0)")
	}
	if selects != 0 {
		t.Errorf("wheel fired OnSelect %d times, want 0", selects)
	}
}

// Wheel scroll clamps within content bounds and back to the top.
func TestWheelScrollClamps(t *testing.T) {
	tv := New(geom.NewRect(0, 0, 20, 5), manyRoots(40))
	for i := 0; i < 50; i++ {
		tv.HandleEvent(&drivers.Event{What: consts.EvMouseWheel, Buttons: consts.MbScrollWheelDown})
	}
	if maxTop := len(tv.flat) - tv.Size.Y; tv.top != maxTop {
		t.Errorf("scrolled past end: top=%d, want %d", tv.top, maxTop)
	}
	for i := 0; i < 50; i++ {
		tv.HandleEvent(&drivers.Event{What: consts.EvMouseWheel, Buttons: consts.MbScrollWheelUp})
	}
	if tv.top != 0 {
		t.Errorf("scrolled past top: top=%d, want 0", tv.top)
	}
}

// Keyboard navigation past the bottom of the viewport scrolls to keep
// the selection visible.
func TestKeyNavScrollsToKeepFocusVisible(t *testing.T) {
	tv := New(geom.NewRect(0, 0, 20, 5), manyRoots(40))
	for i := 0; i < 10; i++ {
		tv.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbDown})
	}
	if tv.Focused != 10 {
		t.Fatalf("Focused = %d, want 10", tv.Focused)
	}
	if tv.Focused < tv.top || tv.Focused >= tv.top+tv.Size.Y {
		t.Errorf("focus %d outside visible window [%d,%d) after key nav", tv.Focused, tv.top, tv.top+tv.Size.Y)
	}
}
