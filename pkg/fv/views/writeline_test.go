package views

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/types"
)

// TestWriteLineMultiRowReadsCorrectRow regression for the latent
// "buf[col] regardless of row" bug in Base.WriteLine. With h>1 and a
// w*h buffer, each output row must come from its own buffer row, not
// repeat row 0.
func TestWriteLineMultiRowReadsCorrectRow(t *testing.T) {
	h := term.NewHeadless(10, 3)
	SetRootBackend(h)
	defer SetRootBackend(nil)

	v := newDummy(geom.NewRect(0, 0, 10, 3))
	v.State |= consts.SfVisible | consts.SfExposed

	const w, rows = 4, 2
	buf := make(screen.DrawBuffer, w*rows)
	for i := 0; i < w; i++ {
		buf[i] = types.DrawCell{Ch: "A"}   // row 0
		buf[w+i] = types.DrawCell{Ch: "B"} // row 1
	}
	v.WriteLine(0, 0, w, rows, buf)
	_ = h.Flush()

	snap := h.Snapshot()
	// Expect two distinct rows. Before the fix this would have been
	// "AAAA\nAAAA".
	want := "AAAA\nBBBB"
	if snap[:len(want)] != want {
		t.Errorf("multi-row WriteLine: got %q, want prefix %q", snap, want)
	}
}
