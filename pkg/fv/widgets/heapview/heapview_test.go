package heapview

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

func TestFormatBytesAuto(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{1024 * 1024 * 1024, "1.00 GB"},
	}
	for _, c := range cases {
		got := formatBytes(c.in, Auto)
		if got != c.want {
			t.Errorf("formatBytes(%d, Auto) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatBytesForcedMode(t *testing.T) {
	// 5 MiB in different forced units.
	const five = 5 * 1024 * 1024
	if got := formatBytes(five, Bytes); got != "5242880 B" {
		t.Errorf("Bytes mode: got %q", got)
	}
	if got := formatBytes(five, KB); got != "5120.0 KB" {
		t.Errorf("KB mode: got %q", got)
	}
	if got := formatBytes(five, MB); got != "5.0 MB" {
		t.Errorf("MB mode: got %q", got)
	}
}

// TestNewDefaults: bare construction sets Auto + 2s + theme color.
func TestNewDefaults(t *testing.T) {
	h := New(geom.NewRect(0, 0, 20, 1))
	defer anim.Unregister(h)
	if h.Mode != Auto {
		t.Errorf("Mode default: got %v, want Auto", h.Mode)
	}
	if h.Interval == 0 {
		t.Error("Interval default should be non-zero")
	}
	if h.BaseView().Self() != h {
		t.Error("constructor forgot SetSelf")
	}
}
