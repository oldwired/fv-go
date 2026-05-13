package terminal

import (
	"strings"
	"testing"
)

// makeRow turns a string into a row of cells, one per rune.
// `range s` iterates by code point so multibyte UTF-8 collapses to a
// single cell, matching the terminal buffer's storage.
func makeRow(s string) []cell {
	var out []cell
	for _, r := range s {
		out = append(out, cell{Ch: r, HasDefault: true})
	}
	return out
}

func TestSearchMatchesInRowBasic(t *testing.T) {
	row := makeRow("the quick brown fox jumps over the lazy dog")
	got := searchMatchesInRow(row, "the")
	want := []cellRange{{0, 3}, {31, 34}}
	if len(got) != len(want) {
		t.Fatalf("got %d matches, want %d (%v)", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("match %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestSearchMatchesInRowCaseInsensitive(t *testing.T) {
	row := makeRow("Hello HELLO hello")
	got := searchMatchesInRow(row, "hello")
	if len(got) != 3 {
		t.Errorf("expected 3 matches, got %d (%v)", len(got), got)
	}
}

func TestSearchMatchesInRowMultibyte(t *testing.T) {
	row := makeRow("café résumé café")
	got := searchMatchesInRow(row, "café")
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d (%v)", len(got), got)
	}
	// The first match is on cells 0..3 (c, a, f, é) — 4 cells.
	if got[0].start != 0 || got[0].end != 4 {
		t.Errorf("first match cells = %v, want {0, 4}", got[0])
	}
	// Second "café" starts after "café résumé " — 12 cells in.
	if got[1].start != 12 || got[1].end != 16 {
		t.Errorf("second match cells = %v, want {12, 16}", got[1])
	}
}

func TestSearchMatchesInRowEmpty(t *testing.T) {
	if got := searchMatchesInRow(makeRow("hi"), ""); got != nil {
		t.Errorf("empty needle should return nil, got %v", got)
	}
	if got := searchMatchesInRow(nil, "x"); got != nil {
		t.Errorf("nil row should return nil, got %v", got)
	}
}

func TestSearchMatchesInRowNoMatch(t *testing.T) {
	if got := searchMatchesInRow(makeRow("hello"), "world"); got != nil {
		t.Errorf("no match should return nil, got %v", got)
	}
}

// TestStartScrollbackSearch confirms the exported entry point puts
// the widget into search mode and (when the viewport is at the live
// bottom) nudges into scrollback so the search-mode UI is visible.
func TestStartScrollbackSearch(t *testing.T) {
	tm := &Terminal{}
	tm.buf = newBuffer(20, 5)
	// Seed enough scrollback that the nudge has somewhere to go.
	tm.buf.scrollback = append(tm.buf.scrollback, makeRow("history line 1"))
	tm.buf.scrollback = append(tm.buf.scrollback, makeRow("history line 2"))

	tm.StartScrollbackSearch()
	if !tm.searching {
		t.Error("StartScrollbackSearch did not set searching=true")
	}
	if len(tm.searchQuery) != 0 {
		t.Errorf("searchQuery should start empty, got %q", string(tm.searchQuery))
	}
	if tm.buf.scrollOffset == 0 {
		t.Error("StartScrollbackSearch did not nudge into scrollback")
	}
}

// TestStartScrollbackSearchAlreadyInScrollback confirms we don't move
// the viewport when the user is already viewing history — they
// invoked search from a specific position.
func TestStartScrollbackSearchAlreadyInScrollback(t *testing.T) {
	tm := &Terminal{}
	tm.buf = newBuffer(20, 5)
	for i := 0; i < 5; i++ {
		tm.buf.scrollback = append(tm.buf.scrollback, makeRow("line"))
	}
	tm.buf.scrollOffset = 3
	tm.StartScrollbackSearch()
	if tm.buf.scrollOffset != 3 {
		t.Errorf("scrollOffset moved from 3 to %d", tm.buf.scrollOffset)
	}
}

// TestCancelScrollbackSearch confirms the exit path clears all state.
func TestCancelScrollbackSearch(t *testing.T) {
	tm := &Terminal{}
	tm.buf = newBuffer(20, 5)
	tm.searching = true
	tm.searchQuery = []byte("foo")
	tm.CancelScrollbackSearch()
	if tm.searching {
		t.Error("CancelScrollbackSearch did not clear searching")
	}
	if len(tm.searchQuery) != 0 {
		t.Errorf("CancelScrollbackSearch did not clear query, got %q", string(tm.searchQuery))
	}
}

// TestSearchHighlightInDraw sanity-checks that searchMatchesInRow is
// stable for typical Western prose — the same flow Draw uses to paint
// the yellow "found" overlay.
func TestSearchHighlightInDraw(t *testing.T) {
	row := makeRow("hello brave new world")
	needle := strings.ToLower("brave")
	matches := searchMatchesInRow(row, needle)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].start != 6 || matches[0].end != 11 {
		t.Errorf("match cells = %v, want {6, 11}", matches[0])
	}
	for x := matches[0].start; x < matches[0].end; x++ {
		if !inAnyRange(matches, x) {
			t.Errorf("inAnyRange returned false for x=%d", x)
		}
	}
	if inAnyRange(matches, matches[0].end) {
		t.Errorf("inAnyRange should be false at end-exclusive boundary")
	}
}
