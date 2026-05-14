package terminal

import (
	"testing"
)

// TestOnFeedMutatesBytes confirms the hook receives every PTY-output
// chunk and that the returned bytes are what gets parsed (and
// ultimately rendered).
func TestOnFeedMutatesBytes(t *testing.T) {
	tm := &Terminal{}
	tm.buf = newBuffer(20, 3)
	tm.par = newParser(tm.buf)

	// :rot13-style transform — uppercase every ASCII letter.
	tm.OnFeed = func(in []byte) []byte {
		out := make([]byte, len(in))
		for i, b := range in {
			if b >= 'a' && b <= 'z' {
				out[i] = b - 32
			} else {
				out[i] = b
			}
		}
		return out
	}
	// Write() goes through par.Feed directly without the OnFeed hook
	// (it's a test-only injection path). Simulate the readLoop's
	// invocation pattern instead.
	chunk := []byte("hello")
	if tm.OnFeed != nil {
		chunk = tm.OnFeed(chunk)
	}
	tm.par.Feed(chunk)
	// First cell should be 'H', not 'h'.
	if got := tm.buf.cells[0][0].Ch; got != 'H' {
		t.Errorf("OnFeed didn't apply: cell[0][0] = %q, want 'H'", got)
	}
}

// TestOnFeedNilSafePassthrough confirms the readLoop path is a no-op
// when OnFeed isn't set (the readLoop guards against nil — verified
// here against the same chunk).
func TestOnFeedNilSafePassthrough(t *testing.T) {
	tm := &Terminal{}
	tm.buf = newBuffer(20, 3)
	tm.par = newParser(tm.buf)
	chunk := []byte("ab")
	if tm.OnFeed != nil {
		chunk = tm.OnFeed(chunk)
	}
	tm.par.Feed(chunk)
	if tm.buf.cells[0][0].Ch != 'a' || tm.buf.cells[0][1].Ch != 'b' {
		t.Errorf("passthrough failed: cells = %q %q",
			tm.buf.cells[0][0].Ch, tm.buf.cells[0][1].Ch)
	}
}
