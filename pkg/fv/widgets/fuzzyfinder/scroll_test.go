package fuzzyfinder

import (
	"fmt"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

func testFinder(itemCount, height int) *FuzzyFinder {
	items := make([]string, itemCount)
	for i := range items {
		items[i] = fmt.Sprintf("item %02d", i)
	}
	return New(geom.NewRect(0, 0, 30, height), items)
}

func key(f *FuzzyFinder, code uint16) {
	f.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: code})
}

func TestSelectionScrollsResultViewport(t *testing.T) {
	f := testFinder(10, 7) // four visible result rows
	for range 5 {
		key(f, consts.KbDown)
	}
	if f.current != 5 || f.top != 2 {
		t.Fatalf("after moving down: current=%d top=%d, want 5 and 2", f.current, f.top)
	}

	key(f, consts.KbEnd)
	if f.current != 9 || f.top != 6 {
		t.Fatalf("after End: current=%d top=%d, want 9 and 6", f.current, f.top)
	}
	key(f, consts.KbHome)
	if f.current != 0 || f.top != 0 {
		t.Fatalf("after Home: current=%d top=%d, want 0 and 0", f.current, f.top)
	}
}

func TestFilteringClampsResultViewport(t *testing.T) {
	f := testFinder(10, 7)
	key(f, consts.KbEnd)
	f.query = "item 00"
	f.recalc()
	if len(f.matches) != 1 || f.current != 0 || f.top != 0 {
		t.Fatalf("after filtering: matches=%d current=%d top=%d, want 1, 0, 0", len(f.matches), f.current, f.top)
	}
}

func TestMouseClickAccountsForScrolledViewport(t *testing.T) {
	f := testFinder(10, 7)
	key(f, consts.KbEnd) // viewport is now 6..9
	ev := drivers.Event{
		What:    consts.EvMouseDown,
		Where:   geom.Point{X: 2, Y: 2}, // first visible result row
		Buttons: consts.MbLeftButton,
	}
	f.HandleEvent(&ev)
	if f.chosen != 6 {
		t.Fatalf("chosen=%d, want original item 6", f.chosen)
	}
}

func TestWheelAndPageKeysScroll(t *testing.T) {
	f := testFinder(20, 7)
	f.HandleEvent(&drivers.Event{What: consts.EvMouseWheel, Buttons: consts.MbScrollWheelDown})
	if f.current != 3 || f.top != 0 {
		t.Fatalf("after wheel: current=%d top=%d, want 3 and 0", f.current, f.top)
	}
	key(f, consts.KbPgDn)
	if f.current != 7 || f.top != 4 {
		t.Fatalf("after PageDown: current=%d top=%d, want 7 and 4", f.current, f.top)
	}
	key(f, consts.KbPgUp)
	if f.current != 3 || f.top != 3 {
		t.Fatalf("after PageUp: current=%d top=%d, want 3 and 3", f.current, f.top)
	}
}
