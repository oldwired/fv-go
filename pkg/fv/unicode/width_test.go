package unicode

import "testing"

func TestCellWidth(t *testing.T) {
	cases := []struct {
		cp   uint32
		want int
	}{
		{'A', 1},
		{' ', 1},
		{'~', 1},
		{0x00, 0},     // NUL counted as zero (in zeroRanges via [0,0])
		{0x300, 0},    // combining grave accent
		{0x200D, 0},   // ZWJ
		{0xFE0F, 0},   // VS16
		{0x1100, 2},   // Hangul Jamo (wide)
		{0x4E2D, 2},   // CJK ideograph
		{0x1F600, 2},  // grinning face emoji
		{0x1F1E6, 2},  // regional indicator A
		{0x00A0, 1},   // non-breaking space (neither wide nor zero)
		{0x00E9, 1},   // é precomposed
		{0x10FFFF, 1}, // unknown high cp falls through to 1
	}
	for _, c := range cases {
		if got := CellWidth(c.cp); got != c.want {
			t.Errorf("CellWidth(%#x) = %d, want %d", c.cp, got, c.want)
		}
	}
}

func TestIsWideAndZero(t *testing.T) {
	if !IsWide(0x4E2D) {
		t.Error("CJK should be wide")
	}
	if IsWide('A') {
		t.Error("ASCII A should not be wide")
	}
	if !IsZero(0x200D) {
		t.Error("ZWJ should be zero")
	}
	if IsZero('A') {
		t.Error("ASCII A should not be zero")
	}
}

func TestRangesSortedAndDisjoint(t *testing.T) {
	check := func(name string, t_ *testing.T, table []rng) {
		t_.Helper()
		for i := 1; i < len(table); i++ {
			if table[i-1].Hi >= table[i].Lo {
				t_.Errorf("%s table not sorted/disjoint at %d: prev=%v cur=%v",
					name, i, table[i-1], table[i])
			}
			if table[i].Lo > table[i].Hi {
				t_.Errorf("%s table reversed range at %d: %v", name, i, table[i])
			}
		}
	}
	check("wide", t, wideRanges[:])
	check("zero", t, zeroRanges[:])
}
