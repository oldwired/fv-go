package imageview

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forgedPNGHeader builds a PNG signature + IHDR chunk declaring w×h with
// no pixel data — enough for image.DecodeConfig to read the dimensions.
func forgedPNGHeader(w, h uint32) []byte {
	var b bytes.Buffer
	b.Write([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 6 // color type: RGBA
	_ = binary.Write(&b, binary.BigEndian, uint32(13))
	b.WriteString("IHDR")
	b.Write(ihdr)
	crc := crc32.ChecksumIEEE(append([]byte("IHDR"), ihdr...))
	_ = binary.Write(&b, binary.BigEndian, crc)
	return b.Bytes()
}

func TestLoadFileRejectsOversizeDimensions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bomb.png")
	// 100000 × 100000 = 10^10 pixels, far over the 64M cap.
	if err := os.WriteFile(path, forgedPNGHeader(100000, 100000), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile accepted a 100000x100000 image (decompression bomb)")
	}
	if !strings.Contains(err.Error(), "exceed") {
		t.Errorf("error = %v, want it to mention the dimension limit", err)
	}
}

func TestLoadFileAcceptsNormalImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.png")
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile rejected a normal 4x3 image: %v", err)
	}
	if got.Bounds().Dx() != 4 || got.Bounds().Dy() != 3 {
		t.Errorf("decoded bounds = %v, want 4x3", got.Bounds())
	}
}
