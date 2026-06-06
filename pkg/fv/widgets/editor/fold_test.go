package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

func numberedLines(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		_, _ = fmt.Fprintf(&sb, "line%02d\n", i) // strings.Builder never errors
	}
	return sb.String()
}

func TestFoldMappingIdentityWithoutFolds(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(20))
	e.Top = 3
	if got := e.LineAtRow(2); got != 5 {
		t.Errorf("LineAtRow(2) = %d, want 5", got)
	}
	if got := e.RowOfLine(5); got != 2 {
		t.Errorf("RowOfLine(5) = %d, want 2", got)
	}
	if got := e.VisibleLineCount(); got != e.LineCount() {
		t.Errorf("VisibleLineCount = %d, want %d", got, e.LineCount())
	}
}

func TestFoldBasicMapping(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(20))
	e.SetFoldRegions([]FoldRegion{{StartLine: 2, EndLine: 5}})
	e.Fold(2)
	if !e.IsFolded(2) {
		t.Fatal("Fold(2) did not collapse")
	}
	// Lines 3..5 hidden: rows are 0,1,2(=line2),3(=line6),...
	if got := e.LineAtRow(3); got != 6 {
		t.Errorf("LineAtRow(3) = %d, want 6", got)
	}
	if got := e.RowOfLine(6); got != 3 {
		t.Errorf("RowOfLine(6) = %d, want 3", got)
	}
	if got := e.RowOfLine(4); got != -1 {
		t.Errorf("RowOfLine(hidden 4) = %d, want -1", got)
	}
	if !e.IsLineVisible(2) || e.IsLineVisible(3) || e.IsLineVisible(5) || !e.IsLineVisible(6) {
		t.Error("visibility wrong around the fold")
	}
	if got := e.VisibleLineCount(); got != e.LineCount()-3 {
		t.Errorf("VisibleLineCount = %d, want %d", got, e.LineCount()-3)
	}
	e.Unfold(2)
	if got := e.LineAtRow(3); got != 3 {
		t.Errorf("after Unfold LineAtRow(3) = %d, want 3", got)
	}
}

func TestNestedFolds(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(20))
	e.SetFoldRegions([]FoldRegion{
		{StartLine: 2, EndLine: 10},
		{StartLine: 4, EndLine: 6},
	})
	e.Fold(4)
	if e.IsLineVisible(5) {
		t.Error("inner fold must hide line 5")
	}
	if !e.IsLineVisible(8) {
		t.Error("line 8 outside inner fold must stay visible")
	}
	e.Fold(2)
	if e.IsLineVisible(8) || e.IsLineVisible(4) {
		t.Error("outer fold must hide everything inside, including the inner header")
	}
	// Unfolding the outer reveals the inner header, still collapsed.
	e.Unfold(2)
	if !e.IsLineVisible(4) {
		t.Error("inner header visible after outer unfold")
	}
	if e.IsLineVisible(5) {
		t.Error("inner fold must stay collapsed after outer unfold")
	}
}

func TestFoldOverlapNotNestedRejected(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(20))
	e.SetFoldRegions([]FoldRegion{
		{StartLine: 2, EndLine: 8},
		{StartLine: 5, EndLine: 12}, // overlaps, not nested
	})
	if got := len(e.FoldRegions()); got != 1 {
		t.Fatalf("regions kept = %d, want 1 (overlap dropped)", got)
	}
	if r := e.FoldRegions()[0]; r.StartLine != 2 || r.EndLine != 8 {
		t.Errorf("kept region = %+v, want {2 8}", r)
	}
}

func TestFoldAtEOF(t *testing.T) {
	for _, trailing := range []string{"", "\n"} {
		e := newTestEditor()
		e.SetText("a\nb\nc\nd" + trailing)
		e.SetFoldRegions([]FoldRegion{{StartLine: 1, EndLine: 3}})
		e.Fold(1)
		if e.IsLineVisible(2) || e.IsLineVisible(3) {
			t.Errorf("trailing=%q: lines 2,3 must hide", trailing)
		}
		if got := e.LineAtRow(2); got != -1 && trailing == "" {
			t.Errorf("trailing=%q: LineAtRow(2) = %d, want -1 (past EOF)", trailing, got)
		}
	}
}

func TestFoldNavigationSkipsHidden(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(20))
	e.SetFoldRegions([]FoldRegion{{StartLine: 2, EndLine: 5}})
	e.Fold(2)
	// Down from line 2 lands on line 6.
	e.MoveCursor(e.posAtVisible(2, 0), false)
	e.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbDown})
	if got := e.lineNumber(e.Cursor); got != 6 {
		t.Errorf("Down from header: line %d, want 6", got)
	}
	// Up from line 6 lands back on the header.
	e.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbUp})
	if got := e.lineNumber(e.Cursor); got != 2 {
		t.Errorf("Up to header: line %d, want 2", got)
	}
}

func TestFoldPgDnCountsVisibleLines(t *testing.T) {
	e := newTestEditor() // 10 rows
	e.SetText(numberedLines(40))
	e.SetFoldRegions([]FoldRegion{{StartLine: 1, EndLine: 8}})
	e.Fold(1)
	e.MoveCursor(0, false)
	e.HandleEvent(&drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbPgDn})
	// 10 visible lines down from 0: 1(header)=1, then 9..17 → line 17.
	if got := e.lineNumber(e.Cursor); got != 17 {
		t.Errorf("PgDn landed on line %d, want 17", got)
	}
}

func TestFoldClickMapsThroughFold(t *testing.T) {
	views.ResetForTest()
	e := newTestEditor()
	e.SetText(numberedLines(20))
	e.SetFoldRegions([]FoldRegion{{StartLine: 1, EndLine: 4}})
	e.Fold(1)
	// Row 2 shows line 5 (rows: 0→0, 1→1, 2→5).
	down := mouse(consts.EvMouseDown, 0, 2, 0)
	e.HandleEvent(&down)
	if got := e.lineNumber(e.Cursor); got != 5 {
		t.Errorf("click row 2 → line %d, want 5", got)
	}
}

func TestFoldCaretEvictedToHeader(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(20))
	e.MoveCursor(e.posAtVisible(4, 3), false)
	e.SetFoldRegions([]FoldRegion{{StartLine: 2, EndLine: 6}})
	e.Fold(2)
	if got := e.lineNumber(e.Cursor); got != 2 {
		t.Errorf("caret line after fold = %d, want 2 (header)", got)
	}
	_, le := e.lineByIndex(2)
	if e.Cursor != le {
		t.Errorf("caret = %d, want header line end %d", e.Cursor, le)
	}
}

func TestFoldMoveCursorIntoHiddenUnfolds(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(20))
	e.SetFoldRegions([]FoldRegion{{StartLine: 2, EndLine: 6}})
	e.Fold(2)
	e.MoveCursor(e.posAtVisible(4, 0), false)
	if e.IsFolded(2) {
		t.Error("moving the caret into a hidden line must unfold the region")
	}
	if !e.IsLineVisible(4) {
		t.Error("line 4 must be visible after auto-unfold")
	}
}

func TestFoldEditFromOtherPaneUnfolds(t *testing.T) {
	a, b := newSharedPair(numberedLines(20))
	b.SetFoldRegions([]FoldRegion{{StartLine: 2, EndLine: 6}})
	b.Fold(2)
	// A edits inside B's collapsed region (line 4).
	a.MoveCursor(a.posAtVisible(4, 2), false)
	a.Insert("X")
	if b.IsFolded(2) {
		t.Error("edit inside the hidden range must auto-unfold")
	}
	regions := b.FoldRegions()
	if len(regions) != 1 || regions[0].StartLine != 2 || regions[0].EndLine != 6 {
		t.Errorf("region after foreign edit = %+v, want [{2 6}]", regions)
	}
}

func TestFoldEditAboveShiftsRegion(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(20))
	e.SetFoldRegions([]FoldRegion{{StartLine: 5, EndLine: 9}})
	e.Fold(5)
	e.MoveCursor(0, false)
	e.Insert("new\nnew\n")
	regions := e.FoldRegions()
	if len(regions) != 1 || regions[0].StartLine != 7 || regions[0].EndLine != 11 {
		t.Fatalf("region after insert above = %+v, want [{7 11}]", regions)
	}
	if !e.IsFolded(7) {
		t.Error("region must stay collapsed when the edit is outside it")
	}
}

func TestFoldDeleteWholeRegionDropsIt(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(20))
	e.SetFoldRegions([]FoldRegion{{StartLine: 5, EndLine: 9}})
	start := e.Buf.OffsetAt(4, 0)
	end := e.Buf.OffsetAt(11, 0)
	e.ReplaceRange(start, end, "")
	if got := len(e.FoldRegions()); got != 0 {
		t.Errorf("regions after deleting the whole range = %d, want 0", got)
	}
}

func TestSetFoldRegionsPreservesCollapsedState(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(30))
	e.SetFoldRegions([]FoldRegion{{StartLine: 5, EndLine: 9}, {StartLine: 12, EndLine: 15}})
	e.Fold(5)
	// An edit above shifts the collapsed region to line 7; the host
	// re-supplies fresh (shifted) regions, as a gopls refresh would.
	e.MoveCursor(0, false)
	e.Insert("a\nb\n")
	if !e.IsFolded(7) {
		t.Fatal("collapsed region should have shifted to line 7")
	}
	e.SetFoldRegions([]FoldRegion{{StartLine: 7, EndLine: 11}, {StartLine: 14, EndLine: 17}})
	if !e.IsFolded(7) {
		t.Error("SetFoldRegions must preserve collapsed state by header line")
	}
	if e.IsFolded(14) {
		t.Error("never-collapsed region must stay expanded")
	}
}

func TestSetTextClearsFolds(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(20))
	e.SetFoldRegions([]FoldRegion{{StartLine: 2, EndLine: 5}})
	e.Fold(2)
	e.SetText("fresh\ncontent\n")
	if got := len(e.FoldRegions()); got != 0 {
		t.Errorf("folds after SetText = %d, want 0", got)
	}
}

func TestFoldScrollbarUsesVisibleLines(t *testing.T) {
	e := newTestEditor()
	e.VScroll = views.NewScrollBar(testRect())
	e.SetText(numberedLines(20))
	e.SetFoldRegions([]FoldRegion{{StartLine: 2, EndLine: 11}})
	e.Fold(2)
	e.refreshScroll()
	// 21 lines (trailing newline yields an empty last line) − 9 hidden.
	if got := e.VisibleLineCount(); got != 12 {
		t.Fatalf("VisibleLineCount = %d, want 12", got)
	}
	if got := e.VScroll.Max; got != 12 {
		t.Errorf("scrollbar max = %d, want 12 (visible-line space)", got)
	}
}

func TestFoldScrollStepsVisibleLines(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(30))
	e.SetFoldRegions([]FoldRegion{{StartLine: 1, EndLine: 8}})
	e.Fold(1)
	e.Scroll(2)
	// Visible: 0,1,9,10,... — two steps from 0 → 9.
	if e.Top != 9 {
		t.Errorf("Top after Scroll(2) = %d, want 9", e.Top)
	}
	e.Scroll(-1)
	if e.Top != 1 {
		t.Errorf("Top after Scroll(-1) = %d, want 1", e.Top)
	}
}

func TestFoldMarkerGlyphs(t *testing.T) {
	e := newTestEditor()
	e.SetText(numberedLines(20))
	e.SetFoldRegions([]FoldRegion{{StartLine: 2, EndLine: 5}})
	if got := e.FoldMarkerAt(2); got != '▾' {
		t.Errorf("expanded marker = %q, want ▾", got)
	}
	e.Fold(2)
	if got := e.FoldMarkerAt(2); got != '▸' {
		t.Errorf("collapsed marker = %q, want ▸", got)
	}
	if got := e.FoldMarkerAt(3); got != 0 {
		t.Errorf("non-header marker = %q, want 0", got)
	}
}

func TestFoldDrawRendersSummaryAndSkipsHidden(t *testing.T) {
	h := term.NewHeadless(40, 12)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	e := newTestEditor() // 40x10
	e.State |= consts.SfExposed | consts.SfVisible
	e.SetText(numberedLines(20))
	e.SetFoldRegions([]FoldRegion{{StartLine: 1, EndLine: 4}})
	e.Fold(1)
	e.Draw()
	_ = h.Flush()

	rows := strings.Split(h.Snapshot(), "\n")
	if !strings.HasPrefix(rows[0], "line00") {
		t.Errorf("row 0 = %q, want line00", rows[0])
	}
	if !strings.HasPrefix(rows[1], "line01") || !strings.Contains(rows[1], "3 lines") {
		t.Errorf("row 1 = %q, want folded header with summary", rows[1])
	}
	if !strings.HasPrefix(rows[2], "line05") {
		t.Errorf("row 2 = %q, want line05 (hidden lines skipped)", rows[2])
	}
}
