// Package screen wraps term.Backend with the FV-style draw helpers
// every view uses (DrawChar, DrawStr, DrawCStr, DrawBuf). It also
// exposes the global cursor + size helpers that App.Init populates.
//
// Views never write directly to the backend — they fill a TDrawBuffer
// (slice of types.DrawCell), then call screen.WriteLine which does
// Z-order clipping in the view layer and ultimately lands on
// backend.SetCell.
package screen

import (
	stdutf8 "unicode/utf8"

	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
)

// MaxViewWidth caps the per-line draw buffer size, mirroring
// Drivers.pas MaxViewWidth.
const MaxViewWidth = 2048

// DrawBuffer is the row buffer views fill in their Draw method.
type DrawBuffer []types.DrawCell

// MakeDrawBuffer returns a fresh buffer of length n with empty cells.
func MakeDrawBuffer(n int) DrawBuffer {
	if n > MaxViewWidth {
		n = MaxViewWidth
	}
	b := make(DrawBuffer, n)
	for i := range b {
		b[i].Ch = " "
	}
	return b
}

// DrawCell writes one cell at pos. No-op if pos is OOB.
func DrawCell(buf DrawBuffer, pos int, ch string, attr uint16) {
	if pos < 0 || pos >= len(buf) {
		return
	}
	if ch == "" {
		ch = " "
	}
	buf[pos] = types.DrawCell{Ch: ch, Attr: attr}
}

// DrawChar writes count copies of ch at pos with attr.
func DrawChar(buf DrawBuffer, pos int, ch rune, attr uint16, count int) {
	s := string(ch)
	for i := 0; i < count; i++ {
		DrawCell(buf, pos+i, s, attr)
	}
}

// DrawStr writes s starting at pos. Stops at end-of-buffer. Each
// rune occupies one cell except wide runes, which take two.
func DrawStr(buf DrawBuffer, pos int, s string, attr uint16) {
	x := pos
	for _, r := range s {
		if x < 0 || x >= len(buf) {
			break
		}
		w := utf8.RuneCellWidth(r)
		if w == 0 {
			// combining mark: append to previous cell
			if x > 0 {
				buf[x-1].Ch += string(r)
			}
			continue
		}
		buf[x] = types.DrawCell{Ch: string(r), Attr: attr}
		x++
		if w == 2 && x < len(buf) {
			// skip a cell so the next ASCII char doesn't overlap the
			// wide glyph; mark as continuation
			buf[x] = types.DrawCell{Ch: "", Attr: attr}
			x++
		}
	}
}

// DrawCStr writes s with hotkey-marker semantics: '~' toggles between
// the normal and hot attributes. Each attribute is a full uint16 packed
// FG-in-low-byte, BG-in-high-byte (matching types.MakeAttr).
//
// The Pascal API squeezed both attrs into a single Word with one byte
// each; that constrains menus to 16 colors. We take two uint16s instead
// — the menu code already has both attrs in hand, and this avoids a
// silent attribute corruption that the old packed layout produced.
func DrawCStr(buf DrawBuffer, pos int, s string, normal, hot uint16) {
	cur := normal
	x := pos
	i := 0
	for i < len(s) {
		if s[i] == '~' {
			if cur == normal {
				cur = hot
			} else {
				cur = normal
			}
			i++
			continue
		}
		r, sz := stdutf8.DecodeRuneInString(s[i:])
		i += sz
		if x < 0 || x >= len(buf) {
			break
		}
		w := utf8.RuneCellWidth(r)
		if w == 0 {
			if x > 0 {
				buf[x-1].Ch += string(r)
			}
			continue
		}
		buf[x] = types.DrawCell{Ch: string(r), Attr: cur}
		x++
		if w == 2 && x < len(buf) {
			buf[x] = types.DrawCell{Ch: "", Attr: cur}
			x++
		}
	}
}

// DrawBuf copies count cells from src[srcPos:] into dest[destPos:].
func DrawBuf(dest DrawBuffer, destPos int, src DrawBuffer, srcPos int, count int) {
	for i := 0; i < count; i++ {
		di := destPos + i
		si := srcPos + i
		if di < 0 || di >= len(dest) || si < 0 || si >= len(src) {
			return
		}
		dest[di] = src[si]
	}
}
