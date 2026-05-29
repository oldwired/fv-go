package term

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/types"
)

// When only the continuation half of a wide glyph differs from the
// previous frame (e.g. SIXEL z-order invalidation or a partial overwrite
// dirties one cell while markClean leaves the other diff-equal), the diff
// must still re-emit the leading half. Otherwise it produces a span with
// empty text and the stale double-width glyph lingers on screen.
func TestDirtyPairsWideGlyphContinuation(t *testing.T) {
	b := newCellBuf(6, 1)
	attrA := types.MakeAttr(7, 0)
	attrB := types.MakeAttr(2, 0)

	// Wide glyph "宽" occupying columns 2 (lead) and 3 (continuation).
	b.Set(2, 0, types.DrawCell{Ch: "宽", Attr: attrA})
	b.Set(3, 0, types.DrawCell{Ch: "", Attr: attrA})
	b.commit() // prev == cur; nothing dirty.

	if got := b.dirty(); len(got) != 0 {
		t.Fatalf("after commit, dirty() = %d spans, want 0", len(got))
	}

	// Dirty ONLY the continuation cell (column 3); the leading cell at
	// column 2 stays diff-equal to prev.
	b.Set(3, 0, types.DrawCell{Ch: "", Attr: attrB})

	spans := b.dirty()
	// The leading half must be re-emitted: a span must start at column 2
	// carrying the wide glyph, not a lone empty-text span at column 3.
	found := false
	for _, s := range spans {
		if s.x == 2 && s.y == 0 && s.text == "宽" {
			found = true
		}
	}
	if !found {
		t.Errorf("leading half not re-emitted; spans=%+v (stale wide glyph would linger)", spans)
	}
}

// Conversely, dirtying the leading half forces the trailing continuation
// into the same repaint so the run covers both columns.
func TestDirtyPairsWideGlyphLeading(t *testing.T) {
	b := newCellBuf(6, 1)
	attrA := types.MakeAttr(7, 0)
	b.Set(2, 0, types.DrawCell{Ch: "宽", Attr: attrA})
	b.Set(3, 0, types.DrawCell{Ch: "", Attr: attrA})
	b.commit()

	// Replace the wide glyph with a different one; only the lead cell's
	// Ch changes, the continuation cell ("",A) stays diff-equal.
	b.Set(2, 0, types.DrawCell{Ch: "狭", Attr: attrA})

	spans := b.dirty()
	if len(spans) != 1 {
		t.Fatalf("want a single coalesced span over both halves, got %+v", spans)
	}
	s := spans[0]
	// The span starts at the lead column and its text is the new glyph;
	// the continuation contributes "" so the terminal's auto-advance
	// covers column 3.
	if s.x != 2 || s.text != "狭" {
		t.Errorf("span = %+v, want x=2 text=狭 spanning columns 2-3", s)
	}
}
