package term

import (
	"io"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/sixel"
)

// probeCellPixelSize asks the terminal for the pixel dimensions of one
// character cell via CSI 16t and, if a parsable response arrives within
// the budget, calls sixel.SetCellSize. The reader's CSI parser is
// already cross-platform; we install an OnCellSize callback to capture
// the reply, fire the query, and wait briefly on a one-shot channel.
//
// Cross-platform: works wherever the reader's Read loop has started
// (Unix tty, Windows console after VT input is enabled). Terminals
// without CSI 16t support never respond and we time out cleanly.
func probeCellPixelSize(r *reader, out io.Writer) {
	if r == nil || out == nil {
		return
	}
	got := make(chan [2]int, 1)
	r.OnCellSize = func(w, h int) {
		select {
		case got <- [2]int{w, h}:
		default: // already received once; ignore subsequent
		}
		// One-shot: clear the callback so subsequent runtime queries
		// (e.g., a window-resize CSI 16t we might add later) don't
		// race with this startup probe.
		r.OnCellSize = nil
	}
	if _, err := io.WriteString(out, "\x1b[16t"); err != nil {
		r.OnCellSize = nil
		return
	}
	// Reader has its own goroutine; we start it before calling here.
	// Wait up to 200ms — slow terminals (Windows Terminal cold start,
	// remote SSH) sometimes lag behind 100ms.
	select {
	case sz := <-got:
		sixel.SetCellSize(sz[0], sz[1])
	case <-time.After(200 * time.Millisecond):
		r.OnCellSize = nil
	}
}
