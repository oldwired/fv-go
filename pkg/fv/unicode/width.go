// Package unicode provides cell-width measurement for Unicode code points.
//
// Width semantics:
//   - 0 cells: combining marks, ZWJ/ZWNJ, BOM, format chars, variation selectors
//   - 2 cells: East-Asian Wide / Fullwidth and most emoji
//   - 1 cell:  everything else, including ASCII printable
//
// Tables ported verbatim from FVUnicodeWidth.pas (Unicode 15.1; ultimately
// derived from spectreconsole/wcwidth).
package unicode

type rng struct {
	Lo, Hi uint32
}

// inRange returns true if cp is contained in any sorted range. Binary search;
// O(log n).
func inRange(cp uint32, table []rng) bool {
	lo, hi := 0, len(table)-1
	for lo <= hi {
		mid := (lo + hi) >> 1
		r := table[mid]
		switch {
		case cp < r.Lo:
			hi = mid - 1
		case cp > r.Hi:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}

// IsWide reports whether cp occupies 2 terminal cells.
func IsWide(cp uint32) bool { return inRange(cp, wideRanges[:]) }

// IsZero reports whether cp occupies 0 terminal cells.
func IsZero(cp uint32) bool { return inRange(cp, zeroRanges[:]) }

// CellWidth returns 0, 1, or 2 for the cell width of cp. ASCII printable
// (U+0020..U+007E) takes a fast path.
func CellWidth(cp uint32) int {
	if cp >= 0x20 && cp < 0x7F {
		return 1
	}
	if IsZero(cp) {
		return 0
	}
	if IsWide(cp) {
		return 2
	}
	return 1
}
