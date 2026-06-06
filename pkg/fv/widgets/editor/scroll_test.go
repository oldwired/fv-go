package editor

import (
	"strings"
	"testing"

	utf8Pkg "github.com/oldwired/fv-go/pkg/fv/utf8"
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

// TestViewStateRoundTripPreservesCursorAndScroll verifies the
// snapshot/restore pair preserves the user's caret + scroll across a
// SetText call. The env-swap A→B→A flow needs this so the user
// doesn't get teleported to line 1 of a buffer they just left.
func TestViewStateRoundTripPreservesCursorAndScroll(t *testing.T) {
	e := newTestEditor()
	original := ""
	for i := 0; i < 60; i++ {
		original += "line\n"
	}
	e.SetText(original)
	e.MoveCursor(125, false) // somewhere in the middle
	e.Top = 12
	e.LeftCol = 4
	want := e.ViewState()

	// Swap to a different buffer, then back.
	e.SetText("something else\n")
	if e.Cursor != 0 || e.Top != 0 {
		t.Fatalf("SetText didn't reset view state — test premise broken")
	}
	e.SetText(original)
	e.RestoreViewState(want)

	if e.Cursor != want.Cursor {
		t.Errorf("Cursor: got %d, want %d", e.Cursor, want.Cursor)
	}
	if e.Top != want.Top {
		t.Errorf("Top: got %d, want %d", e.Top, want.Top)
	}
	if e.LeftCol != want.LeftCol {
		t.Errorf("LeftCol: got %d, want %d", e.LeftCol, want.LeftCol)
	}
}

// TestRestoreViewStateClampsToShorterBuffer: snapshotting against a
// large buffer then restoring against a smaller one must not produce
// out-of-range Cursor or Top values (the editor would panic on the
// first MoveCursor / Draw otherwise).
func TestRestoreViewStateClampsToShorterBuffer(t *testing.T) {
	e := newTestEditor()
	e.SetText("aaaaaaaaaaaaaaaaaaaaaaa\nbbbbbbb\ncccccc")
	v := ViewState{Cursor: 1000, SelAnchor: 500, Top: 1000, LeftCol: 100}
	e.SetText("short")
	e.RestoreViewState(v)
	if e.Cursor > e.Len() {
		t.Errorf("Cursor not clamped: got %d, len=%d", e.Cursor, e.Len())
	}
	if e.Top > e.LineCount() {
		t.Errorf("Top not clamped: got %d, LineCount=%d", e.Top, e.LineCount())
	}
}

// TestAppendDoesNotMoveCursorWhenNotAtTail: the transcript-pane use
// case wants Append to leave the user's cursor alone unless they
// happen to be parked at the end (in which case the cursor follows
// the new tail).
func TestAppendDoesNotMoveCursorWhenNotAtTail(t *testing.T) {
	e := newTestEditor()
	e.SetText("hello\nworld")
	e.MoveCursor(2, false)
	startCursor := e.Cursor

	e.Append("\nappended")
	if e.Cursor != startCursor {
		t.Errorf("Append moved cursor from %d to %d when not at tail", startCursor, e.Cursor)
	}
	if e.Text() != "hello\nworld\nappended" {
		t.Errorf("buffer = %q, want %q", e.Text(), "hello\nworld\nappended")
	}
}

// TestAppendFollowsTailWhenAtEnd: a stick-to-bottom transcript reader
// (cursor parked at len(Data)) should track the new content.
func TestAppendFollowsTailWhenAtEnd(t *testing.T) {
	e := newTestEditor()
	e.SetText("a")
	e.MoveCursor(e.Len(), false)
	e.Append("bc")
	if e.Cursor != 3 {
		t.Errorf("Append at tail: Cursor = %d, want 3", e.Cursor)
	}
}

// TestAppendRoutesUndo: Append goes through applyChange so Undo
// reverses it. Hosts that don't want unbounded undo for log streams
// call ResetUndo periodically.
func TestAppendRoutesUndo(t *testing.T) {
	e := newTestEditor()
	e.SetText("base")
	e.Append("+more")
	if e.Text() != "base+more" {
		t.Fatalf("Append did not concatenate: %q", e.Text())
	}
	e.Undo()
	if e.Text() != "base" {
		t.Errorf("Undo after Append: %q, want %q", e.Text(), "base")
	}
}

// TestEncodingGetterSetter: the encoding field used to be unexported
// with a docstring telling callers to "set e.encoding before
// calling SaveFile" — impossible from outside the package. Now
// exposed via Encoding() / SetEncoding so the BOM-preservation
// path is actually reachable.
func TestEncodingGetterSetter(t *testing.T) {
	e := newTestEditor()
	if e.Encoding() != utf8Pkg.EncUTF8 {
		t.Errorf("default Encoding() = %v, want EncUTF8", e.Encoding())
	}
	e.SetEncoding(utf8Pkg.EncUTF8BOM)
	if e.Encoding() != utf8Pkg.EncUTF8BOM {
		t.Errorf("after SetEncoding(BOM): got %v, want EncUTF8BOM", e.Encoding())
	}
}

// TestAppendBypassesReadOnly: ReadOnly gates user-input mutators
// (Insert from typed keys, Backspace, etc.) but not host-driven
// content APIs. A transcript pane should be able to keep
// ReadOnly=true and still receive programmatic Append calls so the
// user can't type into the streamed output. This matches SetText's
// existing semantics.
func TestAppendBypassesReadOnly(t *testing.T) {
	e := newTestEditor()
	e.SetText("base")
	e.ReadOnly = true

	e.Append("+streamed")
	if e.Text() != "base+streamed" {
		t.Errorf("Append should bypass ReadOnly (matching SetText): got %q, want %q",
			e.Text(), "base+streamed")
	}

	// Insert (user-input path) should still be blocked.
	prev := e.Text()
	e.Insert("typed")
	if e.Text() != prev {
		t.Errorf("Insert under ReadOnly mutated buffer: got %q, want %q", e.Text(), prev)
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
