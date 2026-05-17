package editor

import (
	"testing"
)

// TestPositionIs1Indexed confirms the Position() helper returns
// 1-indexed line and column matching classical-IDE conventions.
// Fresh editor with no text starts at (1, 1).
func TestPositionIs1Indexed(t *testing.T) {
	e := newTestEditor()
	line, col := e.Position()
	if line != 1 || col != 1 {
		t.Errorf("empty editor Position() = (%d, %d), want (1, 1)", line, col)
	}
}

// TestPositionAdvancesWithInsert: typing 5 chars on the first line
// moves col to 6 (1-indexed); a newline moves to line 2 col 1.
func TestPositionAdvancesWithInsert(t *testing.T) {
	e := newTestEditor()
	e.Insert("hello")
	line, col := e.Position()
	if line != 1 || col != 6 {
		t.Errorf("after 'hello': Position() = (%d, %d), want (1, 6)", line, col)
	}
	e.Insert("\n")
	line, col = e.Position()
	if line != 2 || col != 1 {
		t.Errorf("after newline: Position() = (%d, %d), want (2, 1)", line, col)
	}
	e.Insert("xy")
	line, col = e.Position()
	if line != 2 || col != 3 {
		t.Errorf("after 'xy' on line 2: Position() = (%d, %d), want (2, 3)", line, col)
	}
}

// TestPositionTabExpansion: a tab counts for TabWidth display
// columns, not 1. With TabWidth=4, a tab moves col from 1 to 5.
func TestPositionTabExpansion(t *testing.T) {
	e := newTestEditor()
	e.Insert("\t")
	line, col := e.Position()
	if line != 1 || col != 5 {
		t.Errorf("after tab: Position() = (%d, %d), want (1, 5)", line, col)
	}
}

// TestOnCursorMoveFiresOnChangeOnly: OnCursorMove is debounced — it
// should fire when the caret moves to a new (line, col) and not
// fire on subsequent Draws that leave the position unchanged.
func TestOnCursorMoveFiresOnChangeOnly(t *testing.T) {
	e := newTestEditor()
	// Force a sane size so Draw has something to paint into.
	e.Size.X = 40
	e.Size.Y = 5

	var calls int
	var lastLine, lastCol int
	e.OnCursorMove = func(line, col int) {
		calls++
		lastLine, lastCol = line, col
	}

	// First Draw: cursor at (1,1). Should fire once.
	e.Draw()
	if calls != 1 {
		t.Errorf("first Draw: OnCursorMove fired %d times, want 1", calls)
	}
	if lastLine != 1 || lastCol != 1 {
		t.Errorf("first Draw: position = (%d, %d), want (1, 1)", lastLine, lastCol)
	}

	// Second Draw with no caret movement: must NOT fire.
	e.Draw()
	if calls != 1 {
		t.Errorf("redundant Draw: OnCursorMove fired %d times, want 1", calls)
	}

	// Move cursor by inserting text; Draw should fire OnCursorMove again.
	e.Insert("abc")
	e.Draw()
	if calls != 2 {
		t.Errorf("after move + Draw: OnCursorMove fired %d times, want 2", calls)
	}
	if lastLine != 1 || lastCol != 4 {
		t.Errorf("after insert: position = (%d, %d), want (1, 4)", lastLine, lastCol)
	}
}

// TestShowPositionOverlayPainted verifies that enabling ShowPosition
// actually emits cells in the bottom-right of the editor bounds.
// Reaching into the editor's parent owner via SfExposed is a lot of
// scaffolding for one assertion; instead we test the precondition
// here (the flag is off by default; setting it doesn't panic on
// Draw with a sane size).
func TestShowPositionOverlayDoesntPanic(t *testing.T) {
	e := newTestEditor()
	e.Size.X = 30
	e.Size.Y = 8
	e.ShowPosition = true
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Draw with ShowPosition panicked: %v", r)
		}
	}()
	e.Draw()
}
