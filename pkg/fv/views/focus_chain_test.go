package views

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// Focusing a child of a nested group must focus the whole owner chain
// — keyboard dispatch descends through Current() at every level, so a
// click on an editor pane inside a SplitGroup inside a Window must
// leave window.Current() pointing at the SplitGroup, not at whatever
// sibling (file tree, button) was focused before.
func TestFocusPropagatesUpOwnerChain(t *testing.T) {
	win := NewGroup(geom.NewRect(0, 0, 80, 24))
	tree := selDummy()
	win.Insert(tree) // auto-focused: the "file tree" sibling

	split := NewGroup(geom.NewRect(20, 0, 80, 24))
	win.Insert(split)
	pane := NewGroup(geom.NewRect(0, 0, 60, 12))
	split.Insert(pane)
	ed := selDummy()
	pane.Insert(ed)

	if win.Current() != View(tree) {
		t.Fatalf("setup: window current = %#v, want tree", win.Current())
	}

	// The widget click path: leaf calls Owner.Focus(self).
	pane.Focus(ed)

	if pane.Current() != View(ed) {
		t.Errorf("pane current = %#v, want ed", pane.Current())
	}
	if split.Current() != View(pane) {
		t.Errorf("split current = %#v, want pane (chain must propagate)", split.Current())
	}
	if win.Current() != View(split) {
		t.Errorf("window current = %#v, want split (chain must propagate)", win.Current())
	}

	// Clicking back on the tree re-roots the chain at the window level.
	win.Focus(tree)
	if win.Current() != View(tree) {
		t.Errorf("window current = %#v, want tree", win.Current())
	}
}

// Moving focus out of a nested subtree must strip SfFocused from its
// inner chain — otherwise widgets in the abandoned pane keep rendering
// focused forever — and refocusing the subtree must restore the flag
// down its remembered Current() chain.
func TestFocusClearsAndRestoresNestedSfFocused(t *testing.T) {
	win := NewGroup(geom.NewRect(0, 0, 80, 24))
	pane1 := NewGroup(geom.NewRect(0, 0, 40, 24))
	win.Insert(pane1)
	leaf1 := selDummy()
	pane1.Insert(leaf1)
	pane2 := NewGroup(geom.NewRect(40, 0, 80, 24))
	win.Insert(pane2)
	leaf2 := selDummy()
	pane2.Insert(leaf2)

	pane1.Focus(leaf1)
	if leaf1.State&consts.SfFocused == 0 {
		t.Fatal("setup: leaf1 should be focused")
	}

	pane2.Focus(leaf2)
	if leaf1.State&consts.SfFocused != 0 {
		t.Error("leaf1 kept SfFocused after focus moved to the other pane")
	}
	if leaf2.State&consts.SfFocused == 0 {
		t.Error("leaf2 must gain SfFocused")
	}

	// Refocusing pane1 at the window level restores the inner chain.
	win.Focus(pane1)
	if leaf1.State&consts.SfFocused == 0 {
		t.Error("leaf1 must regain SfFocused when its pane is refocused")
	}
	if leaf2.State&consts.SfFocused != 0 {
		t.Error("leaf2 must lose SfFocused when its pane is abandoned")
	}
}
