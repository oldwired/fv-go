// Package ramview provides RAMView — a memory-utilization bar with
// used / free / total annotations.
package ramview

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Stats reports memory usage in bytes.
type Stats struct {
	Used, Total uint64
}

// Sampler returns current Stats.
type Sampler func() Stats

// RAMView paints "RAM 4.2 / 16.0 GiB ████████░░".
type RAMView struct {
	views.Base

	Sample   Sampler
	Interval time.Duration
	stats    Stats

	BarColor   uint16
	BackColor  uint16
	LabelColor uint16
}

// New constructs a RAMView.
func New(bounds geom.Rect, sampler Sampler, interval time.Duration) *RAMView {
	if interval == 0 {
		interval = time.Second
	}
	r := &RAMView{
		Base:       views.NewBase(bounds),
		Sample:     sampler,
		Interval:   interval,
		BarColor:   theme.Get().ChartBar2,
		BackColor:  theme.Get().GaugeBack,
		LabelColor: theme.Get().GaugeLabel,
	}
	r.SetSelf(r)
	if sampler != nil {
		anim.Register(r, interval)
	}
	return r
}

// GetTypeID for serial registry.
func (r *RAMView) GetTypeID() string { return "ramview" }

// Tick polls.
func (r *RAMView) Tick(now time.Time) bool {
	if r.Sample == nil {
		return false
	}
	r.stats = r.Sample()
	return true
}

// Draw paints label + bar.
func (r *RAMView) Draw() {
	frac := 0.0
	if r.stats.Total > 0 {
		frac = float64(r.stats.Used) / float64(r.stats.Total)
	}
	label := fmt.Sprintf("RAM %s / %s",
		humanBytes(r.stats.Used), humanBytes(r.stats.Total))

	for y := 0; y < r.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(r.Size.X)
		fill := int(frac * float64(r.Size.X))
		for x := 0; x < r.Size.X; x++ {
			ch := "░"
			attr := r.BackColor
			if x < fill {
				ch = "█"
				attr = r.BarColor
			}
			buf[x] = types.DrawCell{Ch: ch, Attr: attr}
		}
		if y == r.Size.Y/2 {
			start := (r.Size.X - len(label)) / 2
			if start < 0 {
				start = 0
			}
			for i, c := range label {
				if start+i < r.Size.X {
					buf[start+i].Ch = string(c)
					buf[start+i].Attr = r.LabelColor
				}
			}
		}
		r.WriteLine(0, y, r.Size.X, 1, buf)
	}
}

// humanBytes formats n as a human-readable byte count.
func humanBytes(n uint64) string {
	const k = 1024.0
	v := float64(n)
	switch {
	case v >= k*k*k*k:
		return fmt.Sprintf("%.1f TiB", v/(k*k*k*k))
	case v >= k*k*k:
		return fmt.Sprintf("%.1f GiB", v/(k*k*k))
	case v >= k*k:
		return fmt.Sprintf("%.1f MiB", v/(k*k))
	case v >= k:
		return fmt.Sprintf("%.1f KiB", v/k)
	}
	return fmt.Sprintf("%d B", n)
}
