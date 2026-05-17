package dialogs

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// TestStringListBoxOnFocusFiresOnFocusItem verifies that the new
// OnFocus callback on ListViewer fires when Focused changes via
// FocusItem (the same path used by arrow-key navigation, click,
// wheel, Home/End). Avoids the prior boilerplate of watching for
// CmListItemSelected broadcasts to drive a "right pane follows left
// selection" pattern.
func TestStringListBoxOnFocusFiresOnFocusItem(t *testing.T) {
	lb := NewStringListBox(geom.NewRect(0, 0, 20, 5), nil, []string{"a", "b", "c"})
	var got []int
	lb.OnFocus = func(idx int) { got = append(got, idx) }

	lb.FocusItem(2)
	lb.FocusItem(0)
	lb.FocusItem(0) // no-op — same index, must not refire

	if len(got) != 2 || got[0] != 2 || got[1] != 0 {
		t.Errorf("OnFocus callbacks = %v, want [2, 0]", got)
	}
}

// TestStringListBoxOnFocusNotFiredForSameIndex: FocusItem with the
// current index must not refire OnFocus — keeps host-side dedup
// trivial.
func TestStringListBoxOnFocusNotFiredForSameIndex(t *testing.T) {
	lb := NewStringListBox(geom.NewRect(0, 0, 20, 5), nil, []string{"a", "b"})
	lb.FocusItem(1)
	var calls int
	lb.OnFocus = func(idx int) { calls++ }
	lb.FocusItem(1)
	if calls != 0 {
		t.Errorf("OnFocus fired %d times for no-op FocusItem, want 0", calls)
	}
}

// TestSetItemsResetsFocus documents the existing SetItems contract.
// (Guards against accidentally turning SetItems into a "preserving"
// variant; SetItemsKeepFocus is the explicit way to ask for that.)
func TestSetItemsResetsFocus(t *testing.T) {
	lb := NewStringListBox(geom.NewRect(0, 0, 20, 5), nil, []string{"a", "b", "c"})
	lb.FocusItem(2)
	lb.SetItems([]string{"x", "y", "z", "w"})
	if lb.Focused != 0 {
		t.Errorf("after SetItems: Focused = %d, want 0 (hard reset)", lb.Focused)
	}
}

// TestSetItemsKeepFocusPreserves keeps the prior Focused index when
// it's still in range — the master-detail pattern reporters used to
// have to patch up manually.
func TestSetItemsKeepFocusPreserves(t *testing.T) {
	lb := NewStringListBox(geom.NewRect(0, 0, 20, 5), nil, []string{"a", "b", "c"})
	lb.FocusItem(2)
	lb.SetItemsKeepFocus([]string{"x", "y", "z", "w"})
	if lb.Focused != 2 {
		t.Errorf("SetItemsKeepFocus must keep Focused = 2, got %d", lb.Focused)
	}
}

// TestSetItemsKeepFocusClampsToRange: when the new items are shorter
// than the prior Focused, clamp to the last valid row.
func TestSetItemsKeepFocusClampsToRange(t *testing.T) {
	lb := NewStringListBox(geom.NewRect(0, 0, 20, 5), nil, []string{"a", "b", "c", "d"})
	lb.FocusItem(3)
	lb.SetItemsKeepFocus([]string{"x", "y"})
	if lb.Focused != 1 {
		t.Errorf("SetItemsKeepFocus shrinking from 4→2 with prev=3: Focused = %d, want 1", lb.Focused)
	}
}

// TestSetItemsKeepFocusEmptyClampsToZero: when the new items list is
// empty, Focused clamps to 0 (the "no valid selection" sentinel; the
// caller is expected to check Range before reading).
func TestSetItemsKeepFocusEmptyClampsToZero(t *testing.T) {
	lb := NewStringListBox(geom.NewRect(0, 0, 20, 5), nil, []string{"a", "b"})
	lb.FocusItem(1)
	lb.SetItemsKeepFocus(nil)
	if lb.Focused != 0 {
		t.Errorf("SetItemsKeepFocus(empty): Focused = %d, want 0", lb.Focused)
	}
}
