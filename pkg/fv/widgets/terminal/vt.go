// Package terminal provides Terminal — an embedded VT100/xterm-compatible
// terminal emulator view. Spawns a child process behind a PTY, streams
// its output through a small ANSI parser, and renders the resulting
// cell grid with normal FV widgets.
//
// Scope of the parser is "enough to run a typical interactive shell":
// CSI cursor moves and erase, SGR colors + bold/reverse, scroll
// regions, save/restore cursor, alt-screen toggle, OSC 0/1/2 (window
// title — currently captured into Title and propagated to a callback).
// More obscure sequences (DEC private graphics, character sets,
// double-width lines, …) are silently consumed and ignored, which is
// the right thing for a "good enough for bash + vim" target.
package terminal

import (
	"unicode/utf8"

	"github.com/oldwired/fv-go/pkg/fv/types"
)

// cell is one terminal-grid cell.
type cell struct {
	Ch         rune
	FG, BG     uint8 // CGA-style palette index (0..15)
	Bold       bool
	Underline  bool
	Reverse    bool
	HasDefault bool // true when FG/BG should fall through to the view's default
}

// blankCell returns a fresh space with default colors marked.
func blankCell() cell { return cell{Ch: ' ', HasDefault: true} }

// buffer is the screen state. The grid has H rows × W cols of cells.
//
// Indices are 0-based internally; ANSI clients see 1-based and the
// CSI handlers translate. Scroll region defaults to the full screen
// (top=0, bottom=H-1).
type buffer struct {
	W, H int

	cells [][]cell // [row][col]

	cursorR, cursorC int
	savedR, savedC   int
	savedFG, savedBG uint8
	savedBold        bool
	savedReverse     bool
	savedUL          bool

	scrollTop, scrollBot int

	fg, bg       uint8
	hasFG, hasBG bool
	bold         bool
	reverse      bool
	underline    bool

	autoWrap    bool
	wrapPending bool // cursor is "stuck" at last column waiting for next char to wrap
	insertMode  bool
	originMode  bool

	altActive    bool
	altCells     [][]cell
	altCR, altCC int

	cursorVisible bool
}

// newBuffer constructs a fresh w×h buffer with default attributes.
func newBuffer(w, h int) *buffer {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	b := &buffer{
		W: w, H: h,
		scrollTop:     0,
		scrollBot:     h - 1,
		autoWrap:      true,
		cursorVisible: true,
	}
	b.cells = makeGrid(w, h)
	b.altCells = makeGrid(w, h)
	return b
}

func makeGrid(w, h int) [][]cell {
	g := make([][]cell, h)
	for y := 0; y < h; y++ {
		g[y] = make([]cell, w)
		for x := 0; x < w; x++ {
			g[y][x] = blankCell()
		}
	}
	return g
}

// resize reshapes the grid to w×h, preserving cells where possible.
func (b *buffer) resize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if w == b.W && h == b.H {
		return
	}
	resize := func(grid [][]cell) [][]cell {
		out := makeGrid(w, h)
		for y := 0; y < h && y < len(grid); y++ {
			for x := 0; x < w && x < len(grid[y]); x++ {
				out[y][x] = grid[y][x]
			}
		}
		return out
	}
	b.cells = resize(b.cells)
	b.altCells = resize(b.altCells)
	b.W, b.H = w, h
	b.scrollTop = 0
	b.scrollBot = h - 1
	if b.cursorR >= h {
		b.cursorR = h - 1
	}
	if b.cursorC >= w {
		b.cursorC = w - 1
	}
	b.wrapPending = false
}

// makeAttr returns the current cell's attribute byte (TV-style: bg<<4 | fg).
// Reverse swaps fg/bg. Bold biases fg into the bright range. Underline
// is rendered via the cell's ExtAttrs in the FV draw layer.
func (b *buffer) makeAttr() (fg, bg uint8, hasFG, hasBG bool) {
	fg, bg = b.fg, b.bg
	hasFG, hasBG = b.hasFG, b.hasBG
	if b.reverse {
		fg, bg = bg, fg
		hasFG, hasBG = hasBG, hasFG
	}
	if b.bold && hasFG && fg < 8 {
		fg += 8
	}
	return
}

// putRune writes one rune at the cursor, handling wrap and scrolling.
// Control characters are NOT processed here — call writeBytes/feedByte
// for the parser path; this is the leaf that paints a glyph.
func (b *buffer) putRune(r rune) {
	if b.wrapPending && b.autoWrap {
		b.cursorC = 0
		b.indexDown()
		b.wrapPending = false
	}
	if b.cursorC >= b.W {
		b.cursorC = b.W - 1
	}
	if b.insertMode {
		// Shift cells right, dropping the rightmost.
		row := b.cells[b.cursorR]
		copy(row[b.cursorC+1:], row[b.cursorC:b.W-1])
	}
	fg, bg, hasFG, hasBG := b.makeAttr()
	b.cells[b.cursorR][b.cursorC] = cell{
		Ch:        r,
		FG:        fg,
		BG:        bg,
		Bold:      b.bold,
		Reverse:   b.reverse,
		Underline: b.underline,
		// HasDefault is true if neither FG nor BG was set by SGR.
		HasDefault: !hasFG && !hasBG,
	}
	b.cells[b.cursorR][b.cursorC].FG = fg
	b.cells[b.cursorR][b.cursorC].BG = bg
	if !hasFG {
		b.cells[b.cursorR][b.cursorC].FG = 0xFF
	}
	if !hasBG {
		b.cells[b.cursorR][b.cursorC].BG = 0xFF
	}

	if b.cursorC == b.W-1 {
		b.wrapPending = true
	} else {
		b.cursorC++
	}
}

// indexDown moves cursor down one row, scrolling the scroll region if
// the cursor is on the bottom margin. Linefeed semantics.
func (b *buffer) indexDown() {
	if b.cursorR == b.scrollBot {
		b.scrollUp(1)
		return
	}
	if b.cursorR < b.H-1 {
		b.cursorR++
	}
}

// reverseIndex moves cursor up one row, scrolling down if at the top
// margin. Used by ESC M (RI).
func (b *buffer) reverseIndex() {
	if b.cursorR == b.scrollTop {
		b.scrollDown(1)
		return
	}
	if b.cursorR > 0 {
		b.cursorR--
	}
}

// scrollUp shifts rows in the scroll region up by n, filling new
// bottom rows with blanks.
func (b *buffer) scrollUp(n int) {
	top, bot := b.scrollTop, b.scrollBot
	if n <= 0 || top >= bot {
		return
	}
	if n > bot-top+1 {
		n = bot - top + 1
	}
	for y := top; y <= bot-n; y++ {
		copy(b.cells[y], b.cells[y+n])
	}
	for y := bot - n + 1; y <= bot; y++ {
		row := b.cells[y]
		for x := range row {
			row[x] = blankCell()
		}
	}
}

// scrollDown shifts rows down by n.
func (b *buffer) scrollDown(n int) {
	top, bot := b.scrollTop, b.scrollBot
	if n <= 0 || top >= bot {
		return
	}
	if n > bot-top+1 {
		n = bot - top + 1
	}
	for y := bot; y >= top+n; y-- {
		copy(b.cells[y], b.cells[y-n])
	}
	for y := top; y < top+n; y++ {
		row := b.cells[y]
		for x := range row {
			row[x] = blankCell()
		}
	}
}

// moveTo positions cursor with bounds clamping. ANSI is 1-based; this
// takes 0-based.
func (b *buffer) moveTo(r, c int) {
	if r < 0 {
		r = 0
	}
	if c < 0 {
		c = 0
	}
	if r >= b.H {
		r = b.H - 1
	}
	if c >= b.W {
		c = b.W - 1
	}
	b.cursorR, b.cursorC = r, c
	b.wrapPending = false
}

// eraseInLine implements CSI K. mode: 0=cursor→EOL, 1=BOL→cursor, 2=full row.
func (b *buffer) eraseInLine(mode int) {
	row := b.cells[b.cursorR]
	switch mode {
	case 0:
		for x := b.cursorC; x < b.W; x++ {
			row[x] = blankCell()
		}
	case 1:
		for x := 0; x <= b.cursorC && x < b.W; x++ {
			row[x] = blankCell()
		}
	case 2:
		for x := 0; x < b.W; x++ {
			row[x] = blankCell()
		}
	}
}

// eraseInDisplay implements CSI J. mode: 0=cursor→end, 1=start→cursor, 2=full.
func (b *buffer) eraseInDisplay(mode int) {
	switch mode {
	case 0:
		b.eraseInLine(0)
		for y := b.cursorR + 1; y < b.H; y++ {
			for x := 0; x < b.W; x++ {
				b.cells[y][x] = blankCell()
			}
		}
	case 1:
		b.eraseInLine(1)
		for y := 0; y < b.cursorR; y++ {
			for x := 0; x < b.W; x++ {
				b.cells[y][x] = blankCell()
			}
		}
	case 2, 3:
		for y := 0; y < b.H; y++ {
			for x := 0; x < b.W; x++ {
				b.cells[y][x] = blankCell()
			}
		}
	}
}

// insertChars / deleteChars implement CSI @ / CSI P.
func (b *buffer) insertChars(n int) {
	if n < 1 {
		n = 1
	}
	row := b.cells[b.cursorR]
	if n > b.W-b.cursorC {
		n = b.W - b.cursorC
	}
	copy(row[b.cursorC+n:], row[b.cursorC:b.W-n])
	for x := b.cursorC; x < b.cursorC+n; x++ {
		row[x] = blankCell()
	}
}

func (b *buffer) deleteChars(n int) {
	if n < 1 {
		n = 1
	}
	row := b.cells[b.cursorR]
	if n > b.W-b.cursorC {
		n = b.W - b.cursorC
	}
	copy(row[b.cursorC:], row[b.cursorC+n:])
	for x := b.W - n; x < b.W; x++ {
		row[x] = blankCell()
	}
}

// insertLines / deleteLines implement CSI L / CSI M.
func (b *buffer) insertLines(n int) {
	if b.cursorR < b.scrollTop || b.cursorR > b.scrollBot {
		return
	}
	if n < 1 {
		n = 1
	}
	max := b.scrollBot - b.cursorR + 1
	if n > max {
		n = max
	}
	for y := b.scrollBot; y >= b.cursorR+n; y-- {
		copy(b.cells[y], b.cells[y-n])
	}
	for y := b.cursorR; y < b.cursorR+n; y++ {
		row := b.cells[y]
		for x := range row {
			row[x] = blankCell()
		}
	}
}

func (b *buffer) deleteLines(n int) {
	if b.cursorR < b.scrollTop || b.cursorR > b.scrollBot {
		return
	}
	if n < 1 {
		n = 1
	}
	max := b.scrollBot - b.cursorR + 1
	if n > max {
		n = max
	}
	for y := b.cursorR; y <= b.scrollBot-n; y++ {
		copy(b.cells[y], b.cells[y+n])
	}
	for y := b.scrollBot - n + 1; y <= b.scrollBot; y++ {
		row := b.cells[y]
		for x := range row {
			row[x] = blankCell()
		}
	}
}

// saveCursor / restoreCursor — DECSC / DECRC.
func (b *buffer) saveCursor() {
	b.savedR, b.savedC = b.cursorR, b.cursorC
	b.savedFG, b.savedBG = b.fg, b.bg
	b.savedBold, b.savedReverse, b.savedUL = b.bold, b.reverse, b.underline
}

func (b *buffer) restoreCursor() {
	b.cursorR, b.cursorC = b.savedR, b.savedC
	b.fg, b.bg = b.savedFG, b.savedBG
	b.bold, b.reverse, b.underline = b.savedBold, b.savedReverse, b.savedUL
	b.wrapPending = false
}

// setSGR applies one or more SGR parameters. Empty list = reset.
func (b *buffer) setSGR(params []int) {
	if len(params) == 0 {
		params = []int{0}
	}
	i := 0
	for i < len(params) {
		p := params[i]
		switch {
		case p == 0:
			b.fg, b.bg = 0, 0
			b.hasFG, b.hasBG = false, false
			b.bold, b.reverse, b.underline = false, false, false
		case p == 1:
			b.bold = true
		case p == 4:
			b.underline = true
		case p == 7:
			b.reverse = true
		case p == 22:
			b.bold = false
		case p == 24:
			b.underline = false
		case p == 27:
			b.reverse = false
		case p >= 30 && p <= 37:
			b.fg = ansiToCGA[p-30]
			b.hasFG = true
		case p == 39:
			b.fg, b.hasFG = 0, false
		case p >= 40 && p <= 47:
			b.bg = ansiToCGA[p-40]
			b.hasBG = true
		case p == 49:
			b.bg, b.hasBG = 0, false
		case p >= 90 && p <= 97:
			b.fg = ansiToCGA[p-90] + 8
			b.hasFG = true
		case p >= 100 && p <= 107:
			b.bg = ansiToCGA[p-100] + 8
			b.hasBG = true
		case p == 38 && i+1 < len(params):
			// 256-color or RGB: 38;5;N or 38;2;R;G;B. We don't carry
			// truecolor in the cell model yet, so fold to nearest 16.
			if params[i+1] == 5 && i+2 < len(params) {
				b.fg = palette256ToCGA(params[i+2])
				b.hasFG = true
				i += 2
			} else if params[i+1] == 2 && i+4 < len(params) {
				b.fg = rgbToCGA(byte(params[i+2]), byte(params[i+3]), byte(params[i+4]))
				b.hasFG = true
				i += 4
			} else {
				i++
			}
		case p == 48 && i+1 < len(params):
			if params[i+1] == 5 && i+2 < len(params) {
				b.bg = palette256ToCGA(params[i+2])
				b.hasBG = true
				i += 2
			} else if params[i+1] == 2 && i+4 < len(params) {
				b.bg = rgbToCGA(byte(params[i+2]), byte(params[i+3]), byte(params[i+4]))
				b.hasBG = true
				i += 4
			} else {
				i++
			}
		}
		i++
	}
}

// ansiToCGA maps an ANSI-order palette index (0..7) to the CGA-order
// index used by FV. The mapping is the same shape as cgaToANSI in
// term/sgr.go (it's an involution: red↔red wrt CGA but ANSI/CGA disagree
// on which integer means which color).
var ansiToCGA = [8]byte{0, 4, 2, 6, 1, 5, 3, 7}

// palette256ToCGA reduces a 256-color index to a 16-color CGA palette
// entry. The 8-bit space is: 0..7 standard, 8..15 bright, 16..231 the
// 6×6×6 cube, 232..255 grayscale ramp. We approximate by averaging.
func palette256ToCGA(idx int) byte {
	switch {
	case idx < 8:
		return ansiToCGA[idx]
	case idx < 16:
		return ansiToCGA[idx-8] + 8
	case idx >= 232:
		// Grayscale: idx 232 = near-black, 255 = near-white.
		v := idx - 232
		if v < 6 {
			return 0 // black
		}
		if v < 18 {
			return 8 // dark gray
		}
		return 7 // light gray
	default:
		// Cube
		c := idx - 16
		r := (c / 36) * 51
		g := ((c / 6) % 6) * 51
		bl := (c % 6) * 51
		return rgbToCGA(byte(r), byte(g), byte(bl))
	}
}

// rgbToCGA picks the closest CGA palette entry to an RGB triple.
// Crude — for terminal output it's mostly used by toolkits that
// already emit 16-color SGR; the 24-bit path is a compatibility hatch.
func rgbToCGA(r, g, bl byte) byte {
	bright := r >= 0xC0 || g >= 0xC0 || bl >= 0xC0
	rb := r >= 0x60
	gb := g >= 0x60
	bb := bl >= 0x60
	idx := byte(0)
	if rb {
		idx |= 4
	}
	if gb {
		idx |= 2
	}
	if bb {
		idx |= 1
	}
	if bright {
		idx |= 8
	}
	// idx is RGB-bit order; convert to CGA-order via the standard table.
	// Bits: bit2=R, bit1=G, bit0=B. We need CGA-order palette index.
	low := idx & 7
	high := idx & 8
	cga := byte(0)
	switch low {
	case 0:
		cga = 0 // black
	case 1:
		cga = 1 // blue
	case 2:
		cga = 2 // green
	case 3:
		cga = 3 // cyan
	case 4:
		cga = 4 // red
	case 5:
		cga = 5 // magenta
	case 6:
		cga = 6 // brown/yellow (could be 14)
	case 7:
		cga = 7 // light gray
	}
	return cga | high
}

// ---------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------

type parserState int

const (
	sGround parserState = iota
	sEscape
	sCSI
	sOSC
	sCharset // ESC ( <c> / ESC ) <c> — selects a character set, ignored
)

// parser is a small VT/ANSI state machine that consumes bytes and
// drives the buffer.
type parser struct {
	state parserState
	buf   *buffer

	csiPriv   byte
	csiParams []int
	csiCurr   int
	csiHasCur bool
	csiInter  byte // intermediate byte ('?', '!', ...)

	osc []byte

	// UTF-8 accumulator for the ground state.
	utfBuf [4]byte
	utfLen int

	// OnTitle fires when an OSC 0/1/2 sequence completes; used by the
	// view to surface the running program's title.
	OnTitle func(string)
}

func newParser(b *buffer) *parser {
	return &parser{state: sGround, buf: b}
}

func (p *parser) Feed(data []byte) {
	for _, c := range data {
		p.feedByte(c)
	}
}

func (p *parser) feedByte(c byte) {
	switch p.state {
	case sGround:
		p.feedGround(c)
	case sEscape:
		p.feedEscape(c)
	case sCSI:
		p.feedCSI(c)
	case sOSC:
		p.feedOSC(c)
	case sCharset:
		p.state = sGround // consume one byte, ignore
	}
}

func (p *parser) feedGround(c byte) {
	switch {
	case c == 0x1B:
		p.state = sEscape
		p.utfLen = 0
	case c == 0x07: // BEL — ignored
	case c == 0x08: // BS
		if p.buf.cursorC > 0 {
			p.buf.cursorC--
			p.buf.wrapPending = false
		}
	case c == 0x09: // HT
		next := ((p.buf.cursorC / 8) + 1) * 8
		if next >= p.buf.W {
			next = p.buf.W - 1
		}
		p.buf.cursorC = next
		p.buf.wrapPending = false
	case c == 0x0A, c == 0x0B, c == 0x0C: // LF/VT/FF
		p.buf.indexDown()
	case c == 0x0D: // CR
		p.buf.cursorC = 0
		p.buf.wrapPending = false
	case c < 0x20 || c == 0x7F:
		// Other C0 / DEL — silently drop.
	case c < 0x80:
		p.buf.putRune(rune(c))
	default:
		// UTF-8 lead/continuation.
		p.utfBuf[p.utfLen] = c
		p.utfLen++
		if r, size := utf8.DecodeRune(p.utfBuf[:p.utfLen]); size == p.utfLen && r != utf8.RuneError {
			p.buf.putRune(r)
			p.utfLen = 0
		} else if p.utfLen >= 4 {
			// Bad sequence — emit replacement and reset.
			p.buf.putRune(utf8.RuneError)
			p.utfLen = 0
		}
	}
}

func (p *parser) feedEscape(c byte) {
	switch c {
	case '[':
		p.state = sCSI
		p.csiPriv = 0
		p.csiInter = 0
		p.csiParams = p.csiParams[:0]
		p.csiCurr = 0
		p.csiHasCur = false
	case ']':
		p.state = sOSC
		p.osc = p.osc[:0]
	case '(', ')':
		p.state = sCharset
	case '7': // DECSC
		p.buf.saveCursor()
		p.state = sGround
	case '8': // DECRC
		p.buf.restoreCursor()
		p.state = sGround
	case 'D': // IND
		p.buf.indexDown()
		p.state = sGround
	case 'E': // NEL — CR + LF
		p.buf.cursorC = 0
		p.buf.indexDown()
		p.state = sGround
	case 'M': // RI
		p.buf.reverseIndex()
		p.state = sGround
	case 'c': // RIS — full reset
		p.buf.resetTerminal()
		p.state = sGround
	case '=', '>': // keypad mode — ignored
		p.state = sGround
	default:
		// Unknown / unimplemented — drop.
		p.state = sGround
	}
}

func (p *parser) feedCSI(c byte) {
	if c >= '0' && c <= '9' {
		p.csiCurr = p.csiCurr*10 + int(c-'0')
		p.csiHasCur = true
		return
	}
	if c == ';' {
		if p.csiHasCur {
			p.csiParams = append(p.csiParams, p.csiCurr)
		} else {
			p.csiParams = append(p.csiParams, 0)
		}
		p.csiCurr = 0
		p.csiHasCur = false
		return
	}
	if c == '?' || c == '>' || c == '<' || c == '=' {
		p.csiPriv = c
		return
	}
	if c >= 0x20 && c <= 0x2F {
		p.csiInter = c
		return
	}
	// final byte
	if p.csiHasCur || len(p.csiParams) > 0 {
		if p.csiHasCur {
			p.csiParams = append(p.csiParams, p.csiCurr)
		}
	}
	p.dispatchCSI(c)
	p.state = sGround
}

func (p *parser) dispatchCSI(final byte) {
	get := func(i, def int) int {
		if i < len(p.csiParams) && p.csiParams[i] != 0 {
			return p.csiParams[i]
		}
		if i < len(p.csiParams) {
			return def
		}
		return def
	}
	if p.csiPriv == '?' {
		// DEC private modes: l = reset, h = set.
		set := final == 'h'
		reset := final == 'l'
		if !set && !reset {
			return
		}
		for _, m := range p.csiParams {
			p.applyDECMode(m, set)
		}
		return
	}
	switch final {
	case 'A':
		n := get(0, 1)
		p.buf.cursorR -= n
		if p.buf.cursorR < 0 {
			p.buf.cursorR = 0
		}
		p.buf.wrapPending = false
	case 'B':
		n := get(0, 1)
		p.buf.cursorR += n
		if p.buf.cursorR >= p.buf.H {
			p.buf.cursorR = p.buf.H - 1
		}
		p.buf.wrapPending = false
	case 'C':
		n := get(0, 1)
		p.buf.cursorC += n
		if p.buf.cursorC >= p.buf.W {
			p.buf.cursorC = p.buf.W - 1
		}
		p.buf.wrapPending = false
	case 'D':
		n := get(0, 1)
		p.buf.cursorC -= n
		if p.buf.cursorC < 0 {
			p.buf.cursorC = 0
		}
		p.buf.wrapPending = false
	case 'E':
		n := get(0, 1)
		p.buf.cursorR += n
		if p.buf.cursorR >= p.buf.H {
			p.buf.cursorR = p.buf.H - 1
		}
		p.buf.cursorC = 0
		p.buf.wrapPending = false
	case 'F':
		n := get(0, 1)
		p.buf.cursorR -= n
		if p.buf.cursorR < 0 {
			p.buf.cursorR = 0
		}
		p.buf.cursorC = 0
		p.buf.wrapPending = false
	case 'G':
		c := get(0, 1)
		p.buf.cursorC = c - 1
		if p.buf.cursorC < 0 {
			p.buf.cursorC = 0
		}
		if p.buf.cursorC >= p.buf.W {
			p.buf.cursorC = p.buf.W - 1
		}
		p.buf.wrapPending = false
	case 'H', 'f':
		r := get(0, 1)
		c := get(1, 1)
		p.buf.moveTo(r-1, c-1)
	case 'd':
		r := get(0, 1)
		p.buf.cursorR = r - 1
		if p.buf.cursorR < 0 {
			p.buf.cursorR = 0
		}
		if p.buf.cursorR >= p.buf.H {
			p.buf.cursorR = p.buf.H - 1
		}
		p.buf.wrapPending = false
	case 'J':
		p.buf.eraseInDisplay(get(0, 0))
	case 'K':
		p.buf.eraseInLine(get(0, 0))
	case 'L':
		p.buf.insertLines(get(0, 1))
	case 'M':
		p.buf.deleteLines(get(0, 1))
	case 'P':
		p.buf.deleteChars(get(0, 1))
	case '@':
		p.buf.insertChars(get(0, 1))
	case 'X':
		// ECH — erase chars, no shift.
		n := get(0, 1)
		row := p.buf.cells[p.buf.cursorR]
		for x := p.buf.cursorC; x < p.buf.cursorC+n && x < p.buf.W; x++ {
			row[x] = blankCell()
		}
	case 'r':
		top := get(0, 1) - 1
		bot := get(1, p.buf.H) - 1
		if top < 0 {
			top = 0
		}
		if bot >= p.buf.H {
			bot = p.buf.H - 1
		}
		if top < bot {
			p.buf.scrollTop = top
			p.buf.scrollBot = bot
			p.buf.moveTo(0, 0)
		}
	case 'S':
		p.buf.scrollUp(get(0, 1))
	case 'T':
		p.buf.scrollDown(get(0, 1))
	case 'm':
		p.buf.setSGR(p.csiParams)
	case 's':
		p.buf.saveCursor()
	case 'u':
		p.buf.restoreCursor()
	case 'h', 'l':
		// Public modes (no '?'): IRM (4) and a few others.
		set := final == 'h'
		for _, m := range p.csiParams {
			if m == 4 {
				p.buf.insertMode = set
			}
		}
	}
}

func (p *parser) applyDECMode(m int, set bool) {
	switch m {
	case 1: // DECCKM cursor key application — we just track it via the view's keyboard mapping
	case 6: // DECOM origin mode
		p.buf.originMode = set
	case 7: // DECAWM autowrap
		p.buf.autoWrap = set
	case 25: // DECTCEM cursor visibility
		p.buf.cursorVisible = set
	case 47, 1047, 1049: // alt screen
		if set && !p.buf.altActive {
			p.buf.altActive = true
			p.buf.cells, p.buf.altCells = p.buf.altCells, p.buf.cells
			p.buf.altCR, p.buf.altCC = p.buf.cursorR, p.buf.cursorC
			if m == 1049 {
				p.buf.saveCursor()
				p.buf.eraseInDisplay(2)
				p.buf.moveTo(0, 0)
			}
		} else if !set && p.buf.altActive {
			p.buf.altActive = false
			p.buf.cells, p.buf.altCells = p.buf.altCells, p.buf.cells
			if m == 1049 {
				p.buf.restoreCursor()
			} else {
				p.buf.cursorR, p.buf.cursorC = p.buf.altCR, p.buf.altCC
			}
		}
	}
}

func (p *parser) feedOSC(c byte) {
	if c == 0x07 || c == 0x9C {
		p.completeOSC()
		p.state = sGround
		return
	}
	if c == 0x1B {
		// ST = ESC \ — wait for the trailing '\'.
		p.osc = append(p.osc, c)
		return
	}
	if len(p.osc) > 0 && p.osc[len(p.osc)-1] == 0x1B && c == '\\' {
		p.osc = p.osc[:len(p.osc)-1]
		p.completeOSC()
		p.state = sGround
		return
	}
	if len(p.osc) < 4096 {
		p.osc = append(p.osc, c)
	}
}

func (p *parser) completeOSC() {
	// Format: <Ps>;<Pt>. We mostly care about 0/1/2 (window title).
	s := string(p.osc)
	semi := -1
	for i := 0; i < len(s); i++ {
		if s[i] == ';' {
			semi = i
			break
		}
	}
	if semi < 0 {
		return
	}
	ps := s[:semi]
	pt := s[semi+1:]
	switch ps {
	case "0", "1", "2":
		if p.OnTitle != nil {
			p.OnTitle(pt)
		}
	}
}

// resetTerminal — RIS, full reset.
func (b *buffer) resetTerminal() {
	for y := 0; y < b.H; y++ {
		for x := 0; x < b.W; x++ {
			b.cells[y][x] = blankCell()
		}
	}
	b.cursorR, b.cursorC = 0, 0
	b.savedR, b.savedC = 0, 0
	b.scrollTop, b.scrollBot = 0, b.H-1
	b.fg, b.bg = 0, 0
	b.hasFG, b.hasBG = false, false
	b.bold, b.reverse, b.underline = false, false, false
	b.autoWrap = true
	b.wrapPending = false
	b.altActive = false
	b.cursorVisible = true
}

// CellAt reads cell (col, row); returns the blank cell for OOB.
func (b *buffer) CellAt(c, r int) cell {
	if r < 0 || r >= b.H || c < 0 || c >= b.W {
		return blankCell()
	}
	return b.cells[r][c]
}

// toDrawCell maps a parser cell to an FV DrawCell. Default colors fall
// through (palette 0x07 on 0x00) until the caller's view overrides.
func (c cell) toDrawCell(defaultFG, defaultBG byte) types.DrawCell {
	fg, bg := defaultFG, defaultBG
	if c.FG != 0xFF {
		fg = c.FG
	}
	if c.BG != 0xFF {
		bg = c.BG
	}
	var ext byte
	if c.Underline {
		ext |= 1 << types.EAUnderShift // UnderSingle = 1
	}
	ch := string(c.Ch)
	if c.Ch == 0 {
		ch = " "
	}
	return types.DrawCell{
		Ch:       ch,
		Attr:     types.MakeAttr(fg, bg),
		ExtAttrs: ext,
	}
}
