package treeview

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

func rightClick(x, y int) *drivers.Event {
	return &drivers.Event{
		What:    consts.EvMouseDown,
		Buttons: consts.MbRightButton,
		Where:   geom.Point{X: x, Y: y},
	}
}

func TestRightClickFiresOnContextMenu(t *testing.T) {
	roots := []*Node{
		{Label: "a", Expanded: true, Children: []*Node{{Label: "a1"}}},
		{Label: "b"},
	}
	tv := New(geom.NewRect(0, 0, 20, 10), roots)

	var ctxNode *Node
	var ctxWhere geom.Point
	var activated, toggled bool
	tv.OnContextMenu = func(n *Node, where geom.Point) { ctxNode, ctxWhere = n, where }
	tv.OnActivate = func(*Node) { activated = true }
	tv.OnExpand = func(*Node) { toggled = true }

	// Row 1 = "a1". Click on the marker column too — right-click must
	// never toggle.
	ev := rightClick(2, 1)
	tv.HandleEvent(ev)

	if ctxNode == nil || ctxNode.Label != "a1" {
		t.Fatalf("OnContextMenu node = %v, want a1", ctxNode)
	}
	if ctxWhere != (geom.Point{X: 2, Y: 1}) {
		t.Errorf("where = %v, want screen point of the click", ctxWhere)
	}
	if tv.Focused != 1 {
		t.Errorf("Focused = %d, want 1 (right-click focuses the row)", tv.Focused)
	}
	if activated || toggled {
		t.Error("right-click must not fire OnActivate or toggle expansion")
	}
	if ev.What != consts.EvNothing {
		t.Error("right-click on a row must be consumed")
	}
}

func TestRightClickFiresOnSelectOnFocusChange(t *testing.T) {
	roots := []*Node{{Label: "a"}, {Label: "b"}}
	tv := New(geom.NewRect(0, 0, 20, 10), roots)
	var selected *Node
	tv.OnSelect = func(n *Node) { selected = n }
	tv.OnContextMenu = func(*Node, geom.Point) {}
	tv.HandleEvent(rightClick(3, 1))
	if selected == nil || selected.Label != "b" {
		t.Errorf("OnSelect = %v, want b (focus moved)", selected)
	}
}

func TestRightClickBelowContentIsNoOp(t *testing.T) {
	roots := []*Node{{Label: "only"}}
	tv := New(geom.NewRect(0, 0, 20, 10), roots)
	var fired bool
	tv.OnContextMenu = func(*Node, geom.Point) { fired = true }
	ev := rightClick(3, 7)
	tv.HandleEvent(ev)
	if fired {
		t.Error("right-click below the last row must not fire")
	}
	if ev.What == consts.EvNothing {
		t.Error("right-click below content must not be consumed")
	}
}

func TestRightClickWithoutHookStillFocuses(t *testing.T) {
	roots := []*Node{{Label: "a"}, {Label: "b"}}
	tv := New(geom.NewRect(0, 0, 20, 10), roots)
	tv.HandleEvent(rightClick(3, 1))
	if tv.Focused != 1 {
		t.Errorf("Focused = %d, want 1", tv.Focused)
	}
}
