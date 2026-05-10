// Package cpumeter provides CPUMeter — a single-bar CPU-utilization
// gauge. Pulls samples from a caller-supplied Sampler so the widget
// stays portable; the demo wires a synthetic sample for now.
package cpumeter

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Sampler returns a CPU usage value in [0.0, 1.0].
type Sampler func() float64

// CPUMeter renders a horizontal bar.
type CPUMeter struct {
	views.Base

	Sample   Sampler
	Interval time.Duration
	level    float64

	BarColor  uint16
	BackColor uint16
	HighColor uint16
}

// New constructs a meter that polls sampler every interval.
func New(bounds geom.Rect, sampler Sampler, interval time.Duration) *CPUMeter {
	if interval == 0 {
		interval = 500 * time.Millisecond
	}
	c := &CPUMeter{
		Base:      views.NewBase(bounds),
		Sample:    sampler,
		Interval:  interval,
		BarColor:  types.MakeAttr(0x0A, 0x00),
		BackColor: types.MakeAttr(0x08, 0x00),
		HighColor: types.MakeAttr(0x0C, 0x00),
	}
	c.SetSelf(c)
	if sampler != nil {
		anim.Register(c, interval)
	}
	return c
}

// GetTypeID for serial registry.
func (c *CPUMeter) GetTypeID() string { return "cpumeter" }

// Tick polls the sampler.
func (c *CPUMeter) Tick(now time.Time) bool {
	if c.Sample == nil {
		return false
	}
	c.level = c.Sample()
	if c.level < 0 {
		c.level = 0
	}
	if c.level > 1 {
		c.level = 1
	}
	return true
}

// Draw paints the bar.
func (c *CPUMeter) Draw() {
	pct := int(c.level * 100)
	label := fmt.Sprintf(" CPU %3d%% ", pct)

	for y := 0; y < c.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(c.Size.X)
		barCol := c.BarColor
		if c.level > 0.85 {
			barCol = c.HighColor
		}
		fill := int(c.level * float64(c.Size.X))
		for x := 0; x < c.Size.X; x++ {
			ch := "░"
			attr := c.BackColor
			if x < fill {
				ch = "█"
				attr = barCol
			}
			buf[x] = types.DrawCell{Ch: ch, Attr: attr}
		}
		if y == c.Size.Y/2 {
			start := (c.Size.X - len(label)) / 2
			if start < 0 {
				start = 0
			}
			for i, r := range label {
				if start+i < c.Size.X {
					buf[start+i].Ch = string(r)
				}
			}
		}
		c.WriteLine(0, y, c.Size.X, 1, buf)
	}
}
