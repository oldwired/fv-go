package editor

import (
	"testing"
)

func TestReplaceRangeBasic(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello world")
	b.ReplaceRange(6, 11, "fv-go")
	if got := b.Text(); got != "hello fv-go" {
		t.Fatalf("ReplaceRange: %q", got)
	}
	if !b.Undo() {
		t.Fatal("Undo returned false")
	}
	if got := b.Text(); got != "hello world" {
		t.Fatalf("after undo: %q", got)
	}
	if !b.Redo() {
		t.Fatal("Redo returned false")
	}
	if got := b.Text(); got != "hello fv-go" {
		t.Fatalf("after redo: %q", got)
	}
}

func TestReplaceRangeIsOneUndoEntry(t *testing.T) {
	b := NewBuffer()
	b.SetText("abcdef")
	b.ReplaceRange(1, 3, "XY")
	if len(b.undoStack) != 1 {
		t.Fatalf("undo entries = %d, want 1", len(b.undoStack))
	}
}

func TestReplaceRangeSwapsReversedBounds(t *testing.T) {
	b := NewBuffer()
	b.SetText("abcdef")
	b.ReplaceRange(4, 2, "_")
	if got := b.Text(); got != "ab_ef" {
		t.Fatalf("reversed bounds: %q", got)
	}
}

func TestReplaceRangeSnapsMidRune(t *testing.T) {
	b := NewBuffer()
	b.SetText("aébc") // é = 2 bytes at offset 1
	// Offsets 2 and 4 are mid-rune-safe; offset 2 is inside é.
	b.ReplaceRange(2, 3, "X")
	// Start snaps back to 1 (start of é); end 3 is the rune start of 'b'.
	if got := b.Text(); got != "aXbc" {
		t.Fatalf("mid-rune snap: %q", got)
	}
}

func TestReplaceRangeAtEOFAndPureDelete(t *testing.T) {
	b := NewBuffer()
	b.SetText("abc")
	b.ReplaceRange(3, 99, "!")
	if got := b.Text(); got != "abc!" {
		t.Fatalf("at EOF: %q", got)
	}
	b.ReplaceRange(1, 3, "")
	if got := b.Text(); got != "a!" {
		t.Fatalf("pure delete: %q", got)
	}
}

func TestReplaceRangeClampsNegative(t *testing.T) {
	b := NewBuffer()
	b.SetText("abc")
	b.ReplaceRange(-5, 1, "Z")
	if got := b.Text(); got != "Zbc" {
		t.Fatalf("negative start: %q", got)
	}
}

func TestOffsetAtPositionForRoundTrip(t *testing.T) {
	b := NewBuffer()
	b.SetText("one\ntwo\nthree")
	cases := []struct {
		line, col, want int
	}{
		{0, 0, 0},
		{0, 3, 3},
		{1, 0, 4},
		{1, 2, 6},
		{2, 5, 13},
	}
	for _, c := range cases {
		got := b.OffsetAt(c.line, c.col)
		if got != c.want {
			t.Errorf("OffsetAt(%d,%d) = %d, want %d", c.line, c.col, got, c.want)
		}
		l, cc := b.PositionFor(got)
		if l != c.line || cc != c.col {
			t.Errorf("PositionFor(%d) = (%d,%d), want (%d,%d)", got, l, cc, c.line, c.col)
		}
	}
}

func TestOffsetAtClamps(t *testing.T) {
	b := NewBuffer()
	b.SetText("ab\ncd")
	if got := b.OffsetAt(0, 99); got != 2 {
		t.Errorf("col past line end = %d, want 2 (before the newline)", got)
	}
	if got := b.OffsetAt(99, 0); got != 5 {
		t.Errorf("line past EOF = %d, want Len", got)
	}
	if got := b.OffsetAt(-1, -1); got != 0 {
		t.Errorf("negative = %d, want 0", got)
	}
	empty := NewBuffer()
	if got := empty.OffsetAt(0, 0); got != 0 {
		t.Errorf("empty buffer = %d, want 0", got)
	}
}

func TestOffsetAtCRLFLineIncludesCR(t *testing.T) {
	b := NewBuffer()
	b.SetText("ab\r\ncd")
	// The '\r' is ordinary line content: line 0 is "ab\r" (3 bytes).
	if got := b.OffsetAt(0, 99); got != 3 {
		t.Errorf("CRLF line end = %d, want 3 (after the \\r)", got)
	}
	if got := b.OffsetAt(1, 0); got != 4 {
		t.Errorf("line 1 start = %d, want 4", got)
	}
}

func TestPositionForSnapsMidRune(t *testing.T) {
	b := NewBuffer()
	b.SetText("é") // 2 bytes
	l, c := b.PositionFor(1)
	if l != 0 || c != 0 {
		t.Errorf("mid-rune PositionFor = (%d,%d), want (0,0)", l, c)
	}
	l, c = b.PositionFor(99)
	if l != 0 || c != 2 {
		t.Errorf("past-EOF PositionFor = (%d,%d), want (0,2)", l, c)
	}
}

func TestGroupUndoIsAtomic(t *testing.T) {
	b := NewBuffer()
	b.SetText("aaa bbb")
	b.BeginGroup()
	b.ReplaceRange(0, 3, "XXX")
	b.ReplaceRange(4, 7, "YYY")
	b.EndGroup()
	if got := b.Text(); got != "XXX YYY" {
		t.Fatalf("after grouped edits: %q", got)
	}
	b.Undo()
	if got := b.Text(); got != "aaa bbb" {
		t.Fatalf("one undo must reverse the whole group: %q", got)
	}
	b.Redo()
	if got := b.Text(); got != "XXX YYY" {
		t.Fatalf("one redo must replay the whole group: %q", got)
	}
}

func TestGroupFiresOneChange(t *testing.T) {
	b := NewBuffer()
	b.SetText("aaa bbb")
	e := NewShared(testRect(), nil, nil, b)
	var fires int
	e.OnChange = func(int) { fires++ }
	b.BeginGroup()
	b.ReplaceRange(0, 3, "X")
	b.ReplaceRange(2, 5, "Y")
	b.EndGroup()
	if fires != 1 {
		t.Errorf("OnChange fired %d times for one group, want 1", fires)
	}
	fires = 0
	b.Undo()
	if fires != 1 {
		t.Errorf("OnChange fired %d times for group undo, want 1", fires)
	}
}

func TestGroupNesting(t *testing.T) {
	b := NewBuffer()
	b.SetText("abc")
	b.BeginGroup()
	b.ReplaceRange(0, 1, "X")
	b.BeginGroup()
	b.ReplaceRange(1, 2, "Y")
	b.EndGroup()
	b.ReplaceRange(2, 3, "Z")
	b.EndGroup()
	b.Undo()
	if got := b.Text(); got != "abc" {
		t.Fatalf("nested group should undo as one: %q", got)
	}
}

func TestEmptyGroupFiresNoChange(t *testing.T) {
	b := NewBuffer()
	e := NewShared(testRect(), nil, nil, b)
	var fires int
	e.OnChange = func(int) { fires++ }
	b.BeginGroup()
	b.EndGroup()
	if fires != 0 {
		t.Errorf("empty group fired OnChange %d times, want 0", fires)
	}
}

func TestGroupedEntriesDontCoalesceWithUngrouped(t *testing.T) {
	b := NewBuffer()
	who := &struct{}{} // typing comes from one origin; nil never merges
	// Plain typing coalesces...
	b.splice(who, 0, 0, []byte("a"), -1, -1)
	b.splice(who, 1, 1, []byte("b"), -1, -1)
	if len(b.undoStack) != 1 {
		t.Fatalf("plain typing should coalesce: %d entries", len(b.undoStack))
	}
	// ...but a grouped adjacent insert must not merge into it.
	b.BeginGroup()
	b.splice(who, 2, 2, []byte("c"), -1, -1)
	b.EndGroup()
	if len(b.undoStack) != 2 {
		t.Fatalf("grouped entry coalesced with ungrouped: %d entries", len(b.undoStack))
	}
}
