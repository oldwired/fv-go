// Package utf8 mirrors FVUTF8.pas: encoding sniffing, conversion to UTF-8,
// and grapheme-cluster-aware display-width / slicing helpers.
//
// The Pascal version juggles UTF-16 surrogates because Delphi's string
// type is UTF-16; Go strings are already UTF-8, so the codec layer is
// thin (mostly the standard `unicode/utf8` package). What's worth porting
// is the grapheme-aware behavior: ZWJ (U+200D) merges adjacent clusters,
// VS16 (U+FE0F) promotes a width-1 base to width 2 (emoji presentation),
// other zero-width code points attach to the previous cluster.
package utf8

import (
	stdutf8 "unicode/utf8"

	"github.com/oldwired/fv-go/pkg/fv/unicode"
)

// FileEncoding mirrors TFileEncoding.
type FileEncoding int

const (
	EncUnknown FileEncoding = iota
	EncUTF8
	EncUTF8BOM
	EncUTF16LE
	EncUTF16BE
	EncANSI // CP1252 fallback
)

// DetectEncoding examines up to len(data) bytes and returns the most
// likely encoding. Mirrors DetectEncoding in FVUTF8.pas.
func DetectEncoding(data []byte) FileEncoding {
	n := len(data)
	if n == 0 {
		return EncUnknown
	}
	if n >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return EncUTF8BOM
	}
	if n >= 2 {
		if data[0] == 0xFF && data[1] == 0xFE {
			return EncUTF16LE
		}
		if data[0] == 0xFE && data[1] == 0xFF {
			return EncUTF16BE
		}
	}
	// Validate UTF-8 (without BOM).
	hasHigh := false
	if stdutf8.Valid(data) {
		for _, b := range data {
			if b >= 0x80 {
				hasHigh = true
				break
			}
		}
		if hasHigh {
			return EncUTF8
		}
		return EncUTF8 // pure ASCII is also valid UTF-8
	}
	return EncANSI
}

// BOMLength returns the number of leading bytes the encoding's BOM
// occupies, or 0 if none.
func BOMLength(enc FileEncoding) int {
	switch enc {
	case EncUTF8BOM:
		return 3
	case EncUTF16LE, EncUTF16BE:
		return 2
	}
	return 0
}

// CharLen returns the expected total length in bytes of a UTF-8 sequence
// whose lead byte is b. Returns 1 for ASCII and 0 for invalid lead.
func CharLen(b byte) int {
	switch {
	case b < 0x80:
		return 1
	case b&0xE0 == 0xC0:
		return 2
	case b&0xF0 == 0xE0:
		return 3
	case b&0xF8 == 0xF0:
		return 4
	}
	return 0
}

// IsTrailByte reports whether b is a UTF-8 continuation byte (10xxxxxx).
func IsTrailByte(b byte) bool { return b&0xC0 == 0x80 }

// DecodeRune decodes one rune from buf and returns it together with the
// byte count it consumed. Mirrors DecodeUTF8CodePoint.
func DecodeRune(buf []byte) (r rune, n int) {
	r, n = stdutf8.DecodeRune(buf)
	return
}

// IsWideRune reports whether r occupies 2 cells.
func IsWideRune(r rune) bool { return unicode.IsWide(uint32(r)) }

// RuneCellWidth returns 0/1/2 cells for r.
func RuneCellWidth(r rune) int { return unicode.CellWidth(uint32(r)) }

// StringDisplayWidth returns the number of terminal cells s occupies,
// honoring ZWJ joining and VS16 width promotion the same way FV's
// drawing code lays the string out.
func StringDisplayWidth(s string) int {
	return measure(s, false)
}

// CStrDisplayWidth is like StringDisplayWidth but skips '~' hotkey
// markers that don't render as glyphs.
func CStrDisplayWidth(s string) int {
	return measure(s, true)
}

func measure(s string, skipTilde bool) int {
	total := 0
	clusterWidth := 0
	haveCluster := false
	joinNext := false

	flush := func() {
		if haveCluster {
			total += clusterWidth
			haveCluster = false
			clusterWidth = 0
		}
	}

	i := 0
	for i < len(s) {
		if skipTilde && s[i] == '~' {
			i++
			continue
		}
		r, sz := stdutf8.DecodeRuneInString(s[i:])
		i += sz
		w := unicode.CellWidth(uint32(r))
		if w == 0 {
			if haveCluster {
				switch r {
				case 0xFE0F: // VS16
					if clusterWidth == 1 {
						clusterWidth = 2
					}
				case 0x200D: // ZWJ
					joinNext = true
				}
			}
			continue
		}
		if haveCluster && joinNext {
			if w > clusterWidth {
				clusterWidth = w
			}
			joinNext = false
			continue
		}
		flush()
		haveCluster = true
		clusterWidth = w
		joinNext = false
	}
	flush()
	return total
}

// CopyDisplayCells returns the substring of s that occupies cells
// [startCol, startCol+maxWidth) in the rendered terminal. Whole grapheme
// clusters are kept together; if a wide cluster would straddle the end
// it is omitted entirely.
func CopyDisplayCells(s string, startCol, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if startCol < 0 {
		startCol = 0
	}
	endCol := startCol + maxWidth

	var out []byte
	col := 0
	clusterWidth := 0
	var clusterText []byte
	haveCluster := false
	joinNext := false

	flush := func() bool {
		if !haveCluster {
			return true
		}
		if col >= startCol && col+clusterWidth <= endCol {
			out = append(out, clusterText...)
		}
		col += clusterWidth
		haveCluster = false
		clusterText = clusterText[:0]
		clusterWidth = 0
		return col < endCol
	}

	i := 0
	for i < len(s) {
		r, sz := stdutf8.DecodeRuneInString(s[i:])
		piece := s[i : i+sz]
		i += sz

		w := unicode.CellWidth(uint32(r))
		if w == 0 {
			if haveCluster {
				clusterText = append(clusterText, piece...)
				if r == 0xFE0F && clusterWidth == 1 {
					clusterWidth = 2
				}
				if r == 0x200D {
					joinNext = true
				}
			}
			continue
		}
		if haveCluster && joinNext {
			clusterText = append(clusterText, piece...)
			if w > clusterWidth {
				clusterWidth = w
			}
			joinNext = false
			continue
		}
		if !flush() {
			break
		}
		haveCluster = true
		clusterText = append(clusterText[:0], piece...)
		clusterWidth = w
		joinNext = false
	}
	flush()
	return string(out)
}
