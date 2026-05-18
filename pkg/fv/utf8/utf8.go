// Package utf8 mirrors FVUTF8.pas: encoding sniffing, conversion to UTF-8,
// and grapheme-cluster-aware display-width / slicing helpers.
//
// The Pascal version juggles UTF-16 surrogates because Delphi's string
// type is UTF-16; Go strings are already UTF-8, so the codec layer is
// thin (mostly the standard `unicode/utf8` package). Cluster-aware
// width and slicing delegate to github.com/rivo/uniseg, which
// implements UAX #29 (text segmentation) and the wcwidth-style
// monospace cell-width tables — handling ZWJ clusters (family
// 👨‍👩‍👧‍👦, rainbow flag 🏳️‍🌈), VS16 emoji-presentation promotion,
// regional indicators (🇩🇪), skin-tone modifiers (👋🏼), and combining
// marks without per-codepoint special cases.
package utf8

import (
	stdutf8 "unicode/utf8"

	"github.com/rivo/uniseg"
)

// FileEncoding identifies the encoding DetectEncoding inferred for a
// byte slice. Mirrors TFileEncoding in FVUTF8.pas. Saved on Editor /
// FileEditor so SaveFile can round-trip the original BOM.
type FileEncoding int

const (
	EncUnknown FileEncoding = iota // sniff failed; treat as binary or ANSI
	EncUTF8                        // plain UTF-8, no BOM
	EncUTF8BOM                     // UTF-8 with leading EF BB BF
	EncUTF16LE                     // UTF-16 little-endian (FF FE BOM optional)
	EncUTF16BE                     // UTF-16 big-endian (FE FF BOM optional)
	EncANSI                        // CP1252 fallback for legacy 8-bit files
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
	case EncUnknown, EncUTF8, EncANSI:
		// No BOM for any of these.
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

// StringDisplayWidth returns the number of terminal cells s occupies.
// Grapheme clusters (ZWJ sequences, regional indicators, skin-tone
// modifiers, combining marks) count by the cluster's monospace
// width via UAX #11 / wcwidth.
func StringDisplayWidth(s string) int {
	return uniseg.StringWidth(s)
}

// CStrDisplayWidth is like StringDisplayWidth but skips '~' hotkey
// markers that don't render as glyphs. '~' is single-byte ASCII and
// never appears inside an emoji cluster, so a simple byte strip is
// safe.
func CStrDisplayWidth(s string) int {
	if !containsByte(s, '~') {
		return uniseg.StringWidth(s)
	}
	// Strip '~' markers, then measure. Faster than per-cluster
	// post-filtering for the common menu-label case where '~' marks
	// just one hotkey letter.
	return uniseg.StringWidth(stripTildes(s))
}

// CopyDisplayCells returns the substring of s that occupies cells
// [startCol, startCol+maxWidth) in the rendered terminal. Whole
// grapheme clusters are kept together; if a wide cluster would
// straddle the end it is omitted entirely.
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
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		w := g.Width()
		if col >= endCol {
			break
		}
		if col >= startCol && col+w <= endCol {
			out = append(out, []byte(g.Str())...)
		}
		col += w
	}
	return string(out)
}

// containsByte reports whether s contains byte b.
func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}

// stripTildes returns s with every '~' removed.
func stripTildes(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '~' {
			out = append(out, s[i])
		}
	}
	return string(out)
}
