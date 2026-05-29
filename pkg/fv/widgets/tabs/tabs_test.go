package tabs

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// TestCurrentDoesNotShadowGroup verifies that *Tabs does not declare a
// Current() method whose return type differs from the inherited
// views.Group.Current() views.View. The framework's focus-chain walks
// (Program.placeCursor, focusWantsRawKeys, helpCtxFromFocus, the
// tooltip lookup) detect walkable nodes via `interface{ Current()
// views.View }`. Any Tabs-level shadow with a different return type
// stops those walks at the Tabs node and the focused body view is
// never reached.
func TestCurrentDoesNotShadowGroup(t *testing.T) {
	type currenter interface{ Current() views.View }

	tw := New(geom.NewRect(0, 0, 30, 10))
	body := views.NewGroup(geom.NewRect(0, 0, 28, 7))
	leaf := views.NewGroup(geom.NewRect(0, 0, 10, 1))
	leaf.Options |= consts.OfSelectable
	body.Insert(leaf)
	body.Focus(leaf)
	tw.AddTab("a", body)
	tw.SetCurrent(0)

	c, ok := interface{}(tw).(currenter)
	if !ok {
		t.Fatal("*Tabs does not satisfy `interface{ Current() views.View }` — " +
			"a shadowed Current() with a non-views.View return type breaks " +
			"the focus-chain walk in Program.placeCursor")
	}
	if c.Current() != views.View(body) {
		t.Fatalf("Tabs.Current() should return the focused body view; got %#v, want %#v",
			c.Current(), body)
	}

	// Full walk should reach the focused leaf inside the body.
	var v views.View = tw
	for {
		cc, ok := v.(currenter)
		if !ok {
			break
		}
		next := cc.Current()
		if next == nil || next == v {
			break
		}
		v = next
	}
	if v != views.View(leaf) {
		t.Fatalf("focus-chain walk stopped short of leaf: got %#v, want %#v", v, leaf)
	}
}

// TestDefaultGrowModeFillsParent: the constructor must default to
// "anchor top-left, stretch bottom-right". GfGrowAll would translate
// the widget (LoX/LoY shift Origin too), leaving the parent's
// top-left blank after a resize.
func TestDefaultGrowModeFillsParent(t *testing.T) {
	tw := New(geom.NewRect(0, 0, 30, 10))
	if got, want := tw.GrowMode, consts.GfGrowHiX|consts.GfGrowHiY; got != want {
		t.Errorf("default GrowMode = %#x, want %#x (GfGrowHiX | GfGrowHiY)", got, want)
	}
	// Simulate the parent growing by (+5, +3). Origin must stay (0,0);
	// the size should grow to absorb the delta.
	before := tw.GetBounds()
	// Absolute grow (no GfGrowRel): the new-owner-size arg is unused.
	bounds := tw.CalcBounds(geom.Point{X: 5, Y: 3}, geom.Point{X: 35, Y: 13})
	if bounds.A != before.A {
		t.Errorf("origin shifted from %v to %v on grow; want unchanged", before.A, bounds.A)
	}
	if bounds.B.X != before.B.X+5 || bounds.B.Y != before.B.Y+3 {
		t.Errorf("bottom-right after grow = %v, want %v", bounds.B,
			geom.Point{X: before.B.X + 5, Y: before.B.Y + 3})
	}
}

// TestCurrentIndex returns the active tab's index and updates with
// SetCurrent / DeleteTab.
func TestCurrentIndex(t *testing.T) {
	tw := New(geom.NewRect(0, 0, 30, 10))
	if got := tw.CurrentIndex(); got != 0 {
		t.Errorf("empty tabs CurrentIndex = %d, want 0", got)
	}
	for _, name := range []string{"a", "b", "c"} {
		tw.AddTab(name, views.NewGroup(geom.NewRect(0, 0, 28, 7)))
	}
	if got := tw.CurrentIndex(); got != 0 {
		t.Errorf("after AddTab CurrentIndex = %d, want 0", got)
	}
	tw.SetCurrent(2)
	if got := tw.CurrentIndex(); got != 2 {
		t.Errorf("after SetCurrent(2) CurrentIndex = %d, want 2", got)
	}
	tw.DeleteTab(2)
	if got := tw.CurrentIndex(); got != 1 {
		t.Errorf("after DeleteTab(2) CurrentIndex = %d, want 1", got)
	}
}
