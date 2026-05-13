// Package blink provides BlinkIndicator — a small "activity" dot with
// off / steady / blinking states. Drives its own redraw via the
// shared anim ticker.
package blink

import (
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// State values.
type State int

const (
	StateOff      State = iota // dim dot
	StateSteady                // bright dot, no blinking
	StateBlinking              // dot toggles every Interval
)

// BlinkIndicator paints a single cell that toggles visibility on a timer.
type BlinkIndicator struct {
	views.Base

	State    State
	OnChar   rune
	OffChar  rune
	OnAttr   uint16
	OffAttr  uint16
	Interval time.Duration

	visible bool
}

// New constructs an indicator at the given bounds. A 1×1 size is the
// usual case; multi-cell rectangles fill with the same character.
func New(bounds geom.Rect) *BlinkIndicator {
	b := &BlinkIndicator{
		Base:     views.NewBase(bounds),
		State:    StateOff,
		OnChar:   '●',
		OffChar:  '○',
		OnAttr:   theme.Get().BlinkText,
		OffAttr:  theme.Get().GaugeBack,
		Interval: 500 * time.Millisecond,
		visible:  true,
	}
	b.SetSelf(b)
	return b
}

// GetTypeID for serial registry.
func (b *BlinkIndicator) GetTypeID() string { return "blinkindicator" }

// SetMode reconfigures the indicator and (re)registers the ticker.
// (Named SetMode rather than SetState to avoid colliding with the
// View interface's SetState(uint16, bool) method.)
func (b *BlinkIndicator) SetMode(s State) {
	b.State = s
	if s == StateBlinking {
		anim.Register(b, b.Interval)
	} else {
		anim.Unregister(b)
		b.visible = s == StateSteady
	}
	b.Draw()
}

// Tick is the anim.Ticker callback.
func (b *BlinkIndicator) Tick(now time.Time) bool {
	if b.State != StateBlinking {
		return false
	}
	b.visible = !b.visible
	return true
}

// Draw paints the dot.
func (b *BlinkIndicator) Draw() {
	on := b.State == StateSteady || (b.State == StateBlinking && b.visible)
	ch := b.OffChar
	color := b.OffAttr
	if on {
		ch = b.OnChar
		color = b.OnAttr
	}
	for y := 0; y < b.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(b.Size.X)
		for x := 0; x < b.Size.X; x++ {
			screen.DrawCell(buf, x, string(ch), color)
		}
		b.WriteLine(0, y, b.Size.X, 1, buf)
	}
}
