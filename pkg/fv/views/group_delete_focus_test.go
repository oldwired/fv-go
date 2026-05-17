package views

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// TestDeleteRestoresToPriorSelectable verifies that deleting the
// focused child does NOT land focus on the first non-selectable child
// (the classic "Desktop loses focus to its Background after a modal
// closes" bug). Group.Delete must pick the most-recent selectable
// sibling (walking backward), and fall back to a forward scan when
// nothing precedes the deleted index.
func TestDeleteRestoresToPriorSelectable(t *testing.T) {
	g := NewGroup(geom.NewRect(0, 0, 80, 24))
	g.current = -1
	g.Children = nil

	bg := newDummy(geom.NewRect(0, 0, 80, 24)) // index 0 — non-selectable wallpaper
	win := newDummy(geom.NewRect(0, 0, 40, 10))
	win.Options |= consts.OfSelectable
	modal := newDummy(geom.NewRect(0, 0, 30, 8))
	modal.Options |= consts.OfSelectable

	g.Insert(bg)
	g.Insert(win)
	if g.current != 1 || g.Current() != View(win) {
		t.Fatalf("setup: expected window to be current at index 1; got current=%d, view=%#v", g.current, g.Current())
	}

	// Modal opens on top and grabs focus.
	g.Insert(modal)
	g.Focus(modal)
	if g.Current() != View(modal) {
		t.Fatalf("focus modal: got %#v, want %#v", g.Current(), modal)
	}

	// Modal closes — focus must return to the window, not the wallpaper.
	g.Delete(modal)
	if g.Current() != View(win) {
		t.Fatalf("after Delete(modal): got %#v, want window (%#v). Background (non-selectable) is not a valid focus target",
			g.Current(), win)
	}
}

// TestDeleteShiftsCurrentWhenEarlierSiblingRemoved verifies that
// removing a child positioned BEFORE the current one shifts the
// current index left so it keeps pointing at the same logical view.
func TestDeleteShiftsCurrentWhenEarlierSiblingRemoved(t *testing.T) {
	g := NewGroup(geom.NewRect(0, 0, 80, 24))
	g.current = -1
	g.Children = nil

	a := newDummy(geom.Rect{})
	a.Options |= consts.OfSelectable
	b := newDummy(geom.Rect{})
	b.Options |= consts.OfSelectable
	c := newDummy(geom.Rect{})
	c.Options |= consts.OfSelectable

	g.Insert(a)
	g.Insert(b)
	g.Insert(c)
	g.Focus(c)
	if g.Current() != View(c) {
		t.Fatalf("setup: expected c to be current, got %#v", g.Current())
	}

	g.Delete(a)
	if g.Current() != View(c) {
		t.Fatalf("Delete(a) should keep focus on c; got %#v", g.Current())
	}
	if g.current != 1 {
		t.Fatalf("Delete(a) should shift current 2→1; got %d", g.current)
	}
}

// TestInsertPassive verifies the new InsertPassive primitive does not
// take focus even on an otherwise-empty group.
func TestInsertPassive(t *testing.T) {
	g := NewGroup(geom.NewRect(0, 0, 80, 24))
	g.current = -1
	g.Children = nil

	deco := newDummy(geom.Rect{})
	deco.Options |= consts.OfSelectable
	g.InsertPassive(deco)
	if g.Current() != nil {
		t.Fatalf("InsertPassive must not focus on empty group; got %#v", g.Current())
	}
	if deco.State&consts.SfFocused != 0 {
		t.Fatalf("InsertPassive set SfFocused on the inserted view; should be clear")
	}

	// When the group is already focused on something else, InsertPassive
	// should leave that alone.
	real := newDummy(geom.Rect{})
	real.Options |= consts.OfSelectable
	g.Insert(real)
	g.Focus(real)
	deco2 := newDummy(geom.Rect{})
	deco2.Options |= consts.OfSelectable
	g.InsertPassive(deco2)
	if g.Current() != View(real) {
		t.Fatalf("InsertPassive must not move focus from prior view; got %#v, want %#v", g.Current(), real)
	}
}
