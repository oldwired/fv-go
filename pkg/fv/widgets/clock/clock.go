// Package clock provides a digital + analog clock widget. Inspired by
// Pascal Free Vision's TClockView from gadgets.pas but extended with
// an analog face and timezone awareness.
//
// Construct with NewDigital(bounds) for HH:MM:SS text, or NewAnalog
// (bounds) for a round face with hour/minute/second hands. Field
// defaults are zero-value-sane so a bare construct gives a useful
// clock; tweak fields after construction to customize.
//
// Animation runs via pkg/fv/anim; the widget registers itself on a
// 1 s tick (or 100 ms when an analog clock has SmoothSweep enabled).
// Unregistering happens automatically once the widget's host group
// drops it — anim.Pulse prunes detached tickers via the Liveness
// interface that views.Base satisfies.
package clock

import (
	"math"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Mode selects digital vs analog rendering.
type Mode int

const (
	Digital Mode = iota
	Analog
)

// NumeralStyle controls what's drawn on the analog rim.
type NumeralStyle int

const (
	NumeralsCardinal NumeralStyle = iota // 12 / 3 / 6 / 9 only
	NumeralsNone                         // bare face
	NumeralsAll                          // 1..12 at each hour
	NumeralsTicks                        // dots at each hour, no labels
)

// HandStyle controls the analog hand rendering.
type HandStyle int

const (
	HandsLine  HandStyle = iota // Unicode line-drawing chars: ─│╱╲
	HandsBlock                  // Solid block chars for a chunkier look
)

// Clock is a digital or analog clock widget.
type Clock struct {
	views.Base

	Mode     Mode
	Location *time.Location
	Interval time.Duration
	Color    uint16

	// Digital-only options.
	Format       string // Go time-format string; zero value = "15:04:05"
	ShowDate     bool   // render date on the row above the time
	BlinkColon   bool   // 1 Hz blink on `:` separators
	UseLEDDigits bool   // chunky 3-row digit shapes (height ≥ 3 required)

	// Analog-only options.
	SmoothSweep    bool         // second hand at 100 ms instead of 1 s
	ShowSecondHand bool         // zero value chosen below — true by default
	Numerals       NumeralStyle // see NumeralStyle values
	Hands          HandStyle

	// Per-hand colors (analog only). Zero value = palette default.
	HourColor   uint16
	MinuteColor uint16
	SecondColor uint16
}

// NewDigital constructs a 24-hour digital clock in local time.
func NewDigital(bounds geom.Rect) *Clock {
	pal := theme.Get()
	c := &Clock{
		Base:     views.NewBase(bounds),
		Mode:     Digital,
		Location: time.Local,
		Interval: time.Second,
		Color:    pal.ClockFace,
		Format:   "15:04:05",
	}
	c.SetSelf(c)
	anim.Register(c, c.Interval)
	return c
}

// NewAnalog constructs an analog clock with cardinal numerals and a
// ticking second hand.
func NewAnalog(bounds geom.Rect) *Clock {
	pal := theme.Get()
	c := &Clock{
		Base:           views.NewBase(bounds),
		Mode:           Analog,
		Location:       time.Local,
		Interval:       time.Second,
		Color:          pal.ClockFace,
		ShowSecondHand: true,
		Numerals:       NumeralsCardinal,
		Hands:          HandsLine,
		HourColor:      pal.ClockHourH,
		MinuteColor:    pal.ClockMinH,
		SecondColor:    pal.ClockSecH,
	}
	c.SetSelf(c)
	anim.Register(c, c.Interval)
	return c
}

// GetTypeID for serial registry.
func (c *Clock) GetTypeID() string { return "clock" }

// SetSmoothSweep toggles the analog second hand between ticking (1 s)
// and sweeping (100 ms). Re-registers with the anim package so the
// new interval takes effect immediately.
func (c *Clock) SetSmoothSweep(on bool) {
	c.SmoothSweep = on
	if on {
		c.Interval = 100 * time.Millisecond
	} else {
		c.Interval = time.Second
	}
	anim.Register(c, c.Interval)
}

// Tick is the anim.Ticker entry point. The clock always wants a
// redraw — the wall time changed. Liveness pruning is handled by
// the Alive() method inherited from views.Base.
func (c *Clock) Tick(now time.Time) bool { return true }

// Draw dispatches to the appropriate renderer.
func (c *Clock) Draw() {
	loc := c.Location
	if loc == nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	if c.Mode == Analog {
		c.drawAnalog(now)
	} else {
		c.drawDigital(now)
	}
}

// --------------------------------------------------------------------
// Digital rendering
// --------------------------------------------------------------------

func (c *Clock) drawDigital(now time.Time) {
	format := c.Format
	if format == "" {
		format = "15:04:05"
	}
	text := now.Format(format)
	if c.BlinkColon && now.Second()%2 == 1 {
		text = blankColons(text)
	}

	// Clear the whole view.
	for y := 0; y < c.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(c.Size.X)
		for x := 0; x < c.Size.X; x++ {
			screen.DrawCell(buf, x, " ", c.Color)
		}
		c.WriteLine(0, y, c.Size.X, 1, buf)
	}

	if c.UseLEDDigits && c.Size.Y >= 3 {
		c.drawLEDDigits(text)
		if c.ShowDate {
			c.drawCenteredText(0, now.Format("Mon Jan 2"))
		}
		return
	}

	var timeRow int
	if c.ShowDate && c.Size.Y >= 2 {
		c.drawCenteredText(0, now.Format("Mon Jan 2"))
		timeRow = 1
	} else {
		timeRow = c.Size.Y / 2
	}
	c.drawCenteredText(timeRow, text)
}

// blankColons replaces every ':' in s with a space, matching the
// 1 Hz blink-off frame of a classic digital clock.
func blankColons(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r == ':' {
			out[i] = ' '
		}
	}
	return string(out)
}

func (c *Clock) drawCenteredText(row int, s string) {
	if row < 0 || row >= c.Size.Y {
		return
	}
	buf := screen.MakeDrawBuffer(c.Size.X)
	for x := 0; x < c.Size.X; x++ {
		screen.DrawCell(buf, x, " ", c.Color)
	}
	start := (c.Size.X - runeLen(s)) / 2
	if start < 0 {
		start = 0
	}
	screen.DrawStr(buf, start, s, c.Color)
	c.WriteLine(0, row, c.Size.X, 1, buf)
}

// LED-digit shapes: 5-cells-wide × 3-rows-tall per digit, two-cell
// gap between groups, single-cell colon. Trades realism for chunky
// readability — the Pascal LEDDigits widget renders something very
// similar.
var ledGlyphs = map[rune][3]string{
	'0': {"███", "█ █", "███"},
	'1': {"  █", "  █", "  █"},
	'2': {"███", "  █", "█  "}, // approximation
	'3': {"███", " ██", "███"},
	'4': {"█ █", "███", "  █"},
	'5': {"███", "█  ", "███"}, // approximation
	'6': {"█  ", "███", "███"},
	'7': {"███", "  █", "  █"},
	'8': {"███", "███", "███"},
	'9': {"███", "███", "  █"},
	':': {" ", "█", " "},
	' ': {" ", " ", " "},
}

func (c *Clock) drawLEDDigits(text string) {
	// Compute total width: each glyph is its rune-width in cells + 1 gap.
	totalW := 0
	for _, r := range text {
		g, ok := ledGlyphs[r]
		if !ok {
			g = ledGlyphs[' ']
		}
		totalW += runeLen(g[0]) + 1
	}
	if totalW > 0 {
		totalW--
	}
	startX := (c.Size.X - totalW) / 2
	if startX < 0 {
		startX = 0
	}
	startY := (c.Size.Y - 3) / 2

	rows := [3]screen.DrawBuffer{
		screen.MakeDrawBuffer(c.Size.X),
		screen.MakeDrawBuffer(c.Size.X),
		screen.MakeDrawBuffer(c.Size.X),
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < c.Size.X; x++ {
			screen.DrawCell(rows[y], x, " ", c.Color)
		}
	}
	x := startX
	for _, r := range text {
		g, ok := ledGlyphs[r]
		if !ok {
			g = ledGlyphs[' ']
		}
		w := runeLen(g[0])
		for y := 0; y < 3; y++ {
			screen.DrawStr(rows[y], x, g[y], c.Color)
		}
		x += w + 1
	}
	for y := 0; y < 3; y++ {
		yy := startY + y
		if yy < 0 || yy >= c.Size.Y {
			continue
		}
		c.WriteLine(0, yy, c.Size.X, 1, rows[y])
	}
}

// --------------------------------------------------------------------
// Analog rendering
// --------------------------------------------------------------------

// drawAnalog renders the clock face into an internal char grid and
// then flushes one row at a time. The internal grid lets us paint
// the face, ticks/numerals, and hands in any order without worrying
// about overdraw — the hands always end up on top.
func (c *Clock) drawAnalog(now time.Time) {
	w, h := c.Size.X, c.Size.Y
	if w < 5 || h < 3 {
		// Not enough room for a face. Render compact digital text instead.
		c.drawDigital(now)
		return
	}

	// Build a per-cell character + color grid we can paint over.
	grid := make([][]cell, h)
	for y := range grid {
		grid[y] = make([]cell, w)
		for x := range grid[y] {
			grid[y][x] = cell{ch: " ", attr: c.Color}
		}
	}

	// Aspect-ratio-corrected ellipse: pick the largest vertical radius
	// that fits, then horizontal radius = 2× to compensate for cells
	// being roughly twice as tall as wide. Bounds clamp prevents the
	// face from overflowing narrow widgets.
	cx, cy := float64(w-1)/2, float64(h-1)/2
	ry := math.Min(float64(h-1)/2, float64(w-1)/4)
	rx := ry * 2
	if rx > float64(w-1)/2 {
		rx = float64(w-1) / 2
		ry = rx / 2
	}

	// Face rim. Iterate angles in steps small enough to land on every
	// rim cell at least once (rim circumference ~ 2π * max(rx, ry)).
	steps := int(2*math.Pi*math.Max(rx, ry)*2) + 1
	for i := 0; i < steps; i++ {
		theta := 2 * math.Pi * float64(i) / float64(steps)
		x := int(math.Round(cx + rx*math.Cos(theta)))
		y := int(math.Round(cy + ry*math.Sin(theta)))
		if y < 0 || y >= h || x < 0 || x >= w {
			continue
		}
		grid[y][x] = cell{ch: "·", attr: c.Color}
	}

	// Hour ticks / numerals. Placed slightly inside the rim so they
	// don't clash with face dots.
	drawHour := func(hour int, label string, ch string) {
		theta := hourAngle(hour)
		x := int(math.Round(cx + (rx-1)*math.Cos(theta)))
		y := int(math.Round(cy + (ry-1)*math.Sin(theta)))
		if y < 0 || y >= h || x < 0 || x >= w {
			return
		}
		if label != "" {
			// Center the label horizontally on (x, y). Two-digit hours
			// need an extra cell to the left if there's room.
			for i, r := range label {
				xx := x - (len(label)-1)/2 + i
				if xx < 0 || xx >= w {
					continue
				}
				grid[y][xx] = cell{ch: string(r), attr: c.Color}
			}
			return
		}
		grid[y][x] = cell{ch: ch, attr: c.Color}
	}
	switch c.Numerals {
	case NumeralsCardinal:
		drawHour(12, "12", "")
		drawHour(3, "3", "")
		drawHour(6, "6", "")
		drawHour(9, "9", "")
	case NumeralsAll:
		for h := 1; h <= 12; h++ {
			drawHour(h, itoa(h), "")
		}
	case NumeralsTicks:
		for h := 1; h <= 12; h++ {
			drawHour(h, "", "•")
		}
	case NumeralsNone:
		// nothing
	}

	// Hands. Order matters: hour, minute, second so the second hand is
	// the most visible. Lengths as fractions of the radius.
	hour := now.Hour() % 12
	minute := now.Minute()
	second := now.Second()
	nano := now.Nanosecond()

	hourAng := 2*math.Pi*(float64(hour)/12+float64(minute)/720) - math.Pi/2
	minAng := 2*math.Pi*(float64(minute)/60+float64(second)/3600) - math.Pi/2
	secAng := 2*math.Pi*float64(second)/60 - math.Pi/2
	if c.SmoothSweep {
		secAng = 2*math.Pi*(float64(second)+float64(nano)/1e9)/60 - math.Pi/2
	}

	c.drawHand(grid, cx, cy, rx*0.5, ry*0.5, hourAng, c.HourColor)
	c.drawHand(grid, cx, cy, rx*0.8, ry*0.8, minAng, c.MinuteColor)
	if c.ShowSecondHand {
		c.drawHand(grid, cx, cy, rx*0.9, ry*0.9, secAng, c.SecondColor)
	}

	// Center dot — always last so the hands' line chars don't overwrite it.
	if ccx, ccy := int(math.Round(cx)), int(math.Round(cy)); ccy >= 0 && ccy < h && ccx >= 0 && ccx < w {
		grid[ccy][ccx] = cell{ch: "●", attr: c.Color}
	}

	// Flush grid to the view.
	for y := 0; y < h; y++ {
		buf := screen.MakeDrawBuffer(w)
		for x := 0; x < w; x++ {
			screen.DrawCell(buf, x, grid[y][x].ch, grid[y][x].attr)
		}
		c.WriteLine(0, y, w, 1, buf)
	}
}

// drawHand draws a line from the face center to a point at (rx, ry)
// distance at angle θ. Cells along the way get a glyph picked from the
// hand-style table. Bresenham-ish stepping in continuous space.
func (c *Clock) drawHand(grid [][]cell, cx, cy, rx, ry, theta float64, attr uint16) {
	if attr == 0 {
		attr = c.Color
	}
	endX := cx + rx*math.Cos(theta)
	endY := cy + ry*math.Sin(theta)
	dx := endX - cx
	dy := endY - cy
	steps := int(math.Max(math.Abs(dx), math.Abs(dy))) * 2
	if steps < 1 {
		return
	}

	// Pick the glyph based on the line's dominant direction in cell
	// space. The factor of 2 on dy compensates for cells being roughly
	// 2× taller than wide — without it, the chosen glyph rotates faster
	// than the line itself appears to move on screen.
	angle := math.Atan2(dy*2, dx)
	glyph := handGlyph(c.Hands, angle)

	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(cx + t*dx))
		y := int(math.Round(cy + t*dy))
		if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[0]) {
			continue
		}
		grid[y][x] = cell{ch: glyph, attr: attr}
	}
}

// handGlyph picks a Unicode character that visually points in the
// direction of angle (radians, atan2 convention: 0 = east, π/2 = south).
func handGlyph(style HandStyle, angle float64) string {
	if style == HandsBlock {
		return "█"
	}
	// Snap angle to 8-way compass — vertical / horizontal / diagonal.
	// Cell aspect: vertical neighbors are visually farther apart than
	// horizontal ones, but handGlyph receives an already-corrected
	// angle from drawHand, so we treat the input as screen-space.
	a := math.Mod(angle+2*math.Pi, math.Pi)
	switch {
	case a < math.Pi/8 || a >= 7*math.Pi/8:
		return "─"
	case a < 3*math.Pi/8:
		return "╲"
	case a < 5*math.Pi/8:
		return "│"
	default:
		return "╱"
	}
}

// hourAngle returns the position of hour H on a clock face in radians,
// measured from the positive x-axis (east), with 12 at the top.
func hourAngle(h int) float64 {
	return 2*math.Pi*float64(h)/12 - math.Pi/2
}

type cell struct {
	ch   string
	attr uint16
}

// --------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
