// Package diskusage provides DiskUsageView — one row per mounted
// volume showing used/total and a fill bar.
package diskusage

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Volume describes one filesystem.
type Volume struct {
	Path        string
	Used, Total uint64
}

// Sampler returns the current list of volumes.
type Sampler func() []Volume

// DiskUsageView renders one row per volume.
type DiskUsageView struct {
	views.Base

	Sample   Sampler
	Interval time.Duration
	volumes  []Volume

	BarColor   uint16
	BackColor  uint16
	LabelColor uint16
	HighColor  uint16
}

// New constructs a DiskUsageView.
func New(bounds geom.Rect, sampler Sampler, interval time.Duration) *DiskUsageView {
	if interval == 0 {
		interval = 5 * time.Second
	}
	d := &DiskUsageView{
		Base:       views.NewBase(bounds),
		Sample:     sampler,
		Interval:   interval,
		BarColor:   types.MakeAttr(0x09, 0x00),
		BackColor:  types.MakeAttr(0x08, 0x00),
		LabelColor: types.MakeAttr(0x0F, 0x00),
		HighColor:  types.MakeAttr(0x0C, 0x00),
	}
	d.SetSelf(d)
	if sampler != nil {
		anim.Register(d, interval)
	}
	return d
}

// GetTypeID for serial registry.
func (d *DiskUsageView) GetTypeID() string { return "diskusageview" }

// Tick polls.
func (d *DiskUsageView) Tick(now time.Time) bool {
	if d.Sample == nil {
		return false
	}
	d.volumes = d.Sample()
	return true
}

// Draw paints rows.
func (d *DiskUsageView) Draw() {
	for y := 0; y < d.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(d.Size.X)
		for x := 0; x < d.Size.X; x++ {
			screen.DrawCell(buf, x, " ", d.LabelColor)
		}
		if y < len(d.volumes) {
			v := d.volumes[y]
			frac := 0.0
			if v.Total > 0 {
				frac = float64(v.Used) / float64(v.Total)
			}
			label := fmt.Sprintf("%-16s %s / %s", truncate(v.Path, 16),
				humanBytes(v.Used), humanBytes(v.Total))
			screen.DrawStr(buf, 0, label, d.LabelColor)
			barX := len(label) + 1
			barArea := d.Size.X - barX - 6
			if barArea < 4 {
				barArea = 4
			}
			fill := int(frac * float64(barArea))
			barCol := d.BarColor
			if frac > 0.85 {
				barCol = d.HighColor
			}
			for x := 0; x < barArea && barX+x < d.Size.X; x++ {
				ch := "░"
				attr := d.BackColor
				if x < fill {
					ch = "█"
					attr = barCol
				}
				buf[barX+x] = types.DrawCell{Ch: ch, Attr: attr}
			}
			pct := fmt.Sprintf(" %3d%%", int(frac*100))
			pos := d.Size.X - len(pct)
			for i, r := range pct {
				if pos+i < d.Size.X && pos+i >= 0 {
					buf[pos+i] = types.DrawCell{Ch: string(r), Attr: d.LabelColor}
				}
			}
		}
		d.WriteLine(0, y, d.Size.X, 1, buf)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

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
