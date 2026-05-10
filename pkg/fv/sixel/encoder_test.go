package sixel

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

// TestEncodeRealtimeStructure verifies the DCS envelope, raster header,
// palette block, and pixel-data section all appear in the right order
// for a tiny solid-color input. The encoder builds output deterministically
// so we can string-match the framing.
func TestEncodeRealtimeStructure(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 6))
	red := color.RGBA{255, 0, 0, 255}
	for y := 0; y < 6; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, red)
		}
	}
	out := EncodeRealtime(img, 1)
	if !strings.HasPrefix(out, "\x1bP0;1;0q") {
		t.Fatalf("missing DCS prefix; got %q", first(out, 16))
	}
	if !strings.HasSuffix(out, "\x1b\\") {
		t.Fatalf("missing ST suffix; got %q", last(out, 6))
	}
	if !strings.Contains(out, `"1;1;4;6`) {
		t.Errorf("raster header for 4×6 not found in output")
	}
	// Pure red is cube index 5*36 = 180, so we expect "#180" in the
	// pixel section. A solid 4×6 red block in a single 6-row band
	// becomes one selection + a run of 4 sixel chars all = '?'+0x3F = '~'.
	if !strings.Contains(out, "#180") {
		t.Errorf("expected #180 (pure red) selection in output")
	}
	// 216 palette entries means 216 `#N;2;` color definitions.
	if got := strings.Count(out, ";2;"); got < 216 {
		t.Errorf("expected ≥216 color definitions, got %d", got)
	}
}

// TestEncodeRealtimeBlankImage encodes a zero-area image and gets back
// an empty string; nothing should crash.
func TestEncodeRealtimeBlankImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 0, 0))
	if out := EncodeRealtime(img, 1); out != "" {
		t.Errorf("expected empty output for empty image, got %d bytes", len(out))
	}
}

// TestCubeRoundTrip — every cube index round-trips through cubeIndex.
func TestCubeRoundTrip(t *testing.T) {
	for i := 0; i < 216; i++ {
		c := CubeColor(i)
		if got := cubeIndex(c.R, c.G, c.B); got != i {
			t.Errorf("cube %d → color %v → cube %d (mismatch)", i, c, got)
		}
	}
}

func first(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

func last(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[len(s)-n:]
}
