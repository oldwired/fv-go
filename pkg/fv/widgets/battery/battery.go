// Package battery provides BatteryView — a small "battery icon"
// rendering with charge percentage and AC/discharging status.
package battery

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Status describes one battery sample.
type Status struct {
	Charge   float64 // [0.0, 1.0]
	OnAC     bool
	Charging bool
}

// Sampler returns current battery state.
type Sampler func() Status

// BatteryView renders the battery icon with bar fill + label.
type BatteryView struct {
	views.Base

	Sample   Sampler
	Interval time.Duration
	state    Status

	GoodColor  uint16
	WarnColor  uint16
	CritColor  uint16
	BackColor  uint16
	LabelColor uint16
}

// New constructs a BatteryView.
func New(bounds geom.Rect, sampler Sampler, interval time.Duration) *BatteryView {
	if interval == 0 {
		interval = 30 * time.Second
	}
	b := &BatteryView{
		Base:       views.NewBase(bounds),
		Sample:     sampler,
		Interval:   interval,
		GoodColor:  types.MakeAttr(0x0A, 0x00),
		WarnColor:  types.MakeAttr(0x0E, 0x00),
		CritColor:  types.MakeAttr(0x0C, 0x00),
		BackColor:  types.MakeAttr(0x08, 0x00),
		LabelColor: types.MakeAttr(0x0F, 0x00),
	}
	b.SetSelf(b)
	if sampler != nil {
		anim.Register(b, interval)
	}
	return b
}

// GetTypeID for serial registry.
func (b *BatteryView) GetTypeID() string { return "batteryview" }

// Tick polls.
func (b *BatteryView) Tick(now time.Time) bool {
	if b.Sample == nil {
		return false
	}
	b.state = b.Sample()
	return true
}

// Draw paints the icon.
func (b *BatteryView) Draw() {
	pct := int(b.state.Charge * 100)
	icon := "🔋"
	if b.state.Charging {
		icon = "⚡"
	} else if b.state.OnAC {
		icon = "🔌"
	}
	color := b.GoodColor
	switch {
	case b.state.Charge < 0.15:
		color = b.CritColor
	case b.state.Charge < 0.30:
		color = b.WarnColor
	}
	label := fmt.Sprintf("%s %3d%%", icon, pct)
	for y := 0; y < b.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(b.Size.X)
		fill := int(b.state.Charge * float64(b.Size.X))
		for x := 0; x < b.Size.X; x++ {
			ch := "░"
			attr := b.BackColor
			if x < fill {
				ch = "█"
				attr = color
			}
			buf[x] = types.DrawCell{Ch: ch, Attr: attr}
		}
		if y == b.Size.Y/2 {
			start := (b.Size.X - len(label)) / 2
			if start < 0 {
				start = 0
			}
			for i, r := range label {
				if start+i < b.Size.X {
					buf[start+i].Ch = string(r)
					buf[start+i].Attr = b.LabelColor
				}
			}
		}
		b.WriteLine(0, y, b.Size.X, 1, buf)
	}
}
