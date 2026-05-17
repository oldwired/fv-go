package sixel

import (
	"image"
	"sort"
	"strconv"
	"strings"
)

// EncodeQuality renders src as a SIXEL DCS string using an adaptive
// palette (median-cut) plus Floyd-Steinberg error diffusion. Slower
// than EncodeRealtime but produces dramatically better photographic
// images — the 6×6×6 cube of the realtime path bands hard on
// gradients, while median-cut allocates colors where the image
// actually has them.
//
// maxColors is clamped to [2, 256]; pass 0 for the default of 128 (a
// good speed/fidelity trade-off for most images and well within the
// SIXEL register space).
func EncodeQuality(src image.Image, maxColors int) string {
	if maxColors <= 0 {
		maxColors = 128
	}
	if maxColors < 2 {
		maxColors = 2
	}
	if maxColors > 256 {
		maxColors = 256
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return ""
	}

	// Pull every pixel into a working buffer of int16 RGB so the
	// dither pass can carry signed error into neighbors without
	// allocating per cell.
	pixels := make([]rgbInt16, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pr, pg, pb, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			pixels[y*w+x] = rgbInt16{int16(pr >> 8), int16(pg >> 8), int16(pb >> 8)}
		}
	}

	palette := buildMedianCutPalette(pixels, w, h, maxColors)

	// Quantize with Floyd-Steinberg. Per-pixel error spreads to the
	// right (7/16), below-left (3/16), below (5/16), below-right
	// (1/16). The result is a flat int grid of palette indices.
	indices := make([]byte, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			p := pixels[i]
			idx, e := nearestPaletteEntry(p.r, p.g, p.b, palette)
			indices[i] = byte(idx)
			if x+1 < w {
				dither(&pixels[i+1], e, 7)
			}
			if y+1 < h {
				if x > 0 {
					dither(&pixels[i+w-1], e, 3)
				}
				dither(&pixels[i+w], e, 5)
				if x+1 < w {
					dither(&pixels[i+w+1], e, 1)
				}
			}
		}
	}

	// Emit DCS, raster header, palette, then band-major run-length
	// SIXEL data — same shape as the realtime encoder but with a
	// caller-defined palette.
	var sb strings.Builder
	sb.Grow(w*h + 4096)
	sb.WriteString("\x1bP0;1;0q\"1;1;")
	sb.WriteString(strconv.Itoa(w))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(h))
	for i, c := range palette {
		sb.WriteByte('#')
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(";2;")
		sb.WriteString(strconv.Itoa(int(c.r) * 100 / 255))
		sb.WriteByte(';')
		sb.WriteString(strconv.Itoa(int(c.g) * 100 / 255))
		sb.WriteByte(';')
		sb.WriteString(strconv.Itoa(int(c.b) * 100 / 255))
	}

	bitmaskRow := make([]byte, w)
	used := make([]bool, len(palette))
	for bandY := 0; bandY < h; bandY += 6 {
		bandH := 6
		if bandY+bandH > h {
			bandH = h - bandY
		}
		for i := range used {
			used[i] = false
		}
		for dy := 0; dy < bandH; dy++ {
			row := indices[(bandY+dy)*w : (bandY+dy+1)*w]
			for x := 0; x < w; x++ {
				used[row[x]] = true
			}
		}
		first := true
		for c := 0; c < len(palette); c++ {
			if !used[c] {
				continue
			}
			if !first {
				sb.WriteByte('$')
			}
			first = false
			sb.WriteByte('#')
			sb.WriteString(strconv.Itoa(c))
			cb := byte(c)
			for x := 0; x < w; x++ {
				var m byte
				for dy := 0; dy < bandH; dy++ {
					if indices[(bandY+dy)*w+x] == cb {
						m |= 1 << dy
					}
				}
				bitmaskRow[x] = m
			}
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
		sb.WriteByte('-')
	}
	sb.WriteString("\x1b\\")
	return sb.String()
}

// paletteEntry is the median-cut output: 8-bit-per-channel RGB.
type paletteEntry struct{ r, g, b uint8 }

// buildMedianCutPalette takes the source pixels and returns up to
// maxColors representative entries chosen by recursive longest-axis
// median splits. The classic algorithm — fast and good enough for
// terminal-scale images. We sample at most ~16k pixels for the
// bucket construction; full-image sweeps cost RAM with no fidelity
// gain (the dither pass is what makes it look right).
func buildMedianCutPalette(pixels []rgbInt16, w, h int, maxColors int) []paletteEntry {
	sample := pixels
	const sampleCap = 16384
	if len(pixels) > sampleCap {
		sample = make([]rgbInt16, sampleCap)
		step := len(pixels) / sampleCap
		for i := 0; i < sampleCap; i++ {
			sample[i] = pixels[i*step]
		}
	}
	// Work on copies — the dither path mutates pixels in place.
	work := make([]rgbInt16, len(sample))
	copy(work, sample)
	buckets := [][]rgbInt16{work}
	for len(buckets) < maxColors {
		// Find the bucket with the longest axis to split.
		bestIdx := -1
		bestRange := int16(0)
		bestAxis := 0
		for i, bk := range buckets {
			if len(bk) < 2 {
				continue
			}
			rMin, rMax, gMin, gMax, bMin, bMax := channelRanges(bk)
			ranges := [3]int16{rMax - rMin, gMax - gMin, bMax - bMin}
			ax := 0
			if ranges[1] > ranges[ax] {
				ax = 1
			}
			if ranges[2] > ranges[ax] {
				ax = 2
			}
			if ranges[ax] > bestRange {
				bestRange = ranges[ax]
				bestIdx = i
				bestAxis = ax
			}
		}
		if bestIdx < 0 || bestRange == 0 {
			break
		}
		bk := buckets[bestIdx]
		sort.Slice(bk, func(i, j int) bool {
			switch bestAxis {
			case 0:
				return bk[i].r < bk[j].r
			case 1:
				return bk[i].g < bk[j].g
			default:
				return bk[i].b < bk[j].b
			}
		})
		mid := len(bk) / 2
		left, right := bk[:mid], bk[mid:]
		buckets[bestIdx] = left
		buckets = append(buckets, right)
	}
	out := make([]paletteEntry, 0, len(buckets))
	for _, bk := range buckets {
		if len(bk) == 0 {
			continue
		}
		var rs, gs, bs int64
		for _, p := range bk {
			rs += int64(p.r)
			gs += int64(p.g)
			bs += int64(p.b)
		}
		n := int64(len(bk))
		out = append(out, paletteEntry{
			r: uint8(rs / n),
			g: uint8(gs / n),
			b: uint8(bs / n),
		})
	}
	return out
}

// rgbInt16 is the working pixel type; named so buildMedianCutPalette
// can take it without depending on the function-local type from
// EncodeQuality.
type rgbInt16 struct{ r, g, b int16 }

// channelRanges returns min/max for each channel of bk.
func channelRanges(bk []rgbInt16) (rMin, rMax, gMin, gMax, bMin, bMax int16) {
	rMin, gMin, bMin = 1<<14, 1<<14, 1<<14
	for _, p := range bk {
		if p.r < rMin {
			rMin = p.r
		}
		if p.r > rMax {
			rMax = p.r
		}
		if p.g < gMin {
			gMin = p.g
		}
		if p.g > gMax {
			gMax = p.g
		}
		if p.b < bMin {
			bMin = p.b
		}
		if p.b > bMax {
			bMax = p.b
		}
	}
	return
}

// nearestPaletteEntry returns the closest palette index to (r, g, b)
// (Manhattan distance, fast and close enough for terminal output) and
// the residual error per channel as int16. Error feeds the
// Floyd-Steinberg dither pass.
func nearestPaletteEntry(r, g, b int16, palette []paletteEntry) (int, [3]int16) {
	bestIdx := 0
	bestDist := int32(1 << 30)
	for i, p := range palette {
		dr := int32(r) - int32(p.r)
		dg := int32(g) - int32(p.g)
		db := int32(b) - int32(p.b)
		d := absInt32(dr) + absInt32(dg) + absInt32(db)
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	p := palette[bestIdx]
	return bestIdx, [3]int16{r - int16(p.r), g - int16(p.g), b - int16(p.b)}
}

func absInt32(n int32) int32 {
	if n < 0 {
		return -n
	}
	return n
}

// dither adds (err * weight / 16) to dst, clamped to [0, 255]. Used by
// Floyd-Steinberg to propagate quantization residue.
func dither(dst *rgbInt16, err [3]int16, weight int16) {
	dst.r = clampInt16(int32(dst.r) + int32(err[0])*int32(weight)/16)
	dst.g = clampInt16(int32(dst.g) + int32(err[1])*int32(weight)/16)
	dst.b = clampInt16(int32(dst.b) + int32(err[2])*int32(weight)/16)
}

func clampInt16(v int32) int16 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return int16(v)
}
