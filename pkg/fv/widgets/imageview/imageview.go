// Package imageview provides ImageView — a view that renders a
// stdlib image.Image either as Unicode half-blocks (works on every
// terminal) or as SIXEL graphics (when the host supports it).
//
// Half-block rendering uses U+2580 ("▀"). The cell's foreground RGB
// becomes the top pixel and the background RGB the bottom; one terminal
// cell therefore shows two vertical pixels. The result reads at roughly
// 50% vertical resolution but works everywhere truecolor SGR works.
//
// SIXEL rendering uses pkg/fv/sixel.EncodeRealtime and emits a DCS
// string anchored to the view's screen origin. The underlying cells
// are blanked so the cell-diff flush doesn't fight the graphics. A
// terminal that doesn't support SIXEL (and reports so via FV_SIXEL=0
// or by being unrecognized in sixel.IsSupported) silently falls back
// to half-block.
package imageview

import (
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/sixel"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// ImageView is the view.
type ImageView struct {
	views.Base

	Image    image.Image
	UseSixel bool // honored only when sixel.IsSupported(); otherwise we half-block

	// Pan offsets (image pixels from the top-left).
	OffsetX, OffsetY int

	// Zoom factor for SIXEL mode (integer pixel replication, ≥1).
	// Half-block ignores Zoom — cells dictate fit.
	Zoom int

	// Background color for cells outside the image / half-block bottom
	// when the image height is odd.
	BG uint32
}

// New constructs an ImageView at bounds with no image loaded yet.
// SetImage to populate. Default UseSixel mirrors sixel.IsSupported() so
// that callers don't have to gate at the call site — passing a SIXEL-
// supporting image to a non-SIXEL terminal still draws (as half-block).
func New(bounds geom.Rect) *ImageView {
	iv := &ImageView{
		Base:     views.NewBase(bounds),
		UseSixel: sixel.IsSupported(),
		Zoom:     1,
		BG:       0x010101, // not 0 (which means "use palette") — near-black
	}
	iv.SetSelf(iv)
	iv.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	iv.Options |= consts.OfSelectable
	iv.EventMask = consts.EvMouseDown | consts.EvKeyDown
	return iv
}

// GetTypeID for serial registry.
func (iv *ImageView) GetTypeID() string { return "imageview" }

// SetImage attaches img and resets pan/zoom.
func (iv *ImageView) SetImage(img image.Image) {
	iv.Image = img
	iv.OffsetX, iv.OffsetY = 0, 0
	iv.Zoom = 1
}

// HandleEvent implements basic pan/zoom controls:
//
//	arrows / Page{Up,Dn} / Home / End — pan
//	+ / -                              — zoom in/out (SIXEL only)
//	mouse wheel                        — vertical pan
func (iv *ImageView) HandleEvent(ev *drivers.Event) {
	if iv.Image == nil {
		return
	}
	if ev.What == consts.EvMouseDown {
		if ev.Buttons&consts.MbScrollWheelUp != 0 {
			iv.OffsetY -= 8
			iv.clampOffsets()
			iv.ClearEvent(ev)
			return
		}
		if ev.Buttons&consts.MbScrollWheelDown != 0 {
			iv.OffsetY += 8
			iv.clampOffsets()
			iv.ClearEvent(ev)
			return
		}
		// Plain click selects the view (focus). Don't consume so the
		// owning group can still move focus around.
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	step := 4
	switch ev.KeyCode {
	case consts.KbLeft:
		iv.OffsetX -= step
	case consts.KbRight:
		iv.OffsetX += step
	case consts.KbUp:
		iv.OffsetY -= step
	case consts.KbDown:
		iv.OffsetY += step
	case consts.KbPgUp:
		iv.OffsetY -= 40
	case consts.KbPgDn:
		iv.OffsetY += 40
	case consts.KbHome:
		iv.OffsetX, iv.OffsetY = 0, 0
	default:
		switch ev.UnicodeChar {
		case '+', '=':
			if iv.Zoom < 8 {
				iv.Zoom++
			}
		case '-', '_':
			if iv.Zoom > 1 {
				iv.Zoom--
			}
		default:
			return
		}
	}
	iv.clampOffsets()
	iv.ClearEvent(ev)
}

func (iv *ImageView) clampOffsets() {
	if iv.Image == nil {
		return
	}
	b := iv.Image.Bounds()
	if iv.OffsetX < 0 {
		iv.OffsetX = 0
	}
	if iv.OffsetY < 0 {
		iv.OffsetY = 0
	}
	maxX := b.Dx() - 1
	maxY := b.Dy() - 1
	if iv.OffsetX > maxX {
		iv.OffsetX = maxX
	}
	if iv.OffsetY > maxY {
		iv.OffsetY = maxY
	}
}

// Draw routes to half-block, sentinel-stamping (SIXEL), or BG fill.
// SIXEL emission is deferred to PreFlush so it can resolve z-order
// against any covering view; see PreFlush below.
func (iv *ImageView) Draw() {
	if iv.Image == nil {
		iv.fillBG()
		return
	}
	if iv.UseSixel && sixel.IsSupported() {
		iv.markSixelRegion()
		return
	}
	iv.drawHalfBlock()
}

// markSixelRegion stamps every cell in the view's bounds with the
// SIXEL placeholder rune. PreFlush resolves these against covering
// views.
func (iv *ImageView) markSixelRegion() {
	sentinel := types.DrawCell{Ch: string(types.SixelPlaceholder)}
	for y := 0; y < iv.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(iv.Size.X)
		for x := 0; x < iv.Size.X; x++ {
			buf[x] = sentinel
		}
		iv.WriteLine(0, y, iv.Size.X, 1, buf)
	}
}

func (iv *ImageView) fillBG() {
	for y := 0; y < iv.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(iv.Size.X)
		for x := 0; x < iv.Size.X; x++ {
			buf[x] = types.DrawCell{Ch: " ", BGRGB: nonzeroRGB(iv.BG)}
		}
		iv.WriteLine(0, y, iv.Size.X, 1, buf)
	}
}

// drawHalfBlock encodes the image as a grid of "▀" cells where the
// upper half block's foreground = top pixel and the cell's background =
// bottom pixel. The image is centered/cropped into Size.X cells across
// and Size.Y cells tall (i.e., 2·Size.Y pixels vertically).
func (iv *ImageView) drawHalfBlock() {
	cellsW, cellsH := iv.Size.X, iv.Size.Y
	if cellsW <= 0 || cellsH <= 0 {
		return
	}
	pixelsW := cellsW
	pixelsH := cellsH * 2

	b := iv.Image.Bounds()
	for cy := 0; cy < cellsH; cy++ {
		buf := screen.MakeDrawBuffer(cellsW)
		for cx := 0; cx < cellsW; cx++ {
			topX := iv.OffsetX + scaleAxis(cx, pixelsW, b.Dx())
			topY := iv.OffsetY + scaleAxis(cy*2, pixelsH, b.Dy())
			botY := iv.OffsetY + scaleAxis(cy*2+1, pixelsH, b.Dy())
			top := sampleRGB(iv.Image, b.Min.X+topX, b.Min.Y+topY, b)
			bot := sampleRGB(iv.Image, b.Min.X+topX, b.Min.Y+botY, b)
			buf[cx] = types.DrawCell{
				Ch:    "▀",
				FGRGB: nonzeroRGB(top),
				BGRGB: nonzeroRGB(bot),
			}
		}
		iv.WriteLine(0, cy, cellsW, 1, buf)
	}
}

// PreFlush implements views.PreFlusher. Sentinel cells written by
// Draw mean "I want SIXEL here"; PreFlush turns that into actual DCS
// emission while resolving z-order against any covering view (see
// SixelCanvasView.PreFlush for the full mechanics — same algorithm).
//
// Three pieces of stdout get emitted, in order, every frame the
// imageview is at least partially visible:
//
//  1. A BG fill (truecolor SGR + spaces) over every uncovered cell.
//     This claims the cell content as iv.BG so the desktop's wallpaper
//     pattern doesn't keep showing through wherever SIXEL pixels
//     don't reach (cell-size mismatch, or letterbox padding inside
//     the SIXEL itself).
//  2. The SIXEL DCS, painted on top of the BG fill.
//  3. (Implicit, in the cell flush) covering cells that were
//     Invalidated below — they paint on top of the SIXEL pixels so
//     overlapping windows survive the SIXEL repaint.
func (iv *ImageView) PreFlush(b views.RootBackend) {
	if iv.Image == nil || !iv.UseSixel || !sixel.IsSupported() {
		return
	}
	gx, gy := iv.ScreenOrigin()
	hasSentinel := false
	var sentinels, covered []cellPt
	for y := 0; y < iv.Size.Y; y++ {
		for x := 0; x < iv.Size.X; x++ {
			cell := b.GetCell(gx+x, gy+y)
			if cell.Ch == string(types.SixelPlaceholder) {
				sentinels = append(sentinels, cellPt{gx + x, gy + y})
				hasSentinel = true
			} else {
				covered = append(covered, cellPt{gx + x, gy + y})
			}
		}
	}
	if !hasSentinel {
		for _, p := range covered {
			b.Invalidate(p.x, p.y)
		}
		return
	}

	cellW, cellH := sixel.CellSize()
	pxW := iv.Size.X * cellW
	pxH := iv.Size.Y * cellH
	if pxW <= 0 || pxH <= 0 {
		return
	}
	canvas := fitImageToCanvas(iv.Image, pxW, pxH, iv.BG)
	dcs := sixel.EncodeRealtime(canvas, 1)
	if dcs == "" {
		return
	}

	// Step 1: emit BG fill over uncovered cells.
	if fill := buildBGFill(sentinels, iv.BG); fill != "" {
		_ = views.WriteRaw(fill)
	}
	// Step 2: emit SIXEL.
	move := "\x1b[" + strconv.Itoa(gy+1) + ";" + strconv.Itoa(gx+1) + "H"
	if err := views.WriteRaw(move + dcs); err != nil {
		return
	}
	// Step 3: settle the cell buffer. Sentinels become an iv.BG-colored
	// blank that's MarkClean'd against itself so the cell flush won't
	// paint over the SIXEL we just emitted. Covered cells get
	// Invalidated so they re-emit on top.
	bgCell := types.DrawCell{Ch: " ", BGRGB: nonzeroRGB(iv.BG)}
	for _, p := range sentinels {
		b.SetCell(p.x, p.y, bgCell)
		b.MarkClean(p.x, p.y)
	}
	for _, p := range covered {
		b.Invalidate(p.x, p.y)
	}
}

// cellPt is a screen-coords cell index used in PreFlush bookkeeping.
type cellPt struct{ x, y int }

// buildBGFill produces a single string of SGR + cursor moves + spaces
// that paints a true-color background over each (x, y) cell. Cells are
// assumed to come in row-major order; runs in the same row collapse
// into one cursor move + N spaces. Empty input returns "".
func buildBGFill(cells []cellPt, bg uint32) string {
	if len(cells) == 0 {
		return ""
	}
	r, g, blue := uint8((bg>>16)&0xFF), uint8((bg>>8)&0xFF), uint8(bg&0xFF)
	var sb strings.Builder
	sb.Grow(len(cells)*2 + 64)
	sb.WriteString("\x1b[48;2;")
	sb.WriteString(strconv.Itoa(int(r)))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(int(g)))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(int(blue)))
	sb.WriteByte('m')
	// Walk cells, batching runs that share a row and are contiguous in x.
	i := 0
	for i < len(cells) {
		runY := cells[i].y
		runStartX := cells[i].x
		j := i + 1
		for j < len(cells) && cells[j].y == runY && cells[j].x == cells[j-1].x+1 {
			j++
		}
		runLen := j - i
		// Cursor move: ANSI 1-based.
		sb.WriteString("\x1b[")
		sb.WriteString(strconv.Itoa(runY + 1))
		sb.WriteByte(';')
		sb.WriteString(strconv.Itoa(runStartX + 1))
		sb.WriteByte('H')
		for k := 0; k < runLen; k++ {
			sb.WriteByte(' ')
		}
		i = j
	}
	sb.WriteString("\x1b[0m")
	return sb.String()
}

// fitImageToCanvas builds a canvasW × canvasH RGBA buffer that contains
// src scaled to fit while preserving aspect ratio. The remainder is
// filled with bg. Nearest-neighbor sampling — fine for SIXEL where the
// 6×6×6 cube is the limiting factor on color fidelity anyway.
func fitImageToCanvas(src image.Image, canvasW, canvasH int, bg uint32) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	bgRGBA := color.RGBA{
		R: uint8((bg >> 16) & 0xFF),
		G: uint8((bg >> 8) & 0xFF),
		B: uint8(bg & 0xFF),
		A: 255,
	}
	for y := 0; y < canvasH; y++ {
		for x := 0; x < canvasW; x++ {
			out.SetRGBA(x, y, bgRGBA)
		}
	}
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw <= 0 || sh <= 0 {
		return out
	}
	// Aspect-preserving fit. Use float to avoid rounding-down to zero
	// for tiny images; clamp to ≥1 in each axis at the end.
	scaleW := float64(canvasW) / float64(sw)
	scaleH := float64(canvasH) / float64(sh)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}
	targetW := int(float64(sw) * scale)
	targetH := int(float64(sh) * scale)
	if targetW < 1 {
		targetW = 1
	}
	if targetH < 1 {
		targetH = 1
	}
	offX := (canvasW - targetW) / 2
	offY := (canvasH - targetH) / 2
	for y := 0; y < targetH; y++ {
		srcY := sb.Min.Y + y*sh/targetH
		for x := 0; x < targetW; x++ {
			srcX := sb.Min.X + x*sw/targetW
			r, g, bl, _ := src.At(srcX, srcY).RGBA()
			out.SetRGBA(offX+x, offY+y, color.RGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(bl >> 8),
				A: 255,
			})
		}
	}
	return out
}

// scaleAxis maps coordinate i in [0, dst) to a coordinate in [0, src)
// using nearest-neighbor scaling. Returns 0 when src or dst is zero.
func scaleAxis(i, dst, src int) int {
	if dst <= 0 || src <= 0 {
		return 0
	}
	r := i * src / dst
	if r >= src {
		r = src - 1
	}
	return r
}

// sampleRGB extracts a packed 0x00RRGGBB pixel from src at (x, y),
// clamped to bounds. Out-of-range coords return the BG sentinel so
// rendering past the image edge doesn't garbage-color those cells.
func sampleRGB(src image.Image, x, y int, b image.Rectangle) uint32 {
	if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
		return 0
	}
	r, g, bl, _ := src.At(x, y).RGBA()
	return uint32(r>>8)<<16 | uint32(g>>8)<<8 | uint32(bl>>8)
}

// nonzeroRGB shifts a 0 RGB to a near-black equivalent so the SGR
// encoder's "0 means use palette" sentinel doesn't drop true-color
// black to whatever the legacy palette index 0 is.
func nonzeroRGB(c uint32) uint32 {
	if c == 0 {
		return 0x010101
	}
	return c
}
