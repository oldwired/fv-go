package terminal

import (
	"testing"
)

// newTestTerminal builds a minimal Terminal sized w×h with len lines
// of scrollback already populated. Each scrollback line says "scroll
// N" so extracted text is identifiable.
func newTestTerminal(w, h, scrollLines int) *Terminal {
	t := &Terminal{}
	t.buf = newBuffer(w, h)
	t.Size.X = w
	t.Size.Y = h
	for i := 0; i < scrollLines; i++ {
		t.buf.scrollback = append(t.buf.scrollback, makeRow("scroll N"))
	}
	return t
}

// TestEnterCopyModeParksAtBottomRight: cursor lands at the visible
// viewport's bottom-right cell.
func TestEnterCopyModeParksAtBottomRight(t *testing.T) {
	tm := newTestTerminal(20, 5, 10)
	// scrollOffset = 0 → viewport shows live region. Top abs row =
	// len(scrollback) = 10. Bottom abs = 10 + 5 - 1 = 14.
	tm.EnterCopyMode()
	if !tm.copying {
		t.Fatal("EnterCopyMode did not set copying=true")
	}
	wantRow := 14
	wantCol := 19
	if tm.copyCursor.row != wantRow || tm.copyCursor.col != wantCol {
		t.Errorf("copy cursor = (%d, %d), want (%d, %d)",
			tm.copyCursor.row, tm.copyCursor.col, wantRow, wantCol)
	}
}

// TestMoveCopyCursorClampsVertically: cursor cannot go below the last
// live row or above scrollback row 0.
func TestMoveCopyCursorClampsVertically(t *testing.T) {
	tm := newTestTerminal(20, 5, 10)
	tm.EnterCopyMode()
	// Try to move below — clamps to maxRow = 14.
	tm.MoveCopyCursor(0, 100)
	if tm.copyCursor.row != 14 {
		t.Errorf("cursor row after huge down = %d, want 14 (clamped)", tm.copyCursor.row)
	}
	// Way up: clamps to 0.
	tm.MoveCopyCursor(0, -1000)
	if tm.copyCursor.row != 0 {
		t.Errorf("cursor row after huge up = %d, want 0 (clamped)", tm.copyCursor.row)
	}
}

// TestMoveCopyCursorClampsHorizontally: no wrap to next line.
func TestMoveCopyCursorClampsHorizontally(t *testing.T) {
	tm := newTestTerminal(20, 5, 10)
	tm.EnterCopyMode()
	// Already at col 19 (right edge). Move further right: clamps.
	tm.MoveCopyCursor(50, 0)
	if tm.copyCursor.col != 19 {
		t.Errorf("col after huge right = %d, want 19 (clamped)", tm.copyCursor.col)
	}
	startRow := tm.copyCursor.row
	if tm.copyCursor.row != startRow {
		t.Errorf("horizontal motion shifted row to %d", tm.copyCursor.row)
	}
	tm.MoveCopyCursor(-1000, 0)
	if tm.copyCursor.col != 0 {
		t.Errorf("col after huge left = %d, want 0 (clamped)", tm.copyCursor.col)
	}
}

// TestMoveCopyCursorAdjustsScroll: moving the cursor above the
// viewport top pulls history into view (scrollOffset rises).
func TestMoveCopyCursorAdjustsScroll(t *testing.T) {
	tm := newTestTerminal(20, 5, 10)
	tm.EnterCopyMode()
	if tm.buf.scrollOffset != 0 {
		t.Fatalf("initial scrollOffset = %d, want 0", tm.buf.scrollOffset)
	}
	// Move up 8 — cursor goes from row 14 → 6. Top of viewport was
	// row 10; cursor at 6 is 4 rows above, so scrollOffset becomes 4.
	tm.MoveCopyCursor(0, -8)
	if tm.copyCursor.row != 6 {
		t.Fatalf("cursor row = %d, want 6", tm.copyCursor.row)
	}
	if tm.buf.scrollOffset != 4 {
		t.Errorf("scrollOffset = %d, want 4 (cursor pulled history)", tm.buf.scrollOffset)
	}
}

// TestToggleCopyAnchorPinsAndExtends: first toggle anchors, second
// move extends, second toggle clears.
func TestToggleCopyAnchorPinsAndExtends(t *testing.T) {
	tm := newTestTerminal(20, 5, 10)
	tm.EnterCopyMode()
	anchorRow := tm.copyCursor.row
	anchorCol := tm.copyCursor.col
	tm.ToggleCopyAnchor()
	if !tm.selecting {
		t.Fatal("ToggleCopyAnchor did not set selecting=true")
	}
	if tm.selStartAbs.row != anchorRow || tm.selStartAbs.col != anchorCol {
		t.Errorf("anchor at %+v, want at %+v", tm.selStartAbs, cellPos{anchorRow, anchorCol})
	}
	// Move cursor — selEndAbs should follow.
	tm.MoveCopyCursor(-3, 0)
	if tm.selEndAbs != tm.copyCursor {
		t.Errorf("selEndAbs = %+v, want = copyCursor (%+v)", tm.selEndAbs, tm.copyCursor)
	}
	// Re-toggle clears.
	tm.ToggleCopyAnchor()
	if tm.selecting {
		t.Error("re-toggle did not clear selecting")
	}
	if tm.selStartAbs != (cellPos{}) || tm.selEndAbs != (cellPos{}) {
		t.Error("re-toggle did not clear anchor/endpoint")
	}
}

// TestCopySelectionRoundtrip: anchor + move + CopySelection returns
// the right substring.
func TestCopySelectionRoundtrip(t *testing.T) {
	tm := newTestTerminal(20, 5, 10)
	tm.EnterCopyMode()
	// Place cursor at row 10 (top of live region), col 0. Anchor.
	tm.copyCursor = cellPos{row: 10, col: 0}
	tm.ToggleCopyAnchor()
	// Move to (row 10, col 5) — selecting "scroll" (6 chars).
	// Wait: row 10 is the first live row; the live cells are blank
	// space (newBuffer fills with blankCell). Let me anchor inside
	// scrollback instead — row 5 (which is "scroll N") cols 0..5.
	tm.selStartAbs = cellPos{row: 5, col: 0}
	tm.copyCursor = cellPos{row: 5, col: 5}
	tm.selEndAbs = tm.copyCursor
	got, ok := tm.CopySelection()
	if !ok {
		t.Fatal("CopySelection returned ok=false")
	}
	if got != "scroll" {
		t.Errorf("CopySelection = %q, want %q", got, "scroll")
	}
	// Copy mode is NOT exited by CopySelection.
	if !tm.copying {
		t.Error("CopySelection unexpectedly exited copy mode")
	}
}

// TestExitCopyModePreservesSelection: ExitCopyMode clears the cursor
// flag but leaves the selection range intact.
func TestExitCopyModePreservesSelection(t *testing.T) {
	tm := newTestTerminal(20, 5, 10)
	tm.EnterCopyMode()
	tm.ToggleCopyAnchor()
	tm.MoveCopyCursor(-3, 0)
	startBefore := tm.selStartAbs
	endBefore := tm.selEndAbs
	tm.ExitCopyMode()
	if tm.copying {
		t.Error("ExitCopyMode did not clear copying")
	}
	if tm.selStartAbs != startBefore || tm.selEndAbs != endBefore {
		t.Error("ExitCopyMode mutated the selection state")
	}
}

// TestMoveCopyCursorIgnoredWhenNotCopying confirms the methods are
// no-ops outside copy mode.
func TestMoveCopyCursorIgnoredWhenNotCopying(t *testing.T) {
	tm := newTestTerminal(20, 5, 10)
	// Not in copy mode.
	tm.copyCursor = cellPos{row: 5, col: 5}
	tm.MoveCopyCursor(1, 1)
	if tm.copyCursor != (cellPos{row: 5, col: 5}) {
		t.Errorf("MoveCopyCursor mutated state outside copy mode: %+v", tm.copyCursor)
	}
}
