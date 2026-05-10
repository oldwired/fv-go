// Package cpucore provides CPUCoreView — a multi-bar gauge showing
// per-core CPU utilization.
package cpucore

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Sampler returns per-core utilization, each in [0.0, 1.0].
type Sampler func() []float64

// CPUCoreView paints one row per core.
type CPUCoreView struct {
	views.Base

	Sample   Sampler
	Interval time.Duration
	levels   []float64

	BarColor   uint16
	BackColor  uint16
	HighColor  uint16
	LabelColor uint16
}

// New constructs a CPUCoreView.
func New(bounds geom.Rect, sampler Sampler, interval time.Duration) *CPUCoreView {
	if interval == 0 {
		interval = time.Second
	}
	c := &CPUCoreView{
		Base:       views.NewBase(bounds),
		Sample:     sampler,
		Interval:   interval,
		BarColor:   types.MakeAttr(0x0A, 0x00),
		BackColor:  types.MakeAttr(0x08, 0x00),
		HighColor:  types.MakeAttr(0x0C, 0x00),
		LabelColor: types.MakeAttr(0x07, 0x00),
	}
	c.SetSelf(c)
	if sampler != nil {
		anim.Register(c, interval)
	}
	return c
}

// GetTypeID for serial registry.
func (c *CPUCoreView) GetTypeID() string { return "cpucoreview" }

// Tick polls the sampler.
func (c *CPUCoreView) Tick(now time.Time) bool {
	if c.Sample == nil {
		return false
	}
	c.levels = c.Sample()
	return true
}

// Draw paints the bars.
func (c *CPUCoreView) Draw() {
	for y := 0; y < c.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(c.Size.X)
		for x := 0; x < c.Size.X; x++ {
			screen.DrawCell(buf, x, " ", c.LabelColor)
		}
		if y < len(c.levels) {
			lvl := c.levels[y]
			if lvl < 0 {
				lvl = 0
			}
			if lvl > 1 {
				lvl = 1
			}
			label := fmt.Sprintf("CPU%2d ", y)
			for i, r := range label {
				if i < c.Size.X {
					buf[i] = types.DrawCell{Ch: string(r), Attr: c.LabelColor}
				}
			}
			barX := len(label)
			barArea := c.Size.X - barX - 6
			if barArea < 4 {
				barArea = 4
			}
			fill := int(lvl * float64(barArea))
			barCol := c.BarColor
			if lvl > 0.85 {
				barCol = c.HighColor
			}
			for x := 0; x < barArea && barX+x < c.Size.X; x++ {
				ch := "░"
				attr := c.BackColor
				if x < fill {
					ch = "█"
					attr = barCol
				}
				buf[barX+x] = types.DrawCell{Ch: ch, Attr: attr}
			}
			pct := fmt.Sprintf(" %3d%%", int(lvl*100))
			pos := c.Size.X - len(pct)
			for i, r := range pct {
				if pos+i < c.Size.X && pos+i >= 0 {
					buf[pos+i] = types.DrawCell{Ch: string(r), Attr: c.LabelColor}
				}
			}
		}
		c.WriteLine(0, y, c.Size.X, 1, buf)
	}
}
