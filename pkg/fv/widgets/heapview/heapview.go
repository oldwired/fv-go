// Package heapview displays current Go heap memory usage as a tiny
// status-line gadget. It's the Go-runtime analog of Pascal Free
// Vision's THeapView (from gadgets.pas) — the Pascal version reads
// FPC's MemAvail / HeapPtr globals; we read runtime.MemStats.
//
// Construction:
//
//	hv := heapview.New(bounds)        // auto-units, refreshed every 2s
//
// Mode picks the displayed unit (Bytes, KB, MB, GB) or Auto (which
// chooses the largest unit that keeps the number under 1000). Set
// ShowGC=true to append "/N GC" with the cumulative garbage-collection
// count — useful for visualizing GC pressure alongside heap size.
package heapview

import (
	"fmt"
	"runtime"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Mode selects the byte-unit formatting.
type Mode int

const (
	Auto  Mode = iota // largest unit that keeps the number under 1000
	Bytes             // raw byte count with thousand separators
	KB                // kilobytes
	MB                // megabytes
	GB                // gigabytes
)

// HeapView is a one-line widget showing the current heap allocation.
type HeapView struct {
	views.Base

	Mode     Mode
	Interval time.Duration
	Color    uint16
	ShowGC   bool // append "/N GC" with the cumulative GC count
}

// New constructs a heap view with Auto units and a 2-second refresh.
// Heap stats don't change visibly faster than that under normal load,
// and ReadMemStats stops the world briefly — keeping the interval long
// is the polite default.
func New(bounds geom.Rect) *HeapView {
	h := &HeapView{
		Base:     views.NewBase(bounds),
		Mode:     Auto,
		Interval: 2 * time.Second,
		Color:    theme.Get().HeapValue,
	}
	h.SetSelf(h)
	anim.Register(h, h.Interval)
	return h
}

// GetTypeID for serial registry.
func (h *HeapView) GetTypeID() string { return "heapview" }

// Tick is the anim.Ticker entry point. Always wants a redraw — the
// numbers might have shifted, and even when they haven't, recomputing
// is cheap compared to the runtime.ReadMemStats pause it just took.
func (h *HeapView) Tick(now time.Time) bool { return true }

// Draw renders the current heap allocation centered in the widget's
// bounds.
func (h *HeapView) Draw() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	label := "Heap: " + formatBytes(ms.HeapAlloc, h.Mode)
	if h.ShowGC {
		label += fmt.Sprintf(" / %d GC", ms.NumGC)
	}

	for y := 0; y < h.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(h.Size.X)
		for x := 0; x < h.Size.X; x++ {
			screen.DrawCell(buf, x, " ", h.Color)
		}
		if y == h.Size.Y/2 {
			start := (h.Size.X - len(label)) / 2
			if start < 0 {
				start = 0
			}
			screen.DrawStr(buf, start, label, h.Color)
		}
		h.WriteLine(0, y, h.Size.X, 1, buf)
	}
}

// formatBytes renders n in the requested unit. Auto picks the largest
// unit that keeps the displayed number under 1000 — so 950 stays as
// "950 B", 1.2 KiB renders as "1.2 KB", 2.5 MiB as "2.5 MB", etc.
func formatBytes(n uint64, mode Mode) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch mode {
	case Bytes:
		return fmt.Sprintf("%d B", n)
	case KB:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	case MB:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case GB:
		return fmt.Sprintf("%.2f GB", float64(n)/gb)
	default: // Auto
		switch {
		case n < kb:
			return fmt.Sprintf("%d B", n)
		case n < mb:
			return fmt.Sprintf("%.1f KB", float64(n)/kb)
		case n < gb:
			return fmt.Sprintf("%.1f MB", float64(n)/mb)
		default:
			return fmt.Sprintf("%.2f GB", float64(n)/gb)
		}
	}
}
