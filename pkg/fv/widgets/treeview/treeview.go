// Package treeview provides TreeView — a hierarchical list widget with
// expandable / collapsible nodes.
package treeview

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Node is one tree node. Children may be nil or empty; a non-nil
// children slice (even if empty) means "this is an internal node".
type Node struct {
	Label    string
	Data     any
	Expanded bool
	Children []*Node
	Parent   *Node
}

// flatRow is a precomputed visible-row entry produced by walking the
// tree honoring Expanded flags.
type flatRow struct {
	depth int
	node  *Node
}

// TreeView paints a vertical scroll of flattened tree rows.
type TreeView struct {
	views.Base

	Roots   []*Node
	Focused int // index into the flattened list

	flat []flatRow

	Color, FocusColor, BranchColor uint16
}

// New constructs an empty tree.
func New(bounds geom.Rect, roots []*Node) *TreeView {
	t := &TreeView{
		Base:        views.NewBase(bounds),
		Roots:       roots,
		Color:       types.MakeAttr(0x00, 0x07),
		FocusColor:  types.MakeAttr(0x0F, 0x06),
		BranchColor: types.MakeAttr(0x08, 0x07),
	}
	t.SetSelf(t)
	t.Options |= consts.OfSelectable | consts.OfFirstClick
	t.State |= consts.SfCursorVis
	t.rebuildFlat()
	return t
}

// GetTypeID for serial registry.
func (t *TreeView) GetTypeID() string { return "treeview" }

// SetRoots replaces the root nodes.
func (t *TreeView) SetRoots(roots []*Node) {
	t.Roots = roots
	t.Focused = 0
	t.rebuildFlat()
}

// CurrentNode returns the focused node, or nil.
func (t *TreeView) CurrentNode() *Node {
	if t.Focused < 0 || t.Focused >= len(t.flat) {
		return nil
	}
	return t.flat[t.Focused].node
}

// Toggle expands or collapses the focused node.
func (t *TreeView) Toggle() {
	n := t.CurrentNode()
	if n == nil || len(n.Children) == 0 {
		return
	}
	n.Expanded = !n.Expanded
	t.rebuildFlat()
}

// rebuildFlat rebuilds the visible-row list. Called whenever roots or
// expansion state changes.
func (t *TreeView) rebuildFlat() {
	t.flat = t.flat[:0]
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		t.flat = append(t.flat, flatRow{depth: depth, node: n})
		if n.Expanded {
			for _, c := range n.Children {
				c.Parent = n
				walk(c, depth+1)
			}
		}
	}
	for _, r := range t.Roots {
		walk(r, 0)
	}
	if t.Focused >= len(t.flat) {
		t.Focused = len(t.flat) - 1
	}
	if t.Focused < 0 {
		t.Focused = 0
	}
}

// Draw paints visible rows.
func (t *TreeView) Draw() {
	w, h := t.Size.X, t.Size.Y
	top := 0
	if t.Focused >= h {
		top = t.Focused - h + 1
	}
	for r := 0; r < h; r++ {
		buf := screen.MakeDrawBuffer(w)
		idx := top + r
		c := t.Color
		if idx == t.Focused {
			c = t.FocusColor
		}
		for x := 0; x < w; x++ {
			screen.DrawCell(buf, x, " ", c)
		}
		if idx >= 0 && idx < len(t.flat) {
			row := t.flat[idx]
			x := row.depth * 2
			marker := "  "
			if len(row.node.Children) > 0 {
				if row.node.Expanded {
					marker = "▾ "
				} else {
					marker = "▸ "
				}
			}
			screen.DrawStr(buf, x, marker, t.BranchColor)
			screen.DrawStr(buf, x+2, row.node.Label, c)
		}
		t.WriteLine(0, r, w, 1, buf)
	}
}

// HandleEvent: arrows / pageup-down navigate; Enter or click-on-marker
// toggles; double-click commits.
func (t *TreeView) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		local := t.MakeLocal(ev.Where)
		idx := local.Y
		if idx >= 0 && idx < len(t.flat) {
			t.Focused = idx
			row := t.flat[idx]
			if local.X == row.depth*2 && len(row.node.Children) > 0 {
				row.node.Expanded = !row.node.Expanded
				t.rebuildFlat()
			} else if ev.DoubleClk {
				t.commit()
			}
			t.Draw()
			t.ClearEvent(ev)
		}
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbUp:
		if t.Focused > 0 {
			t.Focused--
		}
	case consts.KbDown:
		if t.Focused+1 < len(t.flat) {
			t.Focused++
		}
	case consts.KbHome:
		t.Focused = 0
	case consts.KbEnd:
		t.Focused = len(t.flat) - 1
	case consts.KbEnter:
		t.Toggle()
		t.commit()
	case consts.KbRight:
		if n := t.CurrentNode(); n != nil && len(n.Children) > 0 && !n.Expanded {
			n.Expanded = true
			t.rebuildFlat()
		}
	case consts.KbLeft:
		if n := t.CurrentNode(); n != nil && n.Expanded {
			n.Expanded = false
			t.rebuildFlat()
		}
	default:
		return
	}
	t.Draw()
	t.ClearEvent(ev)
}

func (t *TreeView) commit() {
	notify := drivers.Event{
		What:    consts.EvBroadcast,
		Command: consts.CmListItemSelected,
		InfoPtr: t.CurrentNode(),
	}
	t.PutEvent(&notify)
}
