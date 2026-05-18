package utf8

import "testing"

// These tests cover cluster-level cases the old hand-coded ZWJ/VS16
// logic couldn't handle: regional-indicator country flags, skin-tone
// modifiers, full ZWJ family clusters with skin-tone joiners,
// Hangul jamo composition. The uniseg-backed StringDisplayWidth and
// CopyDisplayCells should produce stable per-cluster results.

func TestRegionalIndicatorFlag(t *testing.T) {
	// 🇩🇪 = regional indicator D + regional indicator E.
	flag := "\U0001F1E9\U0001F1EA"
	if got := StringDisplayWidth(flag); got != 2 {
		t.Errorf("StringDisplayWidth(%q) = %d, want 2 (one cluster, wide)", flag, got)
	}
}

func TestThreeFlags(t *testing.T) {
	// 🇩🇪🇫🇷🇪🇸 — three flags, each 2 cells.
	flags := "\U0001F1E9\U0001F1EA\U0001F1EB\U0001F1F7\U0001F1EA\U0001F1F8"
	if got := StringDisplayWidth(flags); got != 6 {
		t.Errorf("StringDisplayWidth(three flags) = %d, want 6", got)
	}
}

func TestSkinToneModifier(t *testing.T) {
	// 👋🏼 = wave + medium-light skin tone.
	hand := "\U0001F44B\U0001F3FC"
	if got := StringDisplayWidth(hand); got != 2 {
		t.Errorf("StringDisplayWidth(%q) = %d, want 2", hand, got)
	}
}

func TestFamilyClusterAtomic(t *testing.T) {
	// 👨‍👩‍👧‍👦 — 4-person family, joined by ZWJs. One cluster, 2 cells.
	family := "\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466"
	if got := StringDisplayWidth(family); got != 2 {
		t.Errorf("StringDisplayWidth(family) = %d, want 2", got)
	}
}

func TestRainbowFlagAtomic(t *testing.T) {
	// 🏳️‍🌈 — white flag + VS16 + ZWJ + rainbow. One cluster, 2 cells.
	rainbow := "\U0001F3F3️‍\U0001F308"
	if got := StringDisplayWidth(rainbow); got != 2 {
		t.Errorf("StringDisplayWidth(rainbow) = %d, want 2", got)
	}
}

func TestCJKEachWide(t *testing.T) {
	// 日本語 — three CJK ideographs, two cells each.
	if got := StringDisplayWidth("日本語"); got != 6 {
		t.Errorf("StringDisplayWidth(CJK) = %d, want 6", got)
	}
}

func TestHangulSyllable(t *testing.T) {
	// 한 (precomposed Hangul syllable U+D55C). 2 cells.
	if got := StringDisplayWidth("한"); got != 2 {
		t.Errorf("StringDisplayWidth(Hangul precomposed) = %d, want 2", got)
	}
}

func TestCombiningDoesNotAdvance(t *testing.T) {
	// e + combining acute = one cluster, 1 cell.
	combined := "é"
	if got := StringDisplayWidth(combined); got != 1 {
		t.Errorf("StringDisplayWidth(e + combining) = %d, want 1", got)
	}
}

func TestCopyDisplayCellsKeepsClusterAtomic(t *testing.T) {
	// "abc🇩🇪" — taking first 4 cells should include the flag entirely;
	// taking first 3 should stop before the flag (it would straddle
	// the 5th cell boundary).
	s := "abc\U0001F1E9\U0001F1EA"
	if got := CopyDisplayCells(s, 0, 5); got != s {
		t.Errorf("CopyDisplayCells(%q, 0, 5) = %q, want full string", s, got)
	}
	// Asking for 4 cells: abc fits (3 cells), flag (2 cells) would
	// straddle to col 5; cluster is dropped.
	if got := CopyDisplayCells(s, 0, 4); got != "abc" {
		t.Errorf("CopyDisplayCells(%q, 0, 4) = %q, want %q", s, got, "abc")
	}
}
