package screen

import "testing"

// TestMakeDrawBufferNegativeNoPanic is the regression for the bug
// fvmux hit: a Window resized below its fixed-bound child's left
// edge produced a negative Size.X, which then went straight into
// MakeDrawBuffer and panicked with "len out of range" inside the
// runtime's make call. The defensive clamp prevents that.
func TestMakeDrawBufferNegativeNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MakeDrawBuffer panicked on negative input: %v", r)
		}
	}()
	b := MakeDrawBuffer(-5)
	if len(b) != 0 {
		t.Errorf("negative input should yield empty buffer; got len=%d", len(b))
	}
}

// TestMakeDrawBufferZeroIsEmpty — zero-length buffer is the natural
// no-op for a Draw method whose view has been collapsed.
func TestMakeDrawBufferZeroIsEmpty(t *testing.T) {
	b := MakeDrawBuffer(0)
	if len(b) != 0 {
		t.Errorf("zero input should yield empty buffer; got len=%d", len(b))
	}
}

// TestMakeDrawBufferCapClampsToMaxViewWidth — the existing upper-end
// clamp keeps working alongside the new lower-end clamp.
func TestMakeDrawBufferCapClampsToMaxViewWidth(t *testing.T) {
	b := MakeDrawBuffer(MaxViewWidth + 1000)
	if len(b) != MaxViewWidth {
		t.Errorf("oversized request should clamp to MaxViewWidth; got len=%d", len(b))
	}
}
