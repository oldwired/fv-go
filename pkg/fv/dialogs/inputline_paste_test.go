package dialogs

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/validators"
)

// TestInputLinePasteRespectsValidator regression for the bug where
// the CmPaste path bypassed the validator — a field that rejected
// typed characters would happily accept the same characters pasted
// in. The fix routes each pasted rune through passesInputValidator,
// dropping ones it rejects while keeping the rest.
func TestInputLinePasteRespectsValidator(t *testing.T) {
	il := NewInputLine(geom.NewRect(0, 0, 20, 1), 32)
	il.Validator = validators.NewFilterValidator("ab")

	ev := &drivers.Event{
		What:    consts.EvCommand,
		Command: consts.CmPaste,
		InfoPtr: "aXbYab",
	}
	il.HandleEvent(ev)

	got := string(il.Data)
	want := "abab"
	if got != want {
		t.Errorf("paste with filter validator: got %q, want %q", got, want)
	}
}

// TestInputLineCellColumnForRegionalFlag verifies the rune-index→
// cell-column translation used by Draw to place the cursor. The
// flag 🇩🇪 is a single grapheme cluster of 2 runes that takes 2
// cells. Setting CurPos to 2 (just past both runes of the flag) must
// place the visible caret 2 cells to the right of the field's left
// edge (column 1 = first content column + 2 = column 3 in the
// rendered output).
func TestInputLineCellColumnForRegionalFlag(t *testing.T) {
	il := NewInputLine(geom.NewRect(0, 0, 20, 1), 32)
	// 🇩🇪 — two regional-indicator runes forming one wide cluster.
	il.Data = []rune("\U0001F1E9\U0001F1EA")
	il.CurPos = 2 // past both runes of the flag

	got := il.cellColumnForRuneIndex(il.CurPos)
	if got != 2 {
		t.Errorf("cellColumnForRuneIndex(past flag) = %d, want 2 (cluster is one 2-cell glyph)", got)
	}

	// Cursor in the middle of the cluster (rune index 1) should
	// snap to the cluster's leading edge — cell column 0.
	if got := il.cellColumnForRuneIndex(1); got != 0 {
		t.Errorf("cellColumnForRuneIndex(mid-cluster) = %d, want 0 (snap to leading edge)", got)
	}
}

// TestInputLinePasteAllRejected verifies that a paste consisting
// entirely of invalid characters leaves the field empty (rather
// than crashing or partially committing).
func TestInputLinePasteAllRejected(t *testing.T) {
	il := NewInputLine(geom.NewRect(0, 0, 20, 1), 32)
	il.Validator = validators.NewFilterValidator("ab")

	il.HandleEvent(&drivers.Event{
		What:    consts.EvCommand,
		Command: consts.CmPaste,
		InfoPtr: "XYZ",
	})
	if got := string(il.Data); got != "" {
		t.Errorf("paste of all-invalid input: got %q, want empty", got)
	}
}
