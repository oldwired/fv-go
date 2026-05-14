package clipboard

import (
	"errors"
	"strings"
	"testing"
)

func resetState() {
	mu.Lock()
	buf = ""
	writer = nil
	policy = defaultPolicy
	mu.Unlock()
}

// TestSetTextPreservesNoErrorAPI confirms the existing widget contract:
// SetText returns nothing. Forcing every copy-path caller to handle a
// clipboard error would be noisy for almost no benefit.
func TestSetTextPreservesNoErrorAPI(t *testing.T) {
	resetState()
	SetText("hello") // must compile as a value-less expression
	if GetText() != "hello" {
		t.Errorf("internal buffer: got %q, want %q", GetText(), "hello")
	}
}

// TestTrySetTextOversizedReturnsErr verifies the cap kicks in and the
// internal buffer is still updated. OSC 52 emission is the only thing
// suppressed.
func TestTrySetTextOversizedReturnsErr(t *testing.T) {
	resetState()
	var emitted string
	SetWriter(func(s string) error {
		emitted = s
		return nil
	})
	SetPolicy(Policy{MaxBytes: 16}) // tiny cap so we can test easily

	payload := strings.Repeat("x", 100)
	err := TrySetText(payload)
	if !errors.Is(err, ErrClipboardTooLarge) {
		t.Errorf("expected ErrClipboardTooLarge, got %v", err)
	}
	if emitted != "" {
		t.Errorf("OSC 52 was emitted for oversized payload: %q", emitted)
	}
	if GetText() != payload {
		t.Error("internal buffer should still hold the full payload on oversize")
	}
}

// TestPolicyDisableOSC52 confirms the zero-value-sane DisableOSC52 bool:
// setting it to true suppresses emission, but the buffer still updates
// and TrySetText returns nil (not an error).
func TestPolicyDisableOSC52(t *testing.T) {
	resetState()
	var emitted bool
	SetWriter(func(s string) error { emitted = true; return nil })
	SetPolicy(Policy{DisableOSC52: true})

	if err := TrySetText("hi"); err != nil {
		t.Errorf("TrySetText returned err with OSC 52 disabled: %v", err)
	}
	if emitted {
		t.Error("OSC 52 emitted despite DisableOSC52=true")
	}
	if GetText() != "hi" {
		t.Error("internal buffer not updated when OSC 52 disabled")
	}
}

// TestPolicyZeroValueNormalizesMaxBytes verifies that SetPolicy(Policy{})
// gets a sane 100 KB cap rather than 0 (which would reject every paste).
func TestPolicyZeroValueNormalizesMaxBytes(t *testing.T) {
	resetState()
	SetWriter(func(s string) error { return nil })
	SetPolicy(Policy{}) // both fields zero — should normalize MaxBytes

	if err := TrySetText("ok"); err != nil {
		t.Errorf("zero-value policy rejected a small payload: %v", err)
	}
}
