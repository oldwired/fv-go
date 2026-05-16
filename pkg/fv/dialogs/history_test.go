package dialogs

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/history"
)

// TestHistoryGlyphSetsSelf — even the cosmetic dropdown glyph must
// wire SetSelf so HandleEvent dispatch lands on its overrides
// instead of falling through to Base's no-op.
func TestHistoryGlyphSetsSelf(t *testing.T) {
	il := NewInputLine(geom.NewRect(0, 0, 20, 1), 32)
	h := NewHistory(geom.NewRect(20, 0, 21, 1), il, 99)
	if h.BaseView().Self() != h {
		t.Error("NewHistory forgot SetSelf — HandleEvent overrides won't fire")
	}
}

// TestHistoryItemsReadsStore confirms the history widget consults the
// package-level history store when HistoryID is non-zero.
func TestHistoryItemsReadsStore(t *testing.T) {
	const id = 200
	history.Clear(id)
	defer history.Clear(id)
	history.Add(id, "first")
	history.Add(id, "second")

	il := NewInputLine(geom.NewRect(0, 0, 20, 1), 32)
	h := NewHistory(geom.NewRect(20, 0, 21, 1), il, id)
	got := h.items()
	// history.Get returns most-recent-first.
	if len(got) != 2 || got[0] != "second" || got[1] != "first" {
		t.Errorf("h.items() = %v, want [second first]", got)
	}
}

// TestHistoryItemsAdHocSlice — when HistoryID is 0, the explicit
// Items slice is the candidate list.
func TestHistoryItemsAdHocSlice(t *testing.T) {
	il := NewInputLine(geom.NewRect(0, 0, 20, 1), 32)
	h := NewHistory(geom.NewRect(20, 0, 21, 1), il, 0)
	h.Items = []string{"alpha", "beta"}
	got := h.items()
	if len(got) != 2 || got[0] != "alpha" {
		t.Errorf("h.items() = %v, want [alpha beta]", got)
	}
}

// TestHistoryOpenPopupNoopOnEmpty — an empty history shouldn't pop
// up an empty list. Confirm openPopup exits cleanly with no view-tree
// changes when items is empty.
func TestHistoryOpenPopupNoopOnEmpty(t *testing.T) {
	const id = 201
	history.Clear(id)
	defer history.Clear(id)
	il := NewInputLine(geom.NewRect(0, 0, 20, 1), 32)
	h := NewHistory(geom.NewRect(20, 0, 21, 1), il, id)
	// No items, no owner — openPopup should be a no-op (no panic).
	h.openPopup()
}
