package imageview

import (
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	// Register decoders. image.Decode dispatches by magic bytes once
	// these have run their init() blocks — stdlib covers PNG/JPEG/GIF,
	// x/image extends to BMP, TIFF, and WebP.
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/oldwired/fv-go/pkg/fv/sixel"
)

func sixelCellSize() (w, h int) { return sixel.CellSize() }

// maxImagePixels caps the declared dimensions an image may decode to. A
// small, highly-compressed file (or one with a forged header) can claim
// enormous dimensions and make the decoder allocate gigabytes before any
// of our own bounded rendering runs — a decompression bomb. 64 megapixels
// (~256 MB at RGBA) is far above any real terminal image.
const maxImagePixels = 64 * 1024 * 1024

// LoadFile reads an image from path and returns the decoded image.
// PNG, JPEG, and GIF are supported via stdlib decoders. Other formats
// (BMP, TIFF, WebP) require additional decoder registrations and are
// not supported here — the caller can do that themselves and pass the
// decoded image to SetImage.
//
// The returned error wraps the underlying decoder error with the path
// for debuggability.
func LoadFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	// Check declared dimensions before the full decode so a forged /
	// bomb header is rejected cheaply, then rewind for the real decode.
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 ||
		int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil, fmt.Errorf("decode %s: image dimensions %dx%d exceed the %d-pixel limit",
			path, cfg.Width, cfg.Height, maxImagePixels)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}

	img, format, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	_ = format // available if a caller ever wants to know
	return img, nil
}

// IsLikelyImageFile is a cheap filename-suffix check; the demo's file
// dialog uses it as a glob hint. Real format detection only happens
// inside LoadFile via image.Decode.
func IsLikelyImageFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff", ".webp":
		return true
	}
	return false
}

// PreferredCells returns the cell dimensions needed to display img at
// native pixel resolution given the rendering mode. SIXEL: one image
// pixel = 1/cellW × 1/cellH cells (cell sizes come from sixel.CellSize).
// Half-block: one image-column = 1 cell, two image-rows = 1 cell. The
// caller is expected to add window-frame padding and clamp to the
// available desktop area — this helper just answers "what size of
// view would be 1:1?".
func PreferredCells(img image.Image, useSixel bool) (cols, rows int) {
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw <= 0 || ih <= 0 {
		return 1, 1
	}
	if useSixel {
		cw, ch := sixelCellSize()
		cols = (iw + cw - 1) / cw
		rows = (ih + ch - 1) / ch
	} else {
		cols = iw
		rows = (ih + 1) / 2
	}
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return
}

// FitCells scales (cols, rows) down so it fits inside (maxCols, maxRows)
// while preserving aspect ratio. Both axes are clamped to a minimum of
// 1. If the input already fits, it's returned unchanged.
func FitCells(cols, rows, maxCols, maxRows int) (int, int) {
	if cols <= maxCols && rows <= maxRows {
		return cols, rows
	}
	sx := float64(maxCols) / float64(cols)
	sy := float64(maxRows) / float64(rows)
	s := sx
	if sy < s {
		s = sy
	}
	cols = int(float64(cols) * s)
	rows = int(float64(rows) * s)
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return cols, rows
}
