package treeview

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// TestOnActivateFiresOnRowReclick verifies that OnActivate fires even
// when the click lands on the already-focused row — the file-tree
// "click any file to open it" use case. OnSelect's defer-on-delta
// pattern would skip this case; OnActivate must not.
func TestOnActivateFiresOnRowReclick(t *testing.T) {
	roots := []*Node{{Label: "a"}, {Label: "b"}}
	tv := New(geom.NewRect(0, 0, 20, 10), roots)

	var activateCalls int
	var lastActivated *Node
	tv.OnActivate = func(n *Node) {
		activateCalls++
		lastActivated = n
	}

	// First click on row 0 (already focused at construction).
	click := &drivers.Event{
		What:  consts.EvMouseDown,
		Where: geom.Point{X: 3, Y: 0},
	}
	tv.HandleEvent(click)
	if activateCalls != 1 || lastActivated == nil || lastActivated.Label != "a" {
		t.Fatalf("first click on already-focused row: activateCalls=%d, lastActivated=%v",
			activateCalls, lastActivated)
	}

	// Click row 0 again — this is the case OnSelect skips. OnActivate
	// must still fire.
	click2 := &drivers.Event{
		What:  consts.EvMouseDown,
		Where: geom.Point{X: 3, Y: 0},
	}
	tv.HandleEvent(click2)
	if activateCalls != 2 {
		t.Fatalf("second click on same row: activateCalls=%d, want 2 (OnActivate must fire on every click, not just focus deltas)",
			activateCalls)
	}
}

// TestOnActivateFiresOnEnter covers the keyboard path.
func TestOnActivateFiresOnEnter(t *testing.T) {
	roots := []*Node{{Label: "x"}}
	tv := New(geom.NewRect(0, 0, 20, 10), roots)
	var got *Node
	tv.OnActivate = func(n *Node) { got = n }
	tv.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbEnter})
	if got == nil || got.Label != "x" {
		t.Errorf("after Enter: got %v, want node 'x'", got)
	}
}

// TestDefaultGrowMode verifies the constructor sets a sensible default
// GrowMode so the tree resizes with its host instead of staying at
// its initial bounds.
func TestDefaultGrowMode(t *testing.T) {
	tv := New(geom.NewRect(0, 0, 20, 10), nil)
	want := consts.GfGrowHiX | consts.GfGrowHiY
	if tv.GrowMode != want {
		t.Errorf("default GrowMode = %#x, want %#x (GfGrowHiX | GfGrowHiY)", tv.GrowMode, want)
	}
}
