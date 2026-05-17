// Package types provides the core value types used throughout fv-go:
// draw cells, screen cells, palette colors, and text attributes.
//
// Ported from Delphi unit FVCommon.pas. Pascal records map to Go structs;
// the equality operator on TDrawCell is replaced by Go == (works because
// all fields are comparable, including the string Ch).
package types

// Extended attribute flags packed into DrawCell.ExtAttrs.
const (
	EAItalic        byte = 0x01 // SGR 3
	EAStrikethrough byte = 0x02 // SGR 9
	EAUnderMask     byte = 0x1C // bits 2-4: underline style
	EAUnderShift    byte = 2
	EADim           byte = 0x20 // SGR 2
	EAOverline      byte = 0x40 // SGR 53
)

// Underline style values stored in bits 2..4 of ExtAttrs.
const (
	UnderNone   byte = 0
	UnderSingle byte = 1 // SGR 4
	UnderDouble byte = 2 // SGR 21
	UnderCurly  byte = 3 // SGR 4:3
	UnderDotted byte = 4 // SGR 4:4
	UnderDashed byte = 5 // SGR 4:5
)

// SixelPlaceholder is a Private Use Area code point reserved in the cell
// stream to mark the location of a sixel image. Renderer skips drawing.
const SixelPlaceholder = ''

// DrawCell is what every View.Draw writes into. The renderer eventually
// resolves these into terminal output.
//
// Attr packs background in the high byte and foreground in the low byte
// (Turbo Vision's classic format). FGRGB / BGRGB hold true-color overrides;
// zero means "use the palette byte from Attr". ExtAttrs holds italic,
// strikethrough, dim, overline, and an underline style nibble.
type DrawCell struct {
	Ch           string // grapheme cluster (1+ code points)
	Attr         uint16 // hi byte = BG, lo byte = FG
	FGRGB        uint32 // 0x00RRGGBB; 0 means use palette byte
	BGRGB        uint32 // 0x00RRGGBB; 0 means use palette byte
	ExtAttrs     byte   // EA* flags
	ULRGB        uint32 // 0x00RRGGBB; 0 means use FG color for underline
	HyperlinkURL string // OSC 8 link; empty = none
}

// ScreenCell is the resolved form held by the screen buffer between
// frame flushes. Compared to DrawCell, the booleans are unpacked and
// FG/BG are individual indices not the legacy attribute byte.
type ScreenCell struct {
	Ch             string
	FG             byte
	BG             byte
	Bold           bool
	Underline      bool // mirrors UnderlineStyle > 0
	Inverse        bool
	Italic         bool
	Strikethrough  bool
	UnderlineStyle byte
	Dim            bool
	Overline       bool
	FGRGB          uint32
	BGRGB          uint32
	ULRGB          uint32
	HyperlinkURL   string
}

// EmptyScreenCell returns a blank space with the default light-gray-on-black
// attributes used by Turbo Vision's idle background.
func EmptyScreenCell() ScreenCell {
	return ScreenCell{
		Ch: " ",
		FG: 7,
		BG: 0,
	}
}

// SetUnderlineStyle writes the underline style nibble into ExtAttrs.
func (c *DrawCell) SetUnderlineStyle(style byte) {
	c.ExtAttrs = (c.ExtAttrs &^ EAUnderMask) | ((style << EAUnderShift) & EAUnderMask)
}

// UnderlineStyle reads the underline style nibble from ExtAttrs.
// Pointer receiver for consistency with SetUnderlineStyle so the
// method set is uniform — same rationale as CommandSet.
func (c *DrawCell) UnderlineStyle() byte {
	return (c.ExtAttrs & EAUnderMask) >> EAUnderShift
}

// MakeAttr packs Turbo-Vision-style bg/fg into a single uint16 attribute.
// Both inputs are palette indices in [0,15] for the classic 16-color mode,
// or [0,255] when the renderer is in 256-color mode.
func MakeAttr(fg, bg byte) uint16 {
	return uint16(fg) | uint16(bg)<<8
}

// FG returns the foreground palette byte.
func FG(attr uint16) byte { return byte(attr & 0xFF) }

// BG returns the background palette byte.
func BG(attr uint16) byte { return byte(attr >> 8) }

// Min/Max helpers that mirror the FVCommon names (typed for int).
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
