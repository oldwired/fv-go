package editor

import (
	"strings"
	"testing"
)

// TestScrollDoesNotMoveCursor verifies Editor.Scroll shifts the
// viewport (Top) without disturbing Cursor. Synthesizing arrow keys
// to scroll would also move the caret — wrong UX for a wheel scroll,
// which should let the text move under a stationary caret.
func TestScrollDoesNotMoveCursor(t *testing.T) {
	e := newTestEditor()
	e.SetText(strings.Repeat("line\n", 50))
	startCursor := e.Cursor

	e.Scroll(5)
	if e.Top != 5 {
		t.Errorf("Scroll(5): Top = %d, want 5", e.Top)
	}
	if e.Cursor != startCursor {
		t.Errorf("Scroll(5): Cursor moved from %d to %d", startCursor, e.Cursor)
	}

	e.Scroll(-3)
	if e.Top != 2 {
		t.Errorf("Scroll(-3): Top = %d, want 2", e.Top)
	}
	if e.Cursor != startCursor {
		t.Errorf("Scroll(-3): Cursor moved from %d to %d", startCursor, e.Cursor)
	}
}

// TestScrollClampsToBounds: Top stays in [0, LineCount-1].
func TestScrollClampsToBounds(t *testing.T) {
	e := newTestEditor()
	e.SetText("a\nb\nc\n")
	// LineCount = 4 (last empty line counts).
	e.Scroll(1000)
	if want := e.LineCount() - 1; e.Top != want {
		t.Errorf("Scroll(big): Top = %d, want %d (LineCount - 1)", e.Top, want)
	}
	e.Scroll(-1000)
	if e.Top != 0 {
		t.Errorf("Scroll(-big): Top = %d, want 0", e.Top)
	}
}

// TestOnChangeFires verifies that buffer mutations route through
// applyChange / Undo / Redo / SetText and emit OnChange with a
// monotonically increasing version.
func TestOnChangeFires(t *testing.T) {
	e := newTestEditor()
	var versions []int
	e.OnChange = func(v int) { versions = append(versions, v) }

	e.SetText("hi")
	e.Insert("!")
	e.Backspace()
	e.Undo()
	e.Redo()

	if len(versions) < 4 {
		t.Fatalf("OnChange fired %d times, want at least 4 (SetText, Insert, Backspace, Undo/Redo); got %v",
			len(versions), versions)
	}
	for i := 1; i < len(versions); i++ {
		if versions[i] <= versions[i-1] {
			t.Errorf("OnChange version regressed: %v (versions[%d] <= versions[%d])", versions, i, i-1)
		}
	}
}
