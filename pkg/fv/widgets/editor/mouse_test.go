package editor

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// installQueue installs a fresh event queue prefilled with events; the
// drag capture loop drains it and MUST find a trailing EvMouseUp or it
// would spin forever.
func installQueue(t *testing.T, events ...drivers.Event) {
	t.Helper()
	views.ResetForTest()
	q := drivers.NewQueue()
	for _, ev := range events {
		if !q.Put(ev) {
			t.Fatalf("queue rejected %+v", ev)
		}
	}
	views.SetEventQueue(q)
	t.Cleanup(views.ResetForTest)
}

func mouse(what uint16, x, y int, shift uint16) drivers.Event {
	return drivers.Event{
		What:     what,
		Buttons:  consts.MbLeftButton,
		Where:    geom.Point{X: x, Y: y},
		KeyShift: shift,
	}
}

func TestClickMovesCursorWithoutQueue(t *testing.T) {
	views.ResetForTest()
	e := newTestEditor()
	e.SetText("hello\nworld")
	down := mouse(consts.EvMouseDown, 3, 1, 0)
	e.HandleEvent(&down)
	if e.Cursor != 9 {
		t.Errorf("click cursor = %d, want 9", e.Cursor)
	}
	if down.What != consts.EvNothing {
		t.Error("click must be consumed")
	}
}

func TestDragSelectsRange(t *testing.T) {
	installQueue(t,
		mouse(consts.EvMouseMove, 4, 0, 0),
		mouse(consts.EvMouseUp, 4, 0, 0),
	)
	e := newTestEditor()
	e.SetText("hello world")
	down := mouse(consts.EvMouseDown, 1, 0, 0)
	e.HandleEvent(&down)
	if !e.HasSelection() {
		t.Fatal("drag must create a selection")
	}
	lo, hi := e.selRange()
	if lo != 1 || hi != 4 {
		t.Errorf("drag selection = [%d,%d), want [1,4)", lo, hi)
	}
}

func TestDragBelowBottomAutoScrolls(t *testing.T) {
	installQueue(t,
		mouse(consts.EvMouseMove, 0, 10, 0), // below the 10-row view
		mouse(consts.EvMouseMove, 0, 10, 0),
		mouse(consts.EvMouseMove, 0, 10, 0),
		mouse(consts.EvMouseUp, 0, 10, 0),
	)
	e := newTestEditor() // 40x10
	text := ""
	for i := 0; i < 30; i++ {
		text += "line\n"
	}
	e.SetText(text)
	down := mouse(consts.EvMouseDown, 0, 0, 0)
	e.HandleEvent(&down)
	if e.Top != 3 {
		t.Errorf("Top = %d after 3 off-edge drag events, want 3", e.Top)
	}
	if !e.HasSelection() {
		t.Error("off-edge drag must keep extending the selection")
	}
}

func TestAltClickTogglesCaret(t *testing.T) {
	installQueue(t,
		mouse(consts.EvMouseUp, 4, 0, consts.KbAltShift),
		mouse(consts.EvMouseUp, 4, 0, consts.KbAltShift),
	)
	e := newTestEditor()
	e.SetText("hello world")
	e.MoveCursor(0, false)

	down := mouse(consts.EvMouseDown, 4, 0, consts.KbAltShift)
	e.HandleEvent(&down)
	if !e.HasMultipleCarets() {
		t.Fatal("Alt+click must add a caret")
	}
	if e.Cursor != 0 {
		t.Errorf("Alt+click must not move the primary (cursor = %d)", e.Cursor)
	}

	down2 := mouse(consts.EvMouseDown, 4, 0, consts.KbAltShift)
	e.HandleEvent(&down2)
	if e.HasMultipleCarets() {
		t.Error("Alt+click on an existing caret must remove it")
	}
}

func TestAltDragColumnSelects(t *testing.T) {
	installQueue(t,
		mouse(consts.EvMouseMove, 4, 2, consts.KbAltShift),
		mouse(consts.EvMouseUp, 4, 2, consts.KbAltShift),
	)
	e := newTestEditor()
	e.SetText("aaaaaa\nbbbbbb\ncccccc")
	down := mouse(consts.EvMouseDown, 1, 0, consts.KbAltShift)
	e.HandleEvent(&down)
	cs := e.Carets()
	if len(cs) != 3 {
		t.Fatalf("column drag carets = %d, want 3", len(cs))
	}
	for _, c := range cs {
		if !c.hasSel() || c.hi()-c.lo() != 3 {
			t.Errorf("caret %+v: want a 3-byte column selection", c)
		}
	}
	// Primary sits on the pointer's line (line 2).
	if e.lineNumber(e.Cursor) != 2 {
		t.Errorf("primary on line %d, want 2", e.lineNumber(e.Cursor))
	}
}

func TestPlainDragCollapsesExistingCarets(t *testing.T) {
	installQueue(t,
		mouse(consts.EvMouseUp, 2, 0, 0),
	)
	e := newTestEditor()
	e.SetText("hello")
	e.AddCaret(4)
	down := mouse(consts.EvMouseDown, 2, 0, 0)
	e.HandleEvent(&down)
	if e.HasMultipleCarets() {
		t.Error("plain click must collapse secondary carets")
	}
}
