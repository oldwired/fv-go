// Package vumeter provides VUMeter — an audio-style level meter with
// peak hold and a yellow / red threshold zone.
package vumeter

import (
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// VUMeter is a horizontal level bar with green / yellow / red zones,
// a peak indicator that decays over time, and a fall-back when the
// instantaneous level drops.
type VUMeter struct {
	views.Base

	// Level in [0, 1]. SetLevel clamps.
	Level float64
	Peak  float64

	// Thresholds (fraction of full scale) for the color transition.
	YellowAt float64
	RedAt    float64

	// PeakDecay is the rate at which Peak falls per second.
	PeakDecay float64
	// FallRate is the rate at which Level slides back toward 0 per
	// second when no new SetLevel has come in.
	FallRate float64

	GreenAttr  uint16
	YellowAttr uint16
	RedAttr    uint16
	BackAttr   uint16
	PeakAttr   uint16

	lastTick time.Time
}

// New constructs a VUMeter with default green/yellow/red thresholds.
func New(bounds geom.Rect) *VUMeter {
	v := &VUMeter{
		Base:       views.NewBase(bounds),
		YellowAt:   0.6,
		RedAt:      0.85,
		PeakDecay:  0.5, // 50% of full scale per second
		FallRate:   1.0,
		GreenAttr:  types.MakeAttr(0x0A, 0x00),
		YellowAttr: types.MakeAttr(0x0E, 0x00),
		RedAttr:    types.MakeAttr(0x0C, 0x00),
		BackAttr:   types.MakeAttr(0x08, 0x00),
		PeakAttr:   types.MakeAttr(0x0F, 0x00),
		lastTick:   time.Now(),
	}
	v.SetSelf(v)
	anim.Register(v, 50*time.Millisecond)
	return v
}

// GetTypeID for serial registry.
func (v *VUMeter) GetTypeID() string { return "vumeter" }

// SetLevel sets the instantaneous level, clamping to [0, 1] and bumping
// Peak if the new level is higher.
func (v *VUMeter) SetLevel(level float64) {
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}
	v.Level = level
	if level > v.Peak {
		v.Peak = level
	}
}

// Tick decays Peak and Level. Called by the anim package.
func (v *VUMeter) Tick(now time.Time) bool {
	dt := now.Sub(v.lastTick).Seconds()
	v.lastTick = now
	if dt <= 0 {
		return false
	}
	changed := false
	if v.Peak > v.Level {
		v.Peak -= v.PeakDecay * dt
		if v.Peak < v.Level {
			v.Peak = v.Level
		}
		changed = true
	}
	if v.Level > 0 {
		v.Level -= v.FallRate * dt
		if v.Level < 0 {
			v.Level = 0
		}
		changed = true
	}
	return changed
}

// Draw paints the meter.
func (v *VUMeter) Draw() {
	w := v.Size.X
	if w <= 0 {
		return
	}
	level := int(v.Level * float64(w))
	if level < 0 {
		level = 0
	}
	if level > w {
		level = w
	}
	peak := int(v.Peak * float64(w))
	if peak >= w {
		peak = w - 1
	}
	if peak < 0 {
		peak = 0
	}
	yellowAt := int(v.YellowAt * float64(w))
	redAt := int(v.RedAt * float64(w))

	for y := 0; y < v.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(w)
		for x := 0; x < w; x++ {
			ch := "░"
			attr := v.BackAttr
			if x < level {
				switch {
				case x >= redAt:
					attr = v.RedAttr
				case x >= yellowAt:
					attr = v.YellowAttr
				default:
					attr = v.GreenAttr
				}
				ch = "█"
			}
			buf[x] = types.DrawCell{Ch: ch, Attr: attr}
		}
		if v.Peak > 0 && peak < w {
			buf[peak] = types.DrawCell{Ch: "▌", Attr: v.PeakAttr}
		}
		v.WriteLine(0, y, w, 1, buf)
	}
}
