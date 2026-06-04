package imageview_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/imageview"
)

func solidImage(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// TestImageViewSkipsRedundantSixelEmit locks in the dirty-tracking fix:
// a static SIXEL image re-emits only when its content/layout changed or
// its region was disturbed — not on every flush an unrelated widget
// (a 1 Hz clock, a CPU sparkline) forces. Before the fix PreFlush
// re-blit the cached DCS unconditionally, which flickered the image.
func TestImageViewSkipsRedundantSixelEmit(t *testing.T) {
	t.Setenv("FV_SIXEL", "1")

	h := term.NewHeadless(40, 12)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	iv := imageview.New(geom.NewRect(2, 1, 14, 7)) // 12x6 cells at origin (2,1)
	iv.State |= consts.SfVisible | consts.SfExposed
	iv.Quality = false // realtime encoder: cheap and deterministic here
	iv.SetImage(solidImage(8, 8, color.RGBA{200, 50, 50, 255}))

	// One frame mirrors the app's idle() order: Clear (resets the back
	// buffer), Draw (stamps sentinels), PreFlush (emit or skip), Flush
	// (commits cur->prev so the next diff is relative to this frame).
	frame := func() {
		h.Clear(0)
		iv.Draw()
		iv.PreFlush(h)
		_ = h.Flush()
	}

	h.Reset()
	frame()
	if h.Writes() == "" {
		t.Fatal("first frame emitted nothing; expected the initial SIXEL blit")
	}

	// Unrelated flush, nothing in the region changed: must skip.
	h.Reset()
	frame()
	if got := h.Writes(); got != "" {
		t.Fatalf("static image re-emitted on an unrelated flush (%d bytes); want skip", len(got))
	}

	// Region invalidated (covering window moved off, or a resize wipe):
	// must re-emit.
	views.InvalidateRect(2, 1, iv.Size.X, iv.Size.Y)
	h.Reset()
	frame()
	if h.Writes() == "" {
		t.Fatal("image did not re-emit after its region was invalidated")
	}

	// Back to steady state, then a new image must re-emit.
	h.Reset()
	frame()
	if h.Writes() != "" {
		t.Fatal("expected steady state to skip after a re-emit")
	}
	iv.SetImage(solidImage(8, 8, color.RGBA{50, 200, 50, 255}))
	h.Reset()
	frame()
	if h.Writes() == "" {
		t.Fatal("new image content did not re-emit")
	}
}
