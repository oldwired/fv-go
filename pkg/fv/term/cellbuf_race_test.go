package term

import (
	"sync"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/types"
)

// TestCellBufResizeRace exercises the concurrent-access scenario that
// SIGWINCH (POSIX) and the Windows reader goroutine create: one
// goroutine writes cells while another reallocates the underlying
// buffer. Run with `go test -race` — without the mutex this surfaces
// either a race detector report or an "index out of range" panic.
func TestCellBufResizeRace(t *testing.T) {
	b := newCellBuf(80, 24)

	var wg sync.WaitGroup

	// Writer: hammers SetCell within bounds. Reads its bounds via
	// the locked Size() accessor on every iteration so it sees
	// the current shape after each resize.
	const iters = 5000
	wg.Add(1)
	go func() {
		defer wg.Done()
		cell := types.DrawCell{Ch: "x", Attr: types.MakeAttr(7, 0)}
		for i := 0; i < iters; i++ {
			cols, rows := b.Size()
			if cols == 0 || rows == 0 {
				continue
			}
			b.Set(i%cols, i%rows, cell)
		}
	}()

	// Resizer: alternates between two viewports.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iters/10; i++ {
			if i%2 == 0 {
				b.Resize(80, 24)
			} else {
				b.Resize(120, 40)
			}
		}
	}()

	// Reader: races with the writer.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			cols, rows := b.Size()
			if cols == 0 || rows == 0 {
				continue
			}
			_ = b.Get(i%cols, i%rows)
		}
	}()

	wg.Wait()
}

// TestCellBufConcurrentDirtyAndSet: dirty() snapshots cur+prev under
// the mutex while another goroutine writes. Without the mutex,
// dirty could observe a half-written cell or a torn slice header
// during Resize.
func TestCellBufConcurrentDirtyAndSet(t *testing.T) {
	b := newCellBuf(40, 10)

	var wg sync.WaitGroup
	const iters = 2000

	wg.Add(1)
	go func() {
		defer wg.Done()
		cell := types.DrawCell{Ch: "z", Attr: types.MakeAttr(7, 0)}
		for i := 0; i < iters; i++ {
			cols, rows := b.Size()
			if cols == 0 || rows == 0 {
				continue
			}
			b.Set(i%cols, i%rows, cell)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			_ = b.dirty()
			b.commit()
		}
	}()

	wg.Wait()
}
