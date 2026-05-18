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

// TestDrawStrNegativePosClipsLeft: an oversized centered label whose
// computed start position is negative used to vanish entirely
// (DrawStr broke on x<0). It now advances past the off-screen runes
// and renders the visible suffix.
func TestDrawStrNegativePosClipsLeft(t *testing.T) {
	buf := MakeDrawBuffer(5)
	DrawStr(buf, -3, "abcdefgh", 0)
	got := ""
	for _, c := range buf {
		got += c.Ch
	}
	if got != "defgh" {
		t.Errorf("DrawStr(-3, \"abcdefgh\"): got %q, want %q", got, "defgh")
	}
}

// TestDrawStrZWJClusterCollapsesToOneCluster regression: a ZWJ emoji
// cluster (family, rainbow flag, etc.) used to advance our cellbuf
// once per component while ZWJ-aware terminals render the whole
// cluster as one wide glyph. The leftover trailing cells of our
// cellbuf never got painted in the terminal, so desktop background
// or other stale content showed through at the right side of any row
// containing such a cluster.
//
// Correctness invariant: the two cellbuf cells (leading + continuation)
// taken together hold the full cluster text, and the next char after
// the cluster lands at cellbuf column 2 — not column 8 like before
// the fix.
func TestDrawStrZWJClusterCollapsesToOneCluster(t *testing.T) {
	buf := MakeDrawBuffer(20)
	// 4-person family ZWJ sequence — 👨 ZWJ 👩 ZWJ 👧 ZWJ 👦.
	// ZWJ (U+200D) is escaped so the source bytes stay readable —
	// raw ZWJ is invisible and trips staticcheck's ST1018 ("string
	// literal contains Unicode format characters").
	family := "\U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466"
	DrawStr(buf, 0, family+"X", 0)

	// The full cluster text is split between the leading cell and
	// its continuation. When the cellbuf span aggregator concatenates
	// them, the terminal sees the complete ZWJ sequence and renders
	// one glyph.
	combined := buf[0].Ch + buf[1].Ch
	if combined != family {
		t.Errorf("buf[0]+buf[1] = %q, want full cluster %q", combined, family)
	}
	// The post-cluster 'X' must land at column 2. Before the fix
	// this would have been at column 8 (4 emojis × 2 cells).
	if buf[2].Ch != "X" {
		t.Errorf("buf[2] = %q, want %q (post-cluster char must not drift)", buf[2].Ch, "X")
	}
}

// TestDrawCStrNegativePosClipsLeft: same clip-left behavior with the
// hotkey-marker variant. The '~' toggles still apply to the
// off-screen prefix so the on-screen colors match what the user
// would see if the field were wider.
func TestDrawCStrNegativePosClipsLeft(t *testing.T) {
	buf := MakeDrawBuffer(5)
	// "ab~c~defgh" — c is the only hot rune; with start pos -3 the
	// visible window is "defgh", all in normal color, but the
	// function should have processed the '~' markers correctly so
	// nothing is corrupted.
	DrawCStr(buf, -3, "ab~c~defgh", 0x07, 0x0F)
	got := ""
	for _, c := range buf {
		got += c.Ch
	}
	if got != "defgh" {
		t.Errorf("DrawCStr(-3, \"ab~c~defgh\"): got %q, want %q", got, "defgh")
	}
}
