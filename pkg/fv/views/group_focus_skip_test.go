package views

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

func emptyGroup() *Group {
	g := NewGroup(geom.NewRect(0, 0, 80, 24))
	g.current = -1
	g.Children = nil
	return g
}

func selDummy() *dummy {
	d := newDummy(geom.NewRect(0, 0, 10, 3))
	d.Options |= consts.OfSelectable
	return d
}

// SelectNext must skip selectable-but-disabled and selectable-but-hidden
// children and land on the next enabled+visible selectable view.
func TestSelectNextSkipsDisabledAndHidden(t *testing.T) {
	g := emptyGroup()
	a := selDummy()
	b := selDummy()
	b.State |= consts.SfDisabled
	c := selDummy()
	c.State &^= consts.SfVisible
	d := selDummy()

	g.Insert(a)
	g.Insert(b)
	g.Insert(c)
	g.Insert(d)
	g.Focus(a)

	g.SelectNext(true)
	if g.Current() != View(d) {
		t.Errorf("SelectNext skipped to %#v, want d (disabled b and hidden c must be skipped)", g.Current())
	}
}

// Insert auto-focus must not fire for a disabled selectable child on an
// otherwise-empty group.
func TestInsertAutoFocusSkipsDisabled(t *testing.T) {
	g := emptyGroup()
	b := selDummy()
	b.State |= consts.SfDisabled
	g.Insert(b)
	if g.Current() != nil {
		t.Errorf("disabled child auto-focused; current=%#v, want nil", g.Current())
	}

	a := selDummy()
	g.Insert(a)
	if g.Current() != View(a) {
		t.Errorf("enabled child should auto-focus; current=%#v, want a", g.Current())
	}
}

// Delete's focus restore must skip a disabled prior sibling.
func TestDeleteRestoreSkipsDisabled(t *testing.T) {
	g := emptyGroup()
	a := selDummy()
	bad := selDummy()
	bad.State |= consts.SfDisabled
	modal := selDummy()

	g.Insert(a)
	g.Insert(bad)
	g.Insert(modal)
	g.Focus(modal)

	g.Delete(modal)
	if g.Current() != View(a) {
		t.Errorf("after Delete(modal): current=%#v, want a (disabled sibling must be skipped)", g.Current())
	}
}

// MakeFirst on a non-selectable view raises its z-order but must not
// steal focus from the currently focused view.
func TestMakeFirstDoesNotFocusNonSelectable(t *testing.T) {
	g := emptyGroup()
	real := selDummy()
	deco := newDummy(geom.NewRect(0, 0, 10, 3)) // not OfSelectable

	g.Insert(real)
	g.Insert(deco)
	g.Focus(real)
	if g.Current() != View(real) {
		t.Fatalf("setup: current=%#v, want real", g.Current())
	}

	g.MakeFirst(deco)
	if g.Current() != View(real) {
		t.Errorf("MakeFirst(deco) stole focus; current=%#v, want real", g.Current())
	}
	if g.Children[len(g.Children)-1] != View(deco) {
		t.Errorf("MakeFirst(deco) did not raise deco to front")
	}
}
