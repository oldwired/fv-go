package terminal

import (
	"bytes"
	"testing"
)

// A child that opens an OSC string then emits a long run of bare ESC
// bytes (no ST terminator) must not grow the OSC accumulator without
// bound. Regression: the ESC branch in feedOSC appended uncapped.
func TestOSCBoundedByESCFlood(t *testing.T) {
	b := newBuffer(20, 5)
	p := newParser(b)
	p.Feed([]byte("\x1b]0;"))
	p.Feed(bytes.Repeat([]byte{0x1B}, 100_000))
	if len(p.osc) > 4096 {
		t.Errorf("OSC buffer grew to %d bytes, want <= 4096 (ESC-flood DoS)", len(p.osc))
	}
}

// A single CSI sequence with an enormous parameter list must not grow
// csiParams without bound, and dispatching the final byte must not panic.
func TestCSIParamsBounded(t *testing.T) {
	b := newBuffer(20, 5)
	p := newParser(b)
	p.Feed([]byte("\x1b["))
	p.Feed(bytes.Repeat([]byte("1;"), 100_000))
	if len(p.csiParams) > maxCSIParams {
		t.Errorf("CSI params grew to %d, want <= %d", len(p.csiParams), maxCSIParams)
	}
	p.Feed([]byte("m")) // final byte; must not panic with capped params
}

// A long digit run within one CSI parameter must not integer-overflow
// into a negative value.
func TestCSIDigitOverflowGuard(t *testing.T) {
	b := newBuffer(20, 5)
	p := newParser(b)
	p.Feed([]byte("\x1b["))
	p.Feed(bytes.Repeat([]byte("9"), 100_000))
	if p.csiCurr < 0 {
		t.Errorf("csiCurr overflowed to %d (want a non-negative clamped value)", p.csiCurr)
	}
	p.Feed([]byte("m"))
}
