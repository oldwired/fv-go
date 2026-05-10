// Package sixel emits DEC SIXEL DCS strings for terminals that support
// the protocol (iTerm2, Windows Terminal recently, mlterm, xterm with
// --enable-sixel-graphics, kitty in xterm-compat mode, …).
//
// Two encoders are provided:
//
//	EncodeRealtime — fixed 6×6×6 RGB cube (216 colors), no dithering,
//	                 nearest-neighbor scaling. Single-pass, no allocation
//	                 per pixel — fast enough for animation.
//	(EncodeQuality — adaptive palette, dithering — deferred.)
//
// The output is a self-contained DCS string starting with "\x1bP" and
// ending with "\x1b\\" (ST). Caller is responsible for cursor placement
// (CSI Pn;Pn H) before emitting the string — SIXEL is rendered at the
// current cursor position, growing rightward and downward in cell space.
package sixel

import (
	"image"
	"image/color"
	"strconv"
	"strings"
)

// cubeIndex maps an 8-bit-per-channel RGB triple to a 6×6×6 cube index
// in [0, 216). Each channel is rounded to the nearest of {0, 51, 102,
// 153, 204, 255} (i.e. {0, 20%, 40%, 60%, 80%, 100%}).
//
//	idx = R_level * 36 + G_level * 6 + B_level
//
// This is the same scheme TSixelEncoder.EncodeRealtime uses in the
// Delphi reference — it lets us emit a single fixed palette per frame
// and skip per-frame palette construction entirely.
func cubeIndex(r, g, b uint8) int {
	return int((int(r)+25)/51)*36 + int((int(g)+25)/51)*6 + int((int(b)+25)/51)
}

// pctForLevel returns the SIXEL palette percentage (0..100) for a cube
// level (0..5). Levels are evenly spaced.
func pctForLevel(level int) int { return level * 20 }

// EncodeRealtime renders src to a SIXEL DCS string. scale is a positive
// integer pixel-replication factor (1 = native size, 2 = 2× nearest-
// neighbor, …). The output's pixel dimensions are
// src.Bounds().Dx()*scale × src.Bounds().Dy()*scale.
//
// Color quantization is the 216-entry cube: each pixel is mapped to the
// nearest cube entry independently. There is no dithering — solid
// regions encode cleanly but smooth gradients will band. For game-style
// art this is usually what you want.
func EncodeRealtime(src image.Image, scale int) string {
	if scale < 1 {
		scale = 1
	}
	b := src.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW <= 0 || srcH <= 0 {
		return ""
	}
	w := srcW * scale
	h := srcH * scale

	// Quantize once into a flat grid of cube indices. Using a 2D []byte
	// keeps memory tight (one byte per pixel) and lets us index by
	// y*w+x in the inner loop — important since the band scan visits
	// every pixel up to 216 times.
	grid := make([]byte, w*h)
	for y := 0; y < h; y++ {
		sy := b.Min.Y + y/scale
		for x := 0; x < w; x++ {
			sx := b.Min.X + x/scale
			r, g, bl, _ := src.At(sx, sy).RGBA()
			grid[y*w+x] = byte(cubeIndex(uint8(r>>8), uint8(g>>8), uint8(bl>>8)))
		}
	}

	var sb strings.Builder
	// Reserve a generous initial capacity. Worst case is ~1 byte per
	// pixel per color used + palette + headers; in practice 1× the
	// pixel count is plenty.
	sb.Grow(w*h + 4096)

	// DCS Pq=0 (use background mode 0), Pa=1 (1:1 aspect), Pb=0 q.
	// Followed by raster attributes "1;1;Ph;Pv (1:1 aspect, Ph=width,
	// Pv=height) so terminals don't autosize from pixel data.
	sb.WriteString("\x1bP0;1;0q\"1;1;")
	sb.WriteString(strconv.Itoa(w))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(h))

	// Define the 216 palette entries up front. Mode 2 = RGB percent.
	for i := 0; i < 216; i++ {
		rl, gl, bl := i/36, (i/6)%6, i%6
		sb.WriteByte('#')
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(";2;")
		sb.WriteString(strconv.Itoa(pctForLevel(rl)))
		sb.WriteByte(';')
		sb.WriteString(strconv.Itoa(pctForLevel(gl)))
		sb.WriteByte(';')
		sb.WriteString(strconv.Itoa(pctForLevel(bl)))
	}

	// Emit pixel data in 6-row bands. For each band we visit each color
	// that appears and write a sixel character per column with the bit
	// pattern of which of the 6 rows that color occupies. `$` returns
	// to the start of the band so multiple colors can overlay; `-`
	// advances to the next band.
	//
	// We scan colors-per-band rather than per-pixel because a SIXEL
	// "select color #N" applies until changed — so the cheapest layout
	// is "for each color, paint all its pixels in this band, then
	// reset". The per-band presence map (used) avoids emitting empty
	// passes.
	bitmaskRow := make([]byte, w)
	used := make([]bool, 216)
	for bandY := 0; bandY < h; bandY += 6 {
		bandH := 6
		if bandY+bandH > h {
			bandH = h - bandY
		}
		// Mark which colors appear in this band.
		for i := range used {
			used[i] = false
		}
		for dy := 0; dy < bandH; dy++ {
			row := grid[(bandY+dy)*w : (bandY+dy+1)*w]
			for x := 0; x < w; x++ {
				used[row[x]] = true
			}
		}
		first := true
		for c := 0; c < 216; c++ {
			if !used[c] {
				continue
			}
			if !first {
				sb.WriteByte('$') // CR back to band start; next color overlays
			}
			first = false
			sb.WriteByte('#')
			sb.WriteString(strconv.Itoa(c))
			// Build bitmask for this color across the band.
			cb := byte(c)
			for x := 0; x < w; x++ {
				var m byte
				for dy := 0; dy < bandH; dy++ {
					if grid[(bandY+dy)*w+x] == cb {
						m |= 1 << dy
					}
				}
				bitmaskRow[x] = m
			}
			// Run-length encode runs of the same byte: "!N<char>".
			// The encoded char is '?' + bitmask (0x3F + 0..63).
			x := 0
			for x < w {
				m := bitmaskRow[x]
				run := 1
				for x+run < w && bitmaskRow[x+run] == m {
					run++
				}
				ch := byte('?' + m)
				if run >= 4 {
					sb.WriteByte('!')
					sb.WriteString(strconv.Itoa(run))
					sb.WriteByte(ch)
				} else {
					for i := 0; i < run; i++ {
						sb.WriteByte(ch)
					}
				}
				x += run
			}
		}
		sb.WriteByte('-') // advance to next 6-row band
	}

	sb.WriteString("\x1b\\")
	return sb.String()
}

// CubeColor returns the RGBA representation of cube entry idx (0..215).
// Mostly useful for tests and for previewing what the realtime palette
// looks like — every cube index round-trips through cubeIndex back to
// itself.
func CubeColor(idx int) color.RGBA {
	if idx < 0 || idx >= 216 {
		return color.RGBA{}
	}
	rl, gl, bl := idx/36, (idx/6)%6, idx%6
	levels := [6]uint8{0, 51, 102, 153, 204, 255}
	return color.RGBA{R: levels[rl], G: levels[gl], B: levels[bl], A: 255}
}
