package sixelcanvas_test

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/sixelcanvas"
)

// TestSixelCanvasSkipsRedundantSixelEmit mirrors the ImageView test for
// the canvas: a canvas whose pixels didn't change must not re-encode or
// re-emit on an unrelated flush, but a pixel mutation or a disturbed
// region must.
func TestSixelCanvasSkipsRedundantSixelEmit(t *testing.T) {
	t.Setenv("FV_SIXEL", "1")

	h := term.NewHeadless(40, 12)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	c := sixelcanvas.New(geom.NewRect(2, 1, 14, 7), 32, 32)
	c.State |= consts.SfVisible | consts.SfExposed
	c.Clear(0x204060)

	frame := func() {
		h.Clear(0)
		c.Draw()
		c.PreFlush(h)
		_ = h.Flush()
	}

	h.Reset()
	frame()
	if h.Writes() == "" {
		t.Fatal("first frame emitted nothing; expected the initial SIXEL blit")
	}

	// Unchanged pixels on an unrelated flush: must skip.
	h.Reset()
	frame()
	if got := h.Writes(); got != "" {
		t.Fatalf("static canvas re-emitted on an unrelated flush (%d bytes); want skip", len(got))
	}

	// Mutating the pixel buffer must re-emit.
	c.FillRect(0, 0, 16, 16, 0xC02020)
	h.Reset()
	frame()
	if h.Writes() == "" {
		t.Fatal("canvas did not re-emit after a pixel change")
	}

	// Steady again, then a disturbed region must re-emit.
	h.Reset()
	frame()
	if h.Writes() != "" {
		t.Fatal("expected steady state to skip after a re-emit")
	}
	views.InvalidateRect(2, 1, c.Size.X, c.Size.Y)
	h.Reset()
	frame()
	if h.Writes() == "" {
		t.Fatal("canvas did not re-emit after its region was invalidated")
	}
}
