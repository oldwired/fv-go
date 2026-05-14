package treeview

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// TestOnSelectFiresOnArrowNav: pressing Down moves the highlight and
// fires OnSelect with the new node.
func TestOnSelectFiresOnArrowNav(t *testing.T) {
	roots := []*Node{
		{Label: "a"},
		{Label: "b"},
		{Label: "c"},
	}
	tv := New(geom.NewRect(0, 0, 20, 10), roots)
	var got *Node
	var calls int
	tv.OnSelect = func(n *Node) {
		got = n
		calls++
	}
	// Initial Focused = 0 ("a"). Down → "b".
	tv.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbDown})
	if calls != 1 {
		t.Errorf("OnSelect calls = %d, want 1", calls)
	}
	if got == nil || got.Label != "b" {
		t.Errorf("OnSelect got %v, want node 'b'", got)
	}
}

// TestOnSelectFiresOnHomeEnd covers the Home/End nav paths.
func TestOnSelectFiresOnHomeEnd(t *testing.T) {
	roots := []*Node{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	tv := New(geom.NewRect(0, 0, 20, 10), roots)
	var got *Node
	tv.OnSelect = func(n *Node) { got = n }
	tv.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbEnd})
	if got == nil || got.Label != "c" {
		t.Errorf("after End: got %v, want 'c'", got)
	}
	tv.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbHome})
	if got == nil || got.Label != "a" {
		t.Errorf("after Home: got %v, want 'a'", got)
	}
}

// TestOnSelectNoFireWhenUnchanged: pressing Up at the top doesn't
// move Focused, so OnSelect must not fire.
func TestOnSelectNoFireWhenUnchanged(t *testing.T) {
	roots := []*Node{{Label: "a"}, {Label: "b"}}
	tv := New(geom.NewRect(0, 0, 20, 10), roots)
	var calls int
	tv.OnSelect = func(n *Node) { calls++ }
	// At index 0; Up is a no-op.
	tv.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbUp})
	if calls != 0 {
		t.Errorf("OnSelect fired %d times on no-op nav, want 0", calls)
	}
}

// TestOnSelectAfterFocusUpdate: callback reads of CurrentNode return
// the new node — i.e., the field is updated BEFORE the callback fires.
func TestOnSelectAfterFocusUpdate(t *testing.T) {
	roots := []*Node{{Label: "a"}, {Label: "b"}}
	tv := New(geom.NewRect(0, 0, 20, 10), roots)
	tv.OnSelect = func(n *Node) {
		if cur := tv.CurrentNode(); cur != n {
			t.Errorf("inside OnSelect, CurrentNode() = %v, but n = %v", cur, n)
		}
	}
	tv.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbDown})
}
