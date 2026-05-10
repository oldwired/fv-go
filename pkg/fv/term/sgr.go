package term

import (
	"strconv"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/profile"
	"github.com/oldwired/fv-go/pkg/fv/types"
)

// sgrEncoder builds the minimal escape sequences to transition from
// one (fg, bg, extAttrs, fgRGB, bgRGB) state to another. It tracks the
// previously-applied state so consecutive runs sharing a style emit no
// redundant SGR.
type sgrEncoder struct {
	color   profile.ColorSystem
	last    sgrState
	hasLast bool
}

type sgrState struct {
	fg, bg byte
	fgRGB  uint32
	bgRGB  uint32
	ext    byte
}

func newSGREncoder(color profile.ColorSystem) *sgrEncoder {
	return &sgrEncoder{color: color}
}

// reset emits SGR 0; sgrEncoder will then re-establish the next state.
func (e *sgrEncoder) reset(b *strings.Builder) {
	b.WriteString("\x1b[0m")
	e.hasLast = false
}

// transition emits exactly the SGR codes needed to change from e.last
// to next. If hasLast is false, all attributes are emitted from scratch.
func (e *sgrEncoder) transition(b *strings.Builder, next sgrState) {
	if e.hasLast && e.last == next {
		return
	}

	var parts []string

	// On any extAttr change, do a full reset+rebuild — there's no SGR
	// for "turn off italic only" without turning others off too.
	rebuild := !e.hasLast || e.last.ext != next.ext

	if rebuild {
		parts = append(parts, "0")
		parts = append(parts, encodeFG(e.color, next.fg, next.fgRGB)...)
		parts = append(parts, encodeBG(e.color, next.bg, next.bgRGB)...)
		parts = append(parts, encodeExt(next.ext)...)
	} else {
		if e.last.fg != next.fg || e.last.fgRGB != next.fgRGB {
			parts = append(parts, encodeFG(e.color, next.fg, next.fgRGB)...)
		}
		if e.last.bg != next.bg || e.last.bgRGB != next.bgRGB {
			parts = append(parts, encodeBG(e.color, next.bg, next.bgRGB)...)
		}
	}

	if len(parts) > 0 {
		b.WriteString("\x1b[")
		b.WriteString(strings.Join(parts, ";"))
		b.WriteString("m")
	}
	e.last = next
	e.hasLast = true
}

func encodeFG(cs profile.ColorSystem, idx byte, rgb uint32) []string {
	if cs == profile.NoColors {
		return nil
	}
	if rgb != 0 {
		switch cs {
		case profile.TrueColor:
			r, g, bl := splitRGB(rgb)
			return []string{"38", "2", strconv.Itoa(int(r)), strconv.Itoa(int(g)), strconv.Itoa(int(bl))}
		case profile.EightBit:
			return []string{"38", "5", strconv.Itoa(int(rgbTo256(rgb)))}
		}
		// fall through to legacy mapping
		idx = rgbToLegacy(rgb)
	}
	return []string{strconv.Itoa(int(legacyFGCode(idx)))}
}

func encodeBG(cs profile.ColorSystem, idx byte, rgb uint32) []string {
	if cs == profile.NoColors {
		return nil
	}
	if rgb != 0 {
		switch cs {
		case profile.TrueColor:
			r, g, bl := splitRGB(rgb)
			return []string{"48", "2", strconv.Itoa(int(r)), strconv.Itoa(int(g)), strconv.Itoa(int(bl))}
		case profile.EightBit:
			return []string{"48", "5", strconv.Itoa(int(rgbTo256(rgb)))}
		}
		idx = rgbToLegacy(rgb)
	}
	return []string{strconv.Itoa(int(legacyBGCode(idx)))}
}

func encodeExt(ext byte) []string {
	var out []string
	if ext&types.EAItalic != 0 {
		out = append(out, "3")
	}
	if ext&types.EADim != 0 {
		out = append(out, "2")
	}
	if ext&types.EAStrikethrough != 0 {
		out = append(out, "9")
	}
	if ext&types.EAOverline != 0 {
		out = append(out, "53")
	}
	switch (ext & types.EAUnderMask) >> types.EAUnderShift {
	case types.UnderSingle:
		out = append(out, "4")
	case types.UnderDouble:
		out = append(out, "21")
	case types.UnderCurly:
		out = append(out, "4:3")
	case types.UnderDotted:
		out = append(out, "4:4")
	case types.UnderDashed:
		out = append(out, "4:5")
	}
	return out
}

// cgaToANSI maps the IBM-PC / Turbo-Vision palette (0..15) to the
// xterm/ANSI palette index (0..7). The two palettes disagree on the
// red/blue and yellow/magenta ordering, so a literal "30+idx" produces
// red where TV expects blue. Mapping derived from the standard CGA
// palette (https://en.wikipedia.org/wiki/Color_Graphics_Adapter).
//
//	CGA index → CGA color   → ANSI code
//	0           black           0
//	1           blue            4
//	2           green           2
//	3           cyan            6
//	4           red             1
//	5           magenta         5
//	6           brown / yellow  3
//	7           light gray      7
//
// Bright variants (8..15) follow the same pattern.
var cgaToANSI = [8]byte{0, 4, 2, 6, 1, 5, 3, 7}

// legacyFGCode maps a TV palette byte (0..15) to an SGR foreground code
// (30..37 / 90..97). High-bit bright variants use the 90+ range rather
// than abusing SGR 1 (bold), which some terminals refuse to brighten.
func legacyFGCode(idx byte) byte {
	idx &= 0x0F
	if idx < 8 {
		return 30 + cgaToANSI[idx]
	}
	return 90 + cgaToANSI[idx-8]
}

func legacyBGCode(idx byte) byte {
	idx &= 0x0F
	if idx < 8 {
		return 40 + cgaToANSI[idx]
	}
	return 100 + cgaToANSI[idx-8]
}

func splitRGB(c uint32) (r, g, b byte) {
	return byte((c >> 16) & 0xFF), byte((c >> 8) & 0xFF), byte(c & 0xFF)
}

// rgbTo256 maps an RGB triple to the closest 6x6x6 cube index (16..231)
// or one of the 24 grayscale ramp entries (232..255).
func rgbTo256(c uint32) byte {
	r, g, b := splitRGB(c)
	// Grayscale ramp if r==g==b
	if r == g && g == b {
		switch {
		case r < 8:
			return 16
		case r > 248:
			return 231
		}
		return byte(232 + (int(r)-8)/10)
	}
	q := func(v byte) byte {
		switch {
		case v < 48:
			return 0
		case v < 115:
			return 1
		}
		return byte((int(v) - 35) / 40)
	}
	return 16 + 36*q(r) + 6*q(g) + q(b)
}

// rgbToLegacy maps an RGB triple to one of the 16 palette indices by
// quantizing each channel to bright/dim and mixing.
func rgbToLegacy(c uint32) byte {
	r, g, b := splitRGB(c)
	high := byte(0)
	if r >= 64 {
		high |= 1
	}
	if g >= 64 {
		high |= 2
	}
	if b >= 64 {
		high |= 4
	}
	bright := byte(0)
	if r >= 192 || g >= 192 || b >= 192 {
		bright = 8
	}
	return high | bright
}
