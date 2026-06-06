package editor

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/clipboard"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
)

func TestAddCaretOrderingAndDedupe(t *testing.T) {
	e := newTestEditor()
	e.SetText("0123456789")
	e.MoveCursor(5, false)
	e.AddCaret(8)
	e.AddCaret(2)
	e.AddCaret(8) // duplicate
	cs := e.Carets()
	if len(cs) != 3 {
		t.Fatalf("carets = %d, want 3 (duplicate must merge)", len(cs))
	}
	if cs[0].Pos != 5 {
		t.Errorf("primary = %d, want 5", cs[0].Pos)
	}
	if cs[1].Pos != 2 || cs[2].Pos != 8 {
		t.Errorf("extras = %d,%d, want 2,8 (sorted)", cs[1].Pos, cs[2].Pos)
	}
}

func TestCaretOverlapMerge(t *testing.T) {
	e := newTestEditor()
	e.SetText("0123456789")
	e.SetCarets([]Caret{
		{Pos: 6, Anchor: 2},
		{Pos: 8, Anchor: 4}, // overlaps [2,6)
	})
	cs := e.Carets()
	if len(cs) != 1 {
		t.Fatalf("overlapping selections must merge: %d carets", len(cs))
	}
	if cs[0].lo() != 2 || cs[0].hi() != 8 {
		t.Errorf("merged = [%d,%d), want [2,8)", cs[0].lo(), cs[0].hi())
	}
}

func TestMultiInsertAndSingleUndo(t *testing.T) {
	e := newTestEditor()
	e.SetText("a b c")
	e.MoveCursor(1, false)
	e.AddCaret(3)
	e.AddCaret(5)
	e.Insert("X")
	if got := e.Text(); got != "aX bX cX" {
		t.Fatalf("multi-insert: %q", got)
	}
	cs := e.Carets()
	if cs[0].Pos != 2 || cs[1].Pos != 5 || cs[2].Pos != 8 {
		t.Errorf("carets after insert = %d,%d,%d, want 2,5,8",
			cs[0].Pos, cs[1].Pos, cs[2].Pos)
	}
	e.Undo()
	if got := e.Text(); got != "a b c" {
		t.Fatalf("one undo must reverse all three inserts: %q", got)
	}
	if e.HasMultipleCarets() {
		t.Error("undo should collapse secondary carets")
	}
}

func TestMultiInsertReplacesSelections(t *testing.T) {
	e := newTestEditor()
	e.SetText("foo bar foo")
	e.SetCarets([]Caret{
		{Pos: 3, Anchor: 0},
		{Pos: 11, Anchor: 8},
	})
	e.Insert("qux")
	if got := e.Text(); got != "qux bar qux" {
		t.Fatalf("selection replace: %q", got)
	}
}

func TestBackspaceCollisionMerge(t *testing.T) {
	e := newTestEditor()
	e.SetText("ab")
	e.MoveCursor(1, false)
	e.AddCaret(2)
	e.Backspace()
	if got := e.Text(); got != "" {
		t.Fatalf("both chars deleted: %q", got)
	}
	if e.HasMultipleCarets() {
		t.Error("colliding carets must merge after backspace")
	}
}

func TestDeleteForwardAtEOFSkips(t *testing.T) {
	e := newTestEditor()
	e.SetText("abc")
	e.MoveCursor(1, false)
	e.AddCaret(3) // EOF — nothing to delete forward
	e.DeleteForward()
	if got := e.Text(); got != "ac" {
		t.Fatalf("delete-forward: %q", got)
	}
	if len(e.Carets()) != 2 {
		t.Errorf("EOF caret should survive as a no-op")
	}
}

func TestMultiPaste(t *testing.T) {
	e := newTestEditor()
	e.SetText("1\n2\n3")
	e.MoveCursor(1, false)
	e.AddCaret(3)
	e.AddCaret(5)
	e.Paste(">>")
	if got := e.Text(); got != "1>>\n2>>\n3>>" {
		t.Fatalf("multi-paste: %q", got)
	}
}

func TestColumnSelectShortMiddleLine(t *testing.T) {
	e := newTestEditor()
	e.SetText("longline\nab\nlongline")
	e.ColumnSelect(0, 2, 4, 6)
	cs := e.Carets()
	if len(cs) != 3 {
		t.Fatalf("carets = %d, want 3", len(cs))
	}
	// Primary is on toLine (line 2): bytes 12.. → cols 4..6 → [16,18).
	if cs[0].lo() != 16 || cs[0].hi() != 18 {
		t.Errorf("primary sel = [%d,%d), want [16,18)", cs[0].lo(), cs[0].hi())
	}
	// Line 0: [4,6).
	if cs[1].lo() != 4 || cs[1].hi() != 6 {
		t.Errorf("line-0 sel = [%d,%d), want [4,6)", cs[1].lo(), cs[1].hi())
	}
	// Line 1 ("ab", 2 cols) is shorter than fromCol: empty caret at its
	// end (byte 11).
	if cs[2].hasSel() || cs[2].Pos != 11 {
		t.Errorf("short line caret = %+v, want empty at 11", cs[2])
	}
}

func TestColumnSelectTypingAllLines(t *testing.T) {
	e := newTestEditor()
	e.SetText("aa\nbb\ncc")
	e.ColumnSelect(0, 2, 2, 2) // empty caret at col 2 of each line
	e.Insert("!")
	if got := e.Text(); got != "aa!\nbb!\ncc!" {
		t.Fatalf("column-typing: %q", got)
	}
}

func TestEscCollapsesOnlyWithExtras(t *testing.T) {
	e := newTestEditor()
	e.SetText("abc")
	e.AddCaret(2)

	ev := drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbEsc}
	e.HandleEvent(&ev)
	if e.HasMultipleCarets() {
		t.Error("Esc should collapse secondary carets")
	}
	if ev.What != consts.EvNothing {
		t.Error("Esc that collapsed carets must be consumed")
	}

	ev2 := drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbEsc}
	e.HandleEvent(&ev2)
	if ev2.What == consts.EvNothing {
		t.Error("Esc with a single caret must NOT be consumed (dialogs need it)")
	}
}

func TestUpDownMoveAllCarets(t *testing.T) {
	e := newTestEditor()
	e.SetText("abcd\nefgh\nijkl\nmnop")
	e.MoveCursor(2, false) // line 0 col 2
	e.AddCaret(7)          // line 1 col 2
	ev := drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbDown}
	e.HandleEvent(&ev)
	cs := e.Carets()
	if cs[0].Pos != 7 || cs[1].Pos != 12 {
		t.Errorf("after Down: carets = %d,%d, want 7,12", cs[0].Pos, cs[1].Pos)
	}
}

func TestCtrlAltUpDownAddCarets(t *testing.T) {
	e := newTestEditor()
	e.SetText("abcd\nefgh\nijkl")
	e.MoveCursor(7, false) // line 1 col 2
	up := drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbCtrlUp,
		KeyShift: consts.KbCtrlShift | consts.KbAltShift}
	e.HandleEvent(&up)
	if up.What != consts.EvNothing {
		t.Fatal("Ctrl+Alt+Up must be consumed")
	}
	cs := e.Carets()
	if len(cs) != 2 || cs[1].Pos != 2 {
		t.Fatalf("carets after Ctrl+Alt+Up = %+v, want extra at 2", cs)
	}
	// Repeat extends the stack upward — but line 0 is the top, so a
	// second press is a no-op.
	e.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbCtrlUp,
		KeyShift: consts.KbCtrlShift | consts.KbAltShift})
	if len(e.Carets()) != 2 {
		t.Error("Ctrl+Alt+Up at top line must not add a caret")
	}

	down := drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbCtrlDown,
		KeyShift: consts.KbCtrlShift | consts.KbAltShift}
	e.HandleEvent(&down)
	cs = e.Carets()
	if len(cs) != 3 || cs[2].Pos != 12 {
		t.Fatalf("carets after Ctrl+Alt+Down = %+v, want extra at 12", cs)
	}

	// Bare Ctrl+Up stays unconsumed.
	bare := drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbCtrlUp,
		KeyShift: consts.KbCtrlShift}
	e.HandleEvent(&bare)
	if bare.What == consts.EvNothing {
		t.Error("bare Ctrl+Up must not be consumed")
	}
}

func TestMultiCopyJoinsSelections(t *testing.T) {
	clipboard.ResetForTest()
	defer clipboard.ResetForTest()
	e := newTestEditor()
	e.SetText("foo bar baz")
	e.SetCarets([]Caret{
		{Pos: 3, Anchor: 0},
		{Pos: 11, Anchor: 8},
	})
	e.Copy()
	if got := clipboard.GetText(); got != "foo\nbaz" {
		t.Errorf("multi-copy = %q, want %q", got, "foo\nbaz")
	}
}

func TestMultiCutDeletesAllSelections(t *testing.T) {
	clipboard.ResetForTest()
	defer clipboard.ResetForTest()
	e := newTestEditor()
	e.SetText("foo bar baz")
	e.SetCarets([]Caret{
		{Pos: 3, Anchor: 0},
		{Pos: 11, Anchor: 8},
	})
	e.Cut()
	if got := e.Text(); got != " bar " {
		t.Fatalf("multi-cut: %q", got)
	}
	e.Undo()
	if got := e.Text(); got != "foo bar baz" {
		t.Fatalf("cut must be one undo group: %q", got)
	}
}

func TestReadOnlyBlocksMultiEdit(t *testing.T) {
	e := newTestEditor()
	e.SetText("abc")
	e.AddCaret(2)
	e.ReadOnly = true
	e.Insert("X")
	e.Backspace()
	e.DeleteForward()
	if got := e.Text(); got != "abc" {
		t.Errorf("ReadOnly editor mutated: %q", got)
	}
}

func TestForeignSpliceRemapsExtraCarets(t *testing.T) {
	a, b := newSharedPair("0123456789")
	b.MoveCursor(2, false)
	b.AddCaret(8)
	a.MoveCursor(5, false)
	a.Insert("XX")
	cs := b.Carets()
	if cs[0].Pos != 2 || cs[1].Pos != 10 {
		t.Errorf("B carets = %d,%d, want 2,10", cs[0].Pos, cs[1].Pos)
	}
}
