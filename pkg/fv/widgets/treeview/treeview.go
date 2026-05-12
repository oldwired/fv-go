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
//
// HasChildren is a hint for lazy-loaded trees: set it true on nodes
// whose Children will be populated by OnExpand. The draw layer shows
// the expand marker when HasChildren is true even before any actual
// children exist.
type Node struct {
	Label       string
	Data        any
	Expanded    bool
	Children    []*Node
	Parent      *Node
	HasChildren bool
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

	// OnExpand fires when the user is about to expand a node (its
	// Expanded flag is about to flip from false to true). Callers
	// populate Children here for lazy loading — e.g., a directory
	// tree reads the filesystem on demand. Called before Toggle
	// flips the flag, so Children mutations are picked up by
	// rebuildFlat immediately.
	OnExpand func(n *Node)
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

// Toggle expands or collapses the focused node. When expanding, the
// OnExpand callback (if set) fires first so callers can lazy-populate
// Children — a node with no children before the callback still
// expands if the callback adds them.
func (t *TreeView) Toggle() {
	n := t.CurrentNode()
	if n == nil {
		return
	}
	if !n.Expanded && t.OnExpand != nil {
		t.OnExpand(n)
	}
	if len(n.Children) == 0 {
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
			if row.node.HasChildren || len(row.node.Children) > 0 {
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
// toggles; double-click commits; mouse-wheel scrolls.
//
// Click also takes group-level focus (Owner.Focus(self)) — without
// that, subsequent key events go to whoever was focused before,
// matching the ListViewer pattern.
func (t *TreeView) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		// Mouse wheel: scroll without changing the expand state. We
		// move Focused by ±3, which makes the visible row range
		// shift in Draw (top = Focused - h + 1 when Focused >= h).
		if ev.Buttons&(consts.MbScrollWheelUp|consts.MbScrollWheelDown) != 0 {
			step := 3
			if ev.Buttons&consts.MbScrollWheelUp != 0 {
				t.Focused -= step
			} else {
				t.Focused += step
			}
			t.clampFocused()
			t.ClearEvent(ev)
			return
		}
		local := t.MakeLocal(ev.Where)
		// Account for the visible top — clicking row 0 of the
		// viewport when scrolled means flat[topVisible], not flat[0].
		idx := t.topVisible() + local.Y
		if idx >= 0 && idx < len(t.flat) {
			t.Focused = idx
			row := t.flat[idx]
			// Marker glyph is 2 cells wide (e.g., "▾ "), starting at
			// depth*2. Either cell counts as a marker hit.
			markerCol := row.depth * 2
			onMarker := local.X >= markerCol && local.X <= markerCol+1
			if onMarker && (row.node.HasChildren || len(row.node.Children) > 0) {
				t.Toggle()
			} else if ev.DoubleClk {
				t.commit()
			}
			if t.Owner != nil {
				t.Owner.Focus(t.Self())
			}
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
	case consts.KbPgUp:
		t.Focused -= t.Size.Y
		t.clampFocused()
	case consts.KbPgDn:
		t.Focused += t.Size.Y
		t.clampFocused()
	case consts.KbHome:
		t.Focused = 0
	case consts.KbEnd:
		t.Focused = len(t.flat) - 1
	case consts.KbEnter:
		t.Toggle()
		t.commit()
	case consts.KbRight:
		n := t.CurrentNode()
		if n != nil && !n.Expanded && (n.HasChildren || len(n.Children) > 0) {
			t.Toggle()
		}
	case consts.KbLeft:
		if n := t.CurrentNode(); n != nil && n.Expanded {
			n.Expanded = false
			t.rebuildFlat()
		}
	default:
		return
	}
	t.ClearEvent(ev)
}

// topVisible returns the index of the topmost row currently rendered,
// matching the math in Draw. Used to translate mouse coords into the
// flat-list index when the tree has scrolled.
func (t *TreeView) topVisible() int {
	if t.Focused >= t.Size.Y {
		return t.Focused - t.Size.Y + 1
	}
	return 0
}

func (t *TreeView) clampFocused() {
	if t.Focused < 0 {
		t.Focused = 0
	}
	if t.Focused >= len(t.flat) {
		t.Focused = len(t.flat) - 1
	}
	if t.Focused < 0 {
		t.Focused = 0
	}
}

func (t *TreeView) commit() {
	notify := drivers.Event{
		What:    consts.EvBroadcast,
		Command: consts.CmListItemSelected,
		InfoPtr: t.CurrentNode(),
	}
	t.PutEvent(&notify)
}
