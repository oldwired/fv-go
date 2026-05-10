package term

import (
	"github.com/oldwired/fv-go/pkg/fv/types"
)

// cellBuf is a 2D grid of DrawCell ringed by a previous-frame buffer
// for diffed flushes. Its sole responsibility is "what changed since
// last frame, in row-major order, with run-length grouping by attribute".
type cellBuf struct {
	cols, rows int
	cur        []types.DrawCell // back buffer (writes go here)
	prev       []types.DrawCell // last flushed frame
}

func newCellBuf(cols, rows int) *cellBuf {
	b := &cellBuf{}
	b.Resize(cols, rows)
	return b
}

// Resize discards the current contents and reallocates. Call when the
// terminal viewport changes.
func (b *cellBuf) Resize(cols, rows int) {
	b.cols = cols
	b.rows = rows
	n := cols * rows
	b.cur = make([]types.DrawCell, n)
	b.prev = make([]types.DrawCell, n)
	blank := types.DrawCell{Ch: " ", Attr: types.MakeAttr(7, 0)}
	for i := range b.cur {
		b.cur[i] = blank
		// prev intentionally left zero-valued so the first flush
		// rewrites every cell.
	}
}

// Set writes one cell. Out-of-range coords are dropped.
func (b *cellBuf) Set(x, y int, c types.DrawCell) {
	if x < 0 || y < 0 || x >= b.cols || y >= b.rows {
		return
	}
	if c.Ch == "" {
		c.Ch = " "
	}
	b.cur[y*b.cols+x] = c
}

// Get reads one cell. Returns zero cell if OOB.
func (b *cellBuf) Get(x, y int) types.DrawCell {
	if x < 0 || y < 0 || x >= b.cols || y >= b.rows {
		return types.DrawCell{}
	}
	return b.cur[y*b.cols+x]
}

// Clear fills with empty cells using attr.
func (b *cellBuf) Clear(attr uint16) {
	cell := types.DrawCell{Ch: " ", Attr: attr}
	for i := range b.cur {
		b.cur[i] = cell
	}
}

// span describes a contiguous run of cells with identical attributes,
// suitable for emitting as one chunk of SGR + text.
type span struct {
	x, y int
	attr uint16
	fg   uint32
	bg   uint32
	ext  byte
	text string
}

// dirty walks the diff between cur and prev row-by-row and yields runs
// of changed cells with matching attributes. After diff, the caller is
// responsible for copying cur->prev (DiffSwap).
func (b *cellBuf) dirty() []span {
	var spans []span
	for y := 0; y < b.rows; y++ {
		x := 0
		for x < b.cols {
			i := y*b.cols + x
			if b.cur[i] == b.prev[i] {
				x++
				continue
			}
			start := x
			cur := b.cur[i]
			text := cur.Ch
			x++
			for x < b.cols {
				j := y*b.cols + x
				if b.cur[j] == b.prev[j] {
					break
				}
				next := b.cur[j]
				if next.Attr != cur.Attr || next.FGRGB != cur.FGRGB ||
					next.BGRGB != cur.BGRGB || next.ExtAttrs != cur.ExtAttrs {
					break
				}
				text += next.Ch
				x++
			}
			spans = append(spans, span{
				x:    start,
				y:    y,
				attr: cur.Attr,
				fg:   cur.FGRGB,
				bg:   cur.BGRGB,
				ext:  cur.ExtAttrs,
				text: text,
			})
		}
	}
	return spans
}

// commit copies the back buffer over the front buffer.
func (b *cellBuf) commit() {
	copy(b.prev, b.cur)
}
