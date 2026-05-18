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
