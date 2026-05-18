package term

import (
	"sync"

	"github.com/oldwired/fv-go/pkg/fv/types"
)

// cellBuf is a 2D grid of DrawCell ringed by a previous-frame buffer
// for diffed flushes. Its sole responsibility is "what changed since
// last frame, in row-major order, with run-length grouping by attribute".
//
// Synchronization: mu protects every field. The POSIX backend's
// signalLoop calls Resize from a separate goroutine when SIGWINCH
// fires; the Windows backend's reader goroutine does the same when
// it polls the viewport. Without the mutex those resizes raced with
// the UI goroutine's SetCell / Clear / dirty / commit / Get and
// could produce torn reads, out-of-range index panics during slice
// reallocation, or stale cell counts visible to the diff pass.
type cellBuf struct {
	mu         sync.Mutex
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
	b.mu.Lock()
	defer b.mu.Unlock()
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
//
// An empty Ch is preserved verbatim — it marks the continuation cell
// of a wide (2-column) glyph whose leading half lives at column x-1.
// The previous behavior rewrote it to a literal space, which broke
// the diff: when a row containing wide chars scrolled away, the
// "space" in prev matched the "space" in cur for every continuation
// column, so the diff skipped them and the terminal's actual wide
// glyph stayed on screen. Snapshot still renders "" as " " for
// human-readable golden output (see Headless.Snapshot).
func (b *cellBuf) Set(x, y int, c types.DrawCell) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if x < 0 || y < 0 || x >= b.cols || y >= b.rows {
		return
	}
	b.cur[y*b.cols+x] = c
}

// Get reads one cell. Returns zero cell if OOB.
func (b *cellBuf) Get(x, y int) types.DrawCell {
	b.mu.Lock()
	defer b.mu.Unlock()
	if x < 0 || y < 0 || x >= b.cols || y >= b.rows {
		return types.DrawCell{}
	}
	return b.cur[y*b.cols+x]
}

// Clear fills with empty cells using attr.
func (b *cellBuf) Clear(attr uint16) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cell := types.DrawCell{Ch: " ", Attr: attr}
	for i := range b.cur {
		b.cur[i] = cell
	}
}

// Size returns the current cols/rows under the mutex. External
// callers should use this instead of reading b.cols / b.rows
// directly — those fields may be re-assigned by a concurrent
// Resize from a signal goroutine.
func (b *cellBuf) Size() (cols, rows int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cols, b.rows
}

// span describes a contiguous run of cells with identical attributes,
// suitable for emitting as one chunk of SGR + text. url, when non-
// empty, wraps the span in OSC 8 ; ; URL …  OSC 8 ; ; — terminals
// that honor it render the span clickable (iTerm2, WezTerm, recent
// gnome-terminal, …); others ignore the OSC.
type span struct {
	x, y int
	attr uint16
	fg   uint32
	bg   uint32
	ext  byte
	url  string
	text string
}

// dirty walks the diff between cur and prev row-by-row and yields runs
// of changed cells with matching attributes. After diff, the caller is
// responsible for copying cur->prev (DiffSwap).
func (b *cellBuf) dirty() []span {
	b.mu.Lock()
	defer b.mu.Unlock()
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
					next.BGRGB != cur.BGRGB || next.ExtAttrs != cur.ExtAttrs ||
					next.HyperlinkURL != cur.HyperlinkURL {
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
				url:  cur.HyperlinkURL,
				text: text,
			})
		}
	}
	return spans
}

// commit copies the back buffer over the front buffer.
func (b *cellBuf) commit() {
	b.mu.Lock()
	defer b.mu.Unlock()
	copy(b.prev, b.cur)
}

// markClean sets prev[x,y] = cur[x,y] so the cell appears unchanged in
// the next dirty() pass. Used by SIXEL views to suppress emission of
// their sentinel cells (which would otherwise overwrite the graphics
// pixels with spaces).
func (b *cellBuf) markClean(x, y int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if x < 0 || y < 0 || x >= b.cols || y >= b.rows {
		return
	}
	b.prev[y*b.cols+x] = b.cur[y*b.cols+x]
}

// invalidate zeroes prev[x,y] so the cell forces a re-emit on the next
// dirty() pass even if its content is identical to last frame. Used by
// SIXEL views to ensure cells covering their region get re-drawn each
// frame on top of the freshly-emitted SIXEL pixels.
func (b *cellBuf) invalidate(x, y int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if x < 0 || y < 0 || x >= b.cols || y >= b.rows {
		return
	}
	b.prev[y*b.cols+x] = types.DrawCell{}
}
