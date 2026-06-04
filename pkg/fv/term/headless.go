package term

import (
	"strings"
	"sync"

	"github.com/oldwired/fv-go/pkg/fv/types"
)

// Headless is an in-memory Backend implementation for tests. No tty is
// touched; flush is a no-op (Snapshot reads the back buffer directly).
// Suitable for CI golden-file testing of widget output, and for
// scripted-event tests of event dispatch logic.
//
// Headless is safe to construct in any environment — it never touches
// stdin/stdout/syscalls. Concurrent use of PushEvent + Snapshot is
// safe; SetCell etc. are not goroutine-safe (matches the real backends'
// contract — those are only ever called from the UI goroutine).
type Headless struct {
	cells   *cellBuf
	events  chan Event
	cursorX int
	cursorY int
	visible bool

	mu sync.Mutex // protects writeLog
	// writeLog captures everything passed to WriteRaw, for tests that
	// want to assert on emitted escape sequences (OSC 52 clipboard,
	// SIXEL, etc.).
	writeLog strings.Builder
}

// NewHeadless builds a Headless backend with a viewport of cols×rows
// cells. Use this directly in tests; production code stays on term.New().
func NewHeadless(cols, rows int) *Headless {
	return &Headless{
		cells:   newCellBuf(cols, rows),
		events:  make(chan Event, 64),
		cursorX: -1,
		cursorY: -1,
	}
}

// Compile-time check.
var _ Backend = (*Headless)(nil)

// The methods below satisfy the term.Backend interface. See
// Backend's documentation for per-method semantics; the Headless
// implementations are in-memory no-ops that record state for tests.

func (h *Headless) Init() error  { return nil }
func (h *Headless) Close() error { return nil }

func (h *Headless) Size() (int, int) { return h.cells.cols, h.cells.rows }

func (h *Headless) SetCell(x, y int, c types.DrawCell) { h.cells.Set(x, y, c) }
func (h *Headless) GetCell(x, y int) types.DrawCell    { return h.cells.Get(x, y) }
func (h *Headless) Clear(attr uint16)                  { h.cells.Clear(attr) }

// Flush commits the back buffer over the front buffer so subsequent
// MarkClean / Invalidate operations behave like the real backends.
// No bytes are emitted — Snapshot reads cells directly.
func (h *Headless) Flush() error {
	h.cells.commit()
	return nil
}

func (h *Headless) SetCursor(x, y int) {
	h.cursorX = x
	h.cursorY = y
}

func (h *Headless) WriteRaw(s string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writeLog.WriteString(s)
	return nil
}

func (h *Headless) MarkClean(x, y int)           { h.cells.markClean(x, y) }
func (h *Headless) Invalidate(x, y int)          { h.cells.invalidate(x, y) }
func (h *Headless) WasInvalidated(x, y int) bool { return h.cells.wasInvalidated(x, y) }
func (h *Headless) ShowCursor(v bool)            { h.visible = v }
func (h *Headless) Events() <-chan Event         { return h.events }
func (h *Headless) Suspend() error               { return nil }
func (h *Headless) Resume() error                { return nil }

// PushEvent enqueues an input event. Blocks if the buffered channel
// is full — tests should keep volume modest (the buffer is 64 deep).
func (h *Headless) PushEvent(ev Event) { h.events <- ev }

// Snapshot renders the back buffer as text: rows separated by '\n',
// trailing whitespace stripped per row. Suitable for golden-file
// comparison.
func (h *Headless) Snapshot() string {
	var sb strings.Builder
	for y := 0; y < h.cells.rows; y++ {
		var line strings.Builder
		for x := 0; x < h.cells.cols; x++ {
			c := h.cells.Get(x, y)
			if c.Ch == "" {
				line.WriteByte(' ')
			} else {
				line.WriteString(c.Ch)
			}
		}
		// Trim trailing spaces so unrelated padding doesn't make
		// goldens brittle.
		row := strings.TrimRight(line.String(), " ")
		sb.WriteString(row)
		if y < h.cells.rows-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// Writes returns a copy of everything passed to WriteRaw since the
// last Reset(). Used by tests that want to inspect OSC 52 / SIXEL
// emission.
func (h *Headless) Writes() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.writeLog.String()
}

// Reset clears the recorded WriteRaw log. The cell buffers and event
// channel are untouched.
func (h *Headless) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writeLog.Reset()
}

// Cursor reports the most recent (x, y) passed to SetCursor and the
// most recent ShowCursor visibility. Useful for tests verifying the
// caret position the program would have used on a real backend.
func (h *Headless) Cursor() (x, y int, visible bool) {
	return h.cursorX, h.cursorY, h.visible
}
