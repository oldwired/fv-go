// Package sixelcanvas provides SixelCanvasView — a fixed-size pixel
// buffer the application can draw into and have rendered as SIXEL
// graphics (or half-block fallback). Useful for game-style rendering,
// charts, and any "I have pixels, please put them on screen" use case
// where a plain ImageView with a static *image.RGBA isn't enough.
//
// Drawing primitives (Clear / SetPixel / FillRect / DrawLine) all
// operate on the canvas's pixel buffer; nothing is sent to the
// terminal until Draw runs. Animation: set OnTick to a callback and
// the view auto-registers a 50ms ticker (~20fps); each tick invokes
// the callback before the redraw, giving a simple "logical-frame"
// pattern without the caller having to wire anim.Register manually.
package sixelcanvas

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/sixel"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// SixelCanvasView is a pixel canvas with drawing primitives.
type SixelCanvasView struct {
	views.Base

	// PixelW × PixelH is the buffer's pixel resolution. Independent of
	// the view's cell-bounds; the encoder/half-block path stretches
	// the buffer to fit the cell rectangle.
	PixelW, PixelH int

	pixels *image.RGBA

	// OnTick is called once per frame just before Draw, if non-nil.
	// Callers stage their drawing primitives inside this hook.
	OnTick func()

	// UseSixel mirrors sixel.IsSupported() at construction; set to
	// false to force the half-block path.
	UseSixel bool

	// FrameInterval controls the animation cadence. Default 50ms when
	// OnTick is non-nil. Ignored when OnTick is nil.
	FrameInterval time.Duration

	// BG is the background color emitted in cells that the SIXEL
	// emission doesn't cover (cell-size mismatch, or padding when the
	// integer-scale-up undershoots the cell rect). Default 0x000000.
	BG uint32
}

// New constructs a SixelCanvasView with a pixelW × pixelH backing
// buffer, fitted into the supplied cell bounds.
func New(bounds geom.Rect, pixelW, pixelH int) *SixelCanvasView {
	if pixelW < 1 {
		pixelW = 1
	}
	if pixelH < 1 {
		pixelH = 1
	}
	c := &SixelCanvasView{
		Base:          views.NewBase(bounds),
		PixelW:        pixelW,
		PixelH:        pixelH,
		pixels:        image.NewRGBA(image.Rect(0, 0, pixelW, pixelH)),
		UseSixel:      sixel.IsSupported(),
		FrameInterval: 50 * time.Millisecond,
	}
	c.SetSelf(c)
	c.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	c.Options |= consts.OfSelectable
	// We always animate at FrameInterval — even with OnTick==nil this
	// is harmless and avoids races where OnTick is set after Insert.
	anim.Register(c, c.FrameInterval)
	return c
}

// GetTypeID for serial registry.
func (c *SixelCanvasView) GetTypeID() string { return "sixelcanvas" }

// Tick fires the user's OnTick (if any) and asks for a redraw. Returns
// true to mark dirty whenever something might have changed visually —
// we err on the side of redrawing because skipping a frame in
// animation feels worse than the cost of an extra paint.
func (c *SixelCanvasView) Tick(now time.Time) bool {
	if c.OnTick != nil {
		c.OnTick()
		return true
	}
	return false
}

// Pixels returns the underlying *image.RGBA so callers can draw into
// it with stdlib operations (image/draw, etc.).
func (c *SixelCanvasView) Pixels() *image.RGBA { return c.pixels }

// Resize replaces the pixel buffer with a new size, clearing it.
func (c *SixelCanvasView) Resize(pixelW, pixelH int) {
	if pixelW < 1 {
		pixelW = 1
	}
	if pixelH < 1 {
		pixelH = 1
	}
	c.PixelW = pixelW
	c.PixelH = pixelH
	c.pixels = image.NewRGBA(image.Rect(0, 0, pixelW, pixelH))
}

// --- drawing primitives ---------------------------------------------

// Clear paints the entire buffer with the given 24-bit RGB color.
func (c *SixelCanvasView) Clear(rgb uint32) {
	col := rgbaFrom(rgb)
	pix := c.pixels.Pix
	stride := c.pixels.Stride
	rowLen := c.PixelW * 4
	// Fill first row, then memcpy to subsequent rows.
	for x := 0; x < c.PixelW; x++ {
		pix[x*4+0] = col.R
		pix[x*4+1] = col.G
		pix[x*4+2] = col.B
		pix[x*4+3] = col.A
	}
	first := pix[:rowLen]
	for y := 1; y < c.PixelH; y++ {
		copy(pix[y*stride:y*stride+rowLen], first)
	}
}

// SetPixel writes one pixel, no-op if out of bounds.
func (c *SixelCanvasView) SetPixel(x, y int, rgb uint32) {
	if x < 0 || x >= c.PixelW || y < 0 || y >= c.PixelH {
		return
	}
	c.pixels.SetRGBA(x, y, rgbaFrom(rgb))
}

// FillRect paints a w×h rectangle starting at (x, y), clipped to the
// canvas bounds.
func (c *SixelCanvasView) FillRect(x, y, w, h int, rgb uint32) {
	col := rgbaFrom(rgb)
	x0, y0 := x, y
	x1, y1 := x+w, y+h
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > c.PixelW {
		x1 = c.PixelW
	}
	if y1 > c.PixelH {
		y1 = c.PixelH
	}
	for yy := y0; yy < y1; yy++ {
		for xx := x0; xx < x1; xx++ {
			c.pixels.SetRGBA(xx, yy, col)
		}
	}
}

// DrawLine draws a Bresenham line from (x1,y1) to (x2,y2). Out-of-
// bounds pixels are clipped at SetPixel time.
func (c *SixelCanvasView) DrawLine(x1, y1, x2, y2 int, rgb uint32) {
	dx := absInt(x2 - x1)
	dy := -absInt(y2 - y1)
	sx, sy := 1, 1
	if x1 >= x2 {
		sx = -1
	}
	if y1 >= y2 {
		sy = -1
	}
	err := dx + dy
	for {
		c.SetPixel(x1, y1, rgb)
		if x1 == x2 && y1 == y2 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x1 += sx
		}
		if e2 <= dx {
			err += dx
			y1 += sy
		}
	}
}

// --- rendering ------------------------------------------------------

// Draw routes to SIXEL or half-block.
//
// SIXEL path: writes a sentinel cell (Ch = SixelPlaceholder) into every
// cell of the view's region. PreFlush — which runs after the entire
// tree walk — then resolves z-order against whatever covering views
// drew on top, emits SIXEL DCS only at uncovered cells, and forces
// covering cells to re-emit so they paint over the SIXEL pixels.
//
// Half-block path: writes ordinary "▀" cells; the standard cell
// flush handles z-order automatically because cells overwrite cells.
func (c *SixelCanvasView) Draw() {
	if c.UseSixel && sixel.IsSupported() {
		c.markSixelRegion()
		return
	}
	c.drawHalfBlock()
}

// markSixelRegion fills the view with sentinel cells. The actual SIXEL
// emission happens in PreFlush once we know what's covering us.
func (c *SixelCanvasView) markSixelRegion() {
	sentinel := types.DrawCell{Ch: string(types.SixelPlaceholder)}
	for y := 0; y < c.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(c.Size.X)
		for x := 0; x < c.Size.X; x++ {
			buf[x] = sentinel
		}
		c.WriteLine(0, y, c.Size.X, 1, buf)
	}
}

func (c *SixelCanvasView) drawHalfBlock() {
	cellsW, cellsH := c.Size.X, c.Size.Y
	if cellsW <= 0 || cellsH <= 0 {
		return
	}
	pixelsH := cellsH * 2
	for cy := 0; cy < cellsH; cy++ {
		buf := screen.MakeDrawBuffer(cellsW)
		for cx := 0; cx < cellsW; cx++ {
			topX := scaleAxis(cx, cellsW, c.PixelW)
			topY := scaleAxis(cy*2, pixelsH, c.PixelH)
			botY := scaleAxis(cy*2+1, pixelsH, c.PixelH)
			top := samplePixel(c.pixels, topX, topY)
			bot := samplePixel(c.pixels, topX, botY)
			buf[cx] = types.DrawCell{
				Ch:    "▀",
				FGRGB: nonzeroRGB(top),
				BGRGB: nonzeroRGB(bot),
			}
		}
		c.WriteLine(0, cy, cellsW, 1, buf)
	}
}

// PreFlush implements views.PreFlusher. Runs after the tree walk,
// before the cell-buffer flush. See ImageView.PreFlush for the full
// algorithm — this is the same shape, with c.BG taking the place of
// iv.BG for the under-the-SIXEL background fill.
func (c *SixelCanvasView) PreFlush(b views.RootBackend) {
	if !c.UseSixel || !sixel.IsSupported() {
		return
	}
	gx, gy := c.ScreenOrigin()
	hasSentinel := false
	var sentinels, covered []cellPt
	for y := 0; y < c.Size.Y; y++ {
		for x := 0; x < c.Size.X; x++ {
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
	pxW := c.Size.X * cellW
	pxH := c.Size.Y * cellH
	if pxW <= 0 || pxH <= 0 {
		return
	}
	scale := pxW / c.PixelW
	if alt := pxH / c.PixelH; alt < scale {
		scale = alt
	}
	if scale < 1 {
		scale = 1
	}
	dcs := sixel.EncodeRealtime(c.pixels, scale)
	if dcs == "" {
		return
	}

	// 1) BG fill over uncovered cells.
	if fill := buildBGFill(sentinels, c.BG); fill != "" {
		_ = views.WriteRaw(fill)
	}
	// 2) SIXEL.
	move := "\x1b[" + strconv.Itoa(gy+1) + ";" + strconv.Itoa(gx+1) + "H"
	if err := views.WriteRaw(move + dcs); err != nil {
		return
	}
	// 3) Settle the cellbuf — see ImageView.PreFlush.
	bgCell := types.DrawCell{Ch: " ", BGRGB: nonzeroRGB(c.BG)}
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

// buildBGFill produces a single-row-coalesced SGR 48;2;r;g;b m + cursor
// moves + spaces string covering each cell in cells. Identical in
// shape to imageview.buildBGFill — duplicated to keep the dependency
// arrows pointing one way (sixelcanvas → views, not sixelcanvas →
// imageview).
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
	i := 0
	for i < len(cells) {
		runY := cells[i].y
		runStartX := cells[i].x
		j := i + 1
		for j < len(cells) && cells[j].y == runY && cells[j].x == cells[j-1].x+1 {
			j++
		}
		runLen := j - i
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

// --- helpers --------------------------------------------------------

func rgbaFrom(rgb uint32) color.RGBA {
	return color.RGBA{
		R: uint8((rgb >> 16) & 0xFF),
		G: uint8((rgb >> 8) & 0xFF),
		B: uint8(rgb & 0xFF),
		A: 255,
	}
}

func samplePixel(img *image.RGBA, x, y int) uint32 {
	b := img.Bounds()
	if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
		return 0
	}
	c := img.RGBAAt(x, y)
	return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
}

func nonzeroRGB(c uint32) uint32 {
	if c == 0 {
		return 0x010101
	}
	return c
}

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

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
