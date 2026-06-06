package editor

import (
	"path/filepath"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
)

// Regression tests for the post-feature code-review findings.

func TestReformatCollapsesSecondaryCarets(t *testing.T) {
	e := newTestEditor()
	e.RightMargin = 80
	e.SetText("aaa      bbb") // reformat shrinks the buffer
	e.MoveCursor(0, false)
	e.AddCaret(11)
	e.Reformat()
	if e.HasMultipleCarets() {
		t.Fatal("Reformat must collapse secondary carets (stale offsets corrupt the next edit)")
	}
	// Reformat parks the caret at the paragraph end; the next insert
	// must land exactly once (a stale duplicate caret would double it
	// or panic on the shrunken buffer).
	e.Insert("X")
	if got := e.Text(); got != "aaa bbbX" {
		t.Errorf("post-Reformat insert = %q, want single trailing X", got)
	}
}

func TestTrimTrailingWSPreservesAnchors(t *testing.T) {
	e := newTestEditor()
	e.SetText("hello  \nworld\nfoo  ")
	e.bookmarks[1] = 8 // 'w' of world
	e.SetDecorations("hl", []Decoration{{Start: 8, End: 13, Attr: 7}})
	e.AddCaret(10)
	e.TrimTrailingWS()
	if got := e.Text(); got != "hello\nworld\nfoo" {
		t.Fatalf("trim result: %q", got)
	}
	if e.bookmarks[1] != 6 {
		t.Errorf("bookmark = %d, want 6 (shifted, not wiped)", e.bookmarks[1])
	}
	if d := e.Decorations("hl"); len(d) != 1 || d[0].Start != 6 || d[0].End != 11 {
		t.Errorf("decoration = %+v, want [{6 11 7}]", d)
	}
	if e.HasMultipleCarets() {
		t.Error("TrimTrailingWS must collapse secondary carets")
	}
	e.Undo()
	if got := e.Text(); got != "hello  \nworld\nfoo  " {
		t.Errorf("one undo must restore all trims: %q", got)
	}
}

func TestReplaceAllPreservesAnchorsAndPanes(t *testing.T) {
	a, b := newSharedPair(numberedLines(20))
	a.SetFoldRegions([]FoldRegion{{StartLine: 2, EndLine: 5}, {StartLine: 8, EndLine: 11}})
	a.Fold(2)
	a.SetDecorations("hl", []Decoration{{Start: 70, End: 76, Attr: 7}})
	a.bookmarks[3] = 45
	b.MoveCursor(45, false)

	n := a.ReplaceAll("line01", "LINE01", true)
	if n != 1 {
		t.Fatalf("ReplaceAll = %d, want 1", n)
	}
	regions := a.FoldRegions()
	if len(regions) != 2 || regions[0].StartLine != 2 || regions[1].StartLine != 8 {
		t.Errorf("folds after ReplaceAll = %+v, want both intact", regions)
	}
	if !a.IsFolded(2) {
		t.Error("collapsed state must survive ReplaceAll of untouched text")
	}
	if d := a.Decorations("hl"); len(d) != 1 || d[0].Start != 70 {
		t.Errorf("decoration after ReplaceAll = %+v", d)
	}
	if a.bookmarks[3] != 45 {
		t.Errorf("bookmark = %d, want 45", a.bookmarks[3])
	}
	if b.Cursor != 45 {
		t.Errorf("sibling cursor = %d, want 45 (must not teleport to 0)", b.Cursor)
	}
	a.Undo()
	if got := a.Text(); got != numberedLines(20) {
		t.Errorf("one undo must revert all replacements")
	}
}

func TestReplaceAllReadOnlyNoOp(t *testing.T) {
	e := newTestEditor()
	e.SetText("foo bar foo")
	e.ReadOnly = true
	if n := e.ReplaceAll("foo", "XXX", true); n != 0 {
		t.Errorf("ReadOnly ReplaceAll = %d, want 0", n)
	}
	if got := e.Text(); got != "foo bar foo" {
		t.Errorf("ReadOnly editor mutated: %q", got)
	}
}

func TestSpliceClampsStartPastEOF(t *testing.T) {
	b := NewBuffer()
	b.SetText("abc")
	b.splice(nil, 99, 120, []byte("x"), -1, -1) // must not panic
	if got := b.Text(); got != "abcx" {
		t.Errorf("clamped splice = %q, want abcx", got)
	}
}

func TestSetCaretsClampsSingleCaret(t *testing.T) {
	e := newTestEditor()
	e.SetText("hello")
	e.SetCarets([]Caret{{Pos: 999, Anchor: -1}})
	if e.Cursor != 5 {
		t.Fatalf("cursor = %d, want 5 (clamped)", e.Cursor)
	}
	e.Insert("!") // must not panic
	if got := e.Text(); got != "hello!" {
		t.Errorf("text = %q", got)
	}
}

func TestCrossPaneTypingDoesNotCoalesce(t *testing.T) {
	a, b := newSharedPair("xy")
	a.MoveCursor(1, false)
	a.Insert("a")
	b.MoveCursor(2, false) // adjacent to A's insert end
	b.Insert("b")
	if got := a.Text(); got != "xaby" {
		t.Fatalf("text = %q", got)
	}
	b.Undo()
	if got := a.Text(); got != "xay" {
		t.Errorf("undo must revert only B's keystroke: %q", got)
	}
}

func TestTypingAndReplaceRangeDoNotCoalesce(t *testing.T) {
	e := newTestEditor()
	e.MoveCursor(0, false)
	e.Insert("a")
	e.ReplaceRange(1, 1, "b") // adjacent host edit (LSP completion shape)
	if got := e.Text(); got != "ab" {
		t.Fatalf("text = %q", got)
	}
	e.Undo()
	if got := e.Text(); got != "a" {
		t.Errorf("undo must keep the typed prefix: %q", got)
	}
}

func TestOnChangeSeesPostEditCursor(t *testing.T) {
	e := newTestEditor()
	e.SetText("hello world, long enough to matter")
	var bad []int
	e.OnChange = func(int) {
		if e.Cursor > e.Len() {
			bad = append(bad, e.Cursor)
		}
	}
	e.SelectAll()
	e.Backspace() // shrinks to 0; handler must never see the stale caret
	if len(bad) > 0 {
		t.Errorf("OnChange observed cursor past EOF: %v", bad)
	}
	if e.Cursor != 0 {
		t.Errorf("cursor = %d, want 0", e.Cursor)
	}

	// Multi-caret group: the single OnChange fires with final carets.
	e.SetText("a b c")
	var seen int
	e.OnChange = func(int) {
		seen = e.Cursor
		if e.Cursor > e.Len() {
			t.Errorf("multi-caret OnChange: cursor %d past len %d", e.Cursor, e.Len())
		}
	}
	e.MoveCursor(1, false)
	e.AddCaret(3)
	e.AddCaret(5)
	e.Insert("X")
	if seen != e.Cursor {
		t.Errorf("OnChange saw cursor %d, final is %d", seen, e.Cursor)
	}

	// Undo path.
	e.OnChange = func(int) {
		if e.Cursor > e.Len() {
			t.Errorf("undo OnChange: cursor %d past len %d", e.Cursor, e.Len())
		}
	}
	e.Undo()
}

func TestForeignSpliceCollapsesSnippetMirrors(t *testing.T) {
	a, b := newSharedPair("base ")
	b.MoveCursor(5, false)
	_ = b.InsertSnippet("${1:foo}+${1}")
	if !b.HasMultipleCarets() {
		t.Fatal("setup: mirrors expected")
	}
	a.MoveCursor(0, false)
	a.Insert("!")
	if b.SnippetActive() {
		t.Error("foreign splice must end the session")
	}
	if b.HasMultipleCarets() {
		t.Error("dead session must not leave mirror carets behind")
	}
}

func TestToggleCaretAtSelectionBoundary(t *testing.T) {
	e := newTestEditor()
	e.SetText("hello world")
	e.SelAnchor = 0
	e.Cursor = 5
	e.toggleCaretAt(5) // hi end of the primary selection
	if e.HasMultipleCarets() {
		t.Fatal("toggle at a selection boundary must not add a coexisting caret")
	}
	e.Insert("X")
	if got := e.Text(); got != "X world" {
		t.Errorf("typing doubled: %q", got)
	}
}

func TestNormalizeMergesBoundaryZeroWidthCaret(t *testing.T) {
	e := newTestEditor()
	e.SetText("hello world")
	e.SetCarets([]Caret{{Pos: 5, Anchor: 0}, {Pos: 5, Anchor: -1}})
	if len(e.Carets()) != 1 {
		t.Errorf("carets = %+v, want the zero-width one absorbed", e.Carets())
	}
}

func TestAddCaretVerticallySkipsFoldedLines(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(10))
	e.SetFoldRegions([]FoldRegion{{StartLine: 1, EndLine: 4}})
	e.Fold(1)
	e.MoveCursor(e.posAtVisible(1, 2), false) // on the collapsed header
	e.addCaretVertically(1)
	cs := e.Carets()
	if len(cs) != 2 {
		t.Fatalf("carets = %d, want 2", len(cs))
	}
	if got := e.lineNumber(cs[1].Pos); got != 5 {
		t.Errorf("new caret on line %d, want 5 (next visible)", got)
	}
}

func TestColumnSelectSkipsFoldedLines(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(10))
	e.SetFoldRegions([]FoldRegion{{StartLine: 1, EndLine: 4}})
	e.Fold(1)
	e.ColumnSelect(0, 5, 0, 2)
	cs := e.Carets()
	if len(cs) != 3 {
		t.Fatalf("carets = %d, want 3 (lines 0, 1, 5 — hidden 2-4 skipped)", len(cs))
	}
	for _, c := range cs {
		if !e.IsLineVisible(e.lineNumber(c.Pos)) {
			t.Errorf("caret on hidden line %d", e.lineNumber(c.Pos))
		}
	}
}

func TestArrowsSkipFoldedRegion(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(10))
	e.SetFoldRegions([]FoldRegion{{StartLine: 1, EndLine: 4}})
	e.Fold(1)
	// Right at the collapsed header's EOL crosses into hidden line 2 —
	// must land at the start of the next visible line instead.
	_, headerEnd := e.lineByIndex(1)
	e.MoveCursor(headerEnd, false)
	e.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbRight})
	ls5, _ := e.lineByIndex(5)
	if e.Cursor != ls5 {
		t.Errorf("Right across fold: cursor = %d, want %d (start of line 5)", e.Cursor, ls5)
	}
	// Left from there lands back at the header's EOL.
	e.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbLeft})
	if e.Cursor != headerEnd {
		t.Errorf("Left across fold: cursor = %d, want %d (header EOL)", e.Cursor, headerEnd)
	}
}

func TestFindUnfoldsHiddenMatch(t *testing.T) {
	e := newTestEditor()
	e.SetText("top\nheader\nhidden needle here\nmore\ntail")
	e.SetFoldRegions([]FoldRegion{{StartLine: 1, EndLine: 3}})
	e.Fold(1)
	e.MoveCursor(0, false)
	if !e.Find("needle", true) {
		t.Fatal("Find must locate the hidden match")
	}
	if e.IsFolded(1) {
		t.Error("Find must unfold the region hiding the match")
	}
	if !e.IsLineVisible(e.lineNumber(e.Cursor)) {
		t.Error("selection must be visible after Find")
	}
}

func TestFindOutsideSnippetEndsSession(t *testing.T) {
	e := newTestEditor()
	e.SetText(" find me")
	e.MoveCursor(0, false)
	_ = e.InsertSnippet("${1:ab}")
	if !e.SnippetActive() {
		t.Fatal("setup: session expected")
	}
	if !e.Find("find", true) {
		t.Fatal("Find failed")
	}
	if e.SnippetActive() {
		t.Error("Find jumping outside snippet bounds must end the session")
	}
}

func TestReadOnlyPaneCannotUndoSharedBuffer(t *testing.T) {
	a, b := newSharedPair("base")
	a.MoveCursor(4, false)
	a.Insert("!")
	b.ReadOnly = true
	b.Undo()
	if got := a.Text(); got != "base!" {
		t.Errorf("ReadOnly pane reverted a shared edit: %q", got)
	}
	a.Undo()
	b.Redo()
	if got := a.Text(); got != "base" {
		t.Errorf("ReadOnly pane re-applied a shared edit: %q", got)
	}
}

func TestSnippetTerminalStopDoesNotAbsorbTyping(t *testing.T) {
	e := newTestEditor()
	_ = e.InsertSnippet("${1:x}$0")
	e.Insert("a")
	e.Insert("b")
	e.HandleEvent(key(consts.KbTab))
	if e.SnippetActive() {
		t.Error("session must end at $0")
	}
	if e.HasSelection() {
		lo, hi := e.selRange()
		t.Errorf("$0 selected %q — it must be an empty caret", string(e.Buf.data[lo:hi]))
	}
	if e.Cursor != 2 {
		t.Errorf("cursor = %d, want 2 (end of typed text)", e.Cursor)
	}
}

type recordingListener struct{ saved int }

func (r *recordingListener) bufferSpliced(Splice) {}
func (r *recordingListener) bufferReset()         {}
func (r *recordingListener) bufferChanged(int)    {}
func (r *recordingListener) bufferSaved()         { r.saved++ }

func TestSavePointModified(t *testing.T) {
	b := NewBuffer()
	rec := &recordingListener{}
	b.addListener(rec)
	who := &struct{}{}
	b.SetText("abc")
	b.splice(who, 3, 3, []byte("d"), -1, -1)
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := b.SaveFile(path); err != nil {
		t.Fatal(err)
	}
	if rec.saved != 1 {
		t.Errorf("bufferSaved fired %d times, want 1", rec.saved)
	}
	if b.Modified {
		t.Fatal("Modified after save")
	}
	b.Undo()
	if !b.Modified {
		t.Error("undo past the save point must report modified")
	}
	b.Redo()
	if b.Modified {
		t.Error("redo back onto the save point must report unmodified")
	}
	// Typing right after a save must not merge into the saved entry.
	b.splice(who, 4, 4, []byte("e"), -1, -1)
	if len(b.undoStack) != 2 {
		t.Errorf("undo entries = %d, want 2 (save point is a coalesce barrier)", len(b.undoStack))
	}
	b.Undo()
	if got := b.Text(); got != "abcd" {
		t.Errorf("undo after post-save typing = %q, want the saved state", got)
	}
	if b.Modified {
		t.Error("back at the save point must be unmodified")
	}
}

func TestSetTextClearsBookmarks(t *testing.T) {
	e := newTestEditor()
	e.SetText("hello world")
	e.bookmarks[2] = 6
	e.SetText("fresh")
	if e.bookmarks[2] != -1 {
		t.Errorf("bookmark survived SetText: %d", e.bookmarks[2])
	}
}
