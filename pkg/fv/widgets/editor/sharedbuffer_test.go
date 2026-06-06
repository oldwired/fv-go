package editor

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

func testRect() geom.Rect { return geom.NewRect(0, 0, 40, 10) }

func newSharedPair(text string) (*Editor, *Editor) {
	a := New(testRect(), nil, nil)
	a.SetText(text)
	b := NewShared(testRect(), nil, nil, a.Buf)
	return a, b
}

func TestSharedEditVisibleInOtherPane(t *testing.T) {
	a, b := newSharedPair("hello")
	a.MoveCursor(5, false)
	a.Insert(" world")
	if got := b.Text(); got != "hello world" {
		t.Fatalf("pane B sees %q", got)
	}
}

func TestSharedCursorRemapAcrossSplice(t *testing.T) {
	cases := []struct {
		name    string
		bCursor int
		want    int
	}{
		{"before splice", 2, 2},
		{"at splice start", 5, 5},
		{"inside deleted range", 7, 5},
		{"after splice", 9, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, b := newSharedPair("0123456789")
			b.MoveCursor(c.bCursor, false)
			// A replaces [5,9) ("5678") with "X": net delta -3.
			a.SelAnchor = 5
			a.Cursor = 9
			a.Insert("X")
			if b.Cursor != c.want {
				t.Errorf("B cursor = %d, want %d", b.Cursor, c.want)
			}
		})
	}
}

func TestSharedSelectionRemap(t *testing.T) {
	a, b := newSharedPair("0123456789")
	b.SelAnchor = 6
	b.Cursor = 9
	a.MoveCursor(0, false)
	a.Insert("ab")
	if b.SelAnchor != 8 || b.Cursor != 11 {
		t.Errorf("B selection = [%d,%d], want [8,11]", b.SelAnchor, b.Cursor)
	}
}

func TestSharedTopShiftsForEditsAbove(t *testing.T) {
	a, b := newSharedPair("l0\nl1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\n")
	b.Top = 5
	// Insert two lines at the very top.
	a.MoveCursor(0, false)
	a.Insert("x\ny\n")
	if b.Top != 7 {
		t.Errorf("B Top = %d, want 7 (shifted by inserted lines)", b.Top)
	}
	// An edit below B's Top must not move it.
	a.MoveCursor(a.Len(), false)
	a.Insert("z\n")
	if b.Top != 7 {
		t.Errorf("B Top = %d after edit below, want 7", b.Top)
	}
}

func TestSharedTopClampsWhenLinesAboveDeleted(t *testing.T) {
	a, b := newSharedPair("l0\nl1\nl2\nl3\nl4\nl5\nl6\nl7\n")
	b.Top = 5
	// Delete lines 1..6 (bytes 3..21) — the deletion crosses B's Top.
	a.Buf.ReplaceRange(3, 21, "")
	if b.Top != 1 {
		t.Errorf("B Top = %d, want 1 (clamped to splice start line)", b.Top)
	}
}

func TestSharedUndoFromOtherPane(t *testing.T) {
	a, b := newSharedPair("hello")
	a.MoveCursor(5, false)
	a.Insert("!")
	b.MoveCursor(0, false)
	b.Undo()
	if got := a.Text(); got != "hello" {
		t.Fatalf("undo from B: %q", got)
	}
	// B's caret jumps to the undone edit's site (A's pre-edit cursor).
	if b.Cursor != 5 {
		t.Errorf("B cursor after undo = %d, want 5", b.Cursor)
	}
}

func TestSharedSetTextResetsBothPanes(t *testing.T) {
	a, b := newSharedPair("one\ntwo\nthree")
	a.MoveCursor(8, false)
	b.MoveCursor(5, false)
	b.Top = 2
	a.SetText("fresh")
	if a.Cursor != 0 || b.Cursor != 0 || b.Top != 0 {
		t.Errorf("SetText must reset both panes: aCur=%d bCur=%d bTop=%d",
			a.Cursor, b.Cursor, b.Top)
	}
	if got := b.Text(); got != "fresh" {
		t.Errorf("B text = %q", got)
	}
}

func TestSharedOnChangeFiresOnBothPanes(t *testing.T) {
	a, b := newSharedPair("x")
	var aV, bV []int
	a.OnChange = func(v int) { aV = append(aV, v) }
	b.OnChange = func(v int) { bV = append(bV, v) }
	a.Insert("y")
	if len(aV) != 1 || len(bV) != 1 {
		t.Fatalf("OnChange fires: A=%d B=%d, want 1 each", len(aV), len(bV))
	}
	if aV[0] != bV[0] {
		t.Errorf("version mismatch: A=%d B=%d", aV[0], bV[0])
	}
}

func TestSharedDetachStopsNotifications(t *testing.T) {
	a, b := newSharedPair("abc")
	b.MoveCursor(3, false)
	b.Detach()
	a.MoveCursor(0, false)
	a.Insert("xx")
	if b.Cursor != 3 {
		t.Errorf("detached pane was still remapped: cursor = %d", b.Cursor)
	}
}

func TestBookmarksRemapAcrossEdits(t *testing.T) {
	e := New(testRect(), nil, nil)
	e.SetText("hello world")
	e.MoveCursor(6, false)
	e.bookmarks[1] = 6 // mark at "world"
	e.MoveCursor(0, false)
	e.Insert("say: ")
	if e.bookmarks[1] != 11 {
		t.Errorf("bookmark = %d, want 11 (shifted by insert above)", e.bookmarks[1])
	}
}

func TestReplaceRangeKeepsActingEditorCursorStable(t *testing.T) {
	e := New(testRect(), nil, nil)
	e.SetText("0123456789")
	e.MoveCursor(8, false)
	// Host-style edit ahead of the caret: caret shifts with the text.
	e.ReplaceRange(0, 4, "ab")
	if e.Cursor != 6 {
		t.Errorf("cursor = %d, want 6", e.Cursor)
	}
}
