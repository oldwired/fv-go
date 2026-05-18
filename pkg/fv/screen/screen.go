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
	"github.com/oldwired/fv-go/pkg/fv/types"

	"github.com/rivo/uniseg"
)

// MaxViewWidth caps the per-line draw buffer size, mirroring
// Drivers.pas MaxViewWidth.
const MaxViewWidth = 2048

// DrawBuffer is the row buffer views fill in their Draw method.
type DrawBuffer []types.DrawCell

// MakeDrawBuffer returns a fresh buffer of length n with empty cells.
//
// Defensive clamps on both ends: n < 0 yields a zero-length buffer
// rather than panicking inside `make`, and n > MaxViewWidth is
// truncated. Without the negative-n guard, a Window resized to a
// dimension narrower than a fixed-bound child (no GrowMode set)
// produces a child Size.X that's negative — every Draw method's
// first line, `buf := screen.MakeDrawBuffer(b.Size.X)`, would
// then panic with "len out of range." The fix at the resize site
// (window.go) is the primary remedy; this is the backstop.
func MakeDrawBuffer(n int) DrawBuffer {
	if n < 0 {
		n = 0
	}
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

// DrawStr writes s starting at pos. Stops at end-of-buffer.
//
// Iterates s as Unicode grapheme clusters via uniseg, so multi-codepoint
// glyphs (ZWJ sequences like family 👨‍👩‍👧‍👦 and rainbow flag 🏳️‍🌈,
// regional-indicator country flags like 🇩🇪, skin-tone modifiers like
// 👋🏼, and combining marks) are atomic: each cluster lands in one
// cellbuf cell, plus a continuation cell (Ch="") when the cluster is
// 2 cells wide. This keeps the cellbuf advance in lockstep with the
// terminal's cursor advance per UAX #29 + UAX #11.
//
// When pos < 0, clusters whose cell position is still negative are
// skipped (width still accounted for), then drawing resumes once
// the column reaches 0.
func DrawStr(buf DrawBuffer, pos int, s string, attr uint16) {
	x := pos
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		cluster := g.Str()
		w := g.Width()

		if w == 0 {
			// Zero-width cluster (rare — typically only at the start
			// of a string before any base char). Attach defensively
			// to the previous cell so we never silently drop bytes.
			if x > 0 && x-1 < len(buf) {
				buf[x-1].Ch += cluster
			}
			continue
		}

		if x >= len(buf) {
			break
		}
		if x < 0 {
			x += w
			continue
		}
		buf[x] = types.DrawCell{Ch: cluster, Attr: attr}
		x++
		if w == 2 && x < len(buf) {
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
//
// Cluster handling is the same as DrawStr's. '~' is single-byte
// ASCII and can never appear inside a multi-codepoint cluster, so
// we run a pre-pass that splits s on '~' into alternating
// normal/hot segments, then DrawStr-style emit each segment.
func DrawCStr(buf DrawBuffer, pos int, s string, normal, hot uint16) {
	cur := normal
	x := pos
	segStart := 0
	emit := func(seg string) {
		if seg == "" {
			return
		}
		g := uniseg.NewGraphemes(seg)
		for g.Next() {
			cluster := g.Str()
			w := g.Width()
			if w == 0 {
				if x > 0 && x-1 < len(buf) {
					buf[x-1].Ch += cluster
				}
				continue
			}
			if x >= len(buf) {
				return
			}
			if x < 0 {
				x += w
				continue
			}
			buf[x] = types.DrawCell{Ch: cluster, Attr: cur}
			x++
			if w == 2 && x < len(buf) {
				buf[x] = types.DrawCell{Ch: "", Attr: cur}
				x++
			}
		}
	}
	for i := 0; i < len(s); i++ {
		if s[i] != '~' {
			continue
		}
		emit(s[segStart:i])
		if cur == normal {
			cur = hot
		} else {
			cur = normal
		}
		segStart = i + 1
	}
	emit(s[segStart:])
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
