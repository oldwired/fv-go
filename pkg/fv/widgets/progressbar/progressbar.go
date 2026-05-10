// Package progressbar provides ProgressBar, a single-line visual
// progress indicator with optional percentage label.
package progressbar

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// ProgressBar is a [Min..Max] gauge filled left-to-right.
type ProgressBar struct {
	views.Base

	Min, Max    int
	Position    int
	ShowPercent bool
	FilledChar  rune
	EmptyChar   rune
}

// New constructs a ProgressBar with range [min..max].
func New(bounds geom.Rect, min, max int) *ProgressBar {
	p := &ProgressBar{
		Base:        views.NewBase(bounds),
		Min:         min,
		Max:         max,
		Position:    min,
		ShowPercent: true,
		FilledChar:  '█',
		EmptyChar:   '░',
	}
	p.SetSelf(p)
	return p
}

// GetTypeID for the serial registry.
func (p *ProgressBar) GetTypeID() string { return "progressbar" }

// SetProgress clamps v to [Min, Max] and stores it.
func (p *ProgressBar) SetProgress(v int) {
	if v < p.Min {
		v = p.Min
	}
	if v > p.Max {
		v = p.Max
	}
	p.Position = v
}

// SetRange installs a new (min, max), clamping the current position.
func (p *ProgressBar) SetRange(min, max int) {
	p.Min, p.Max = min, max
	if p.Position < min {
		p.Position = min
	}
	if p.Position > max {
		p.Position = max
	}
}

// Reset returns the position to Min.
func (p *ProgressBar) Reset() { p.Position = p.Min }

// Draw paints the bar.
func (p *ProgressBar) Draw() {
	emptyColor := types.MakeAttr(0x07, 0x01)  // light gray on blue
	filledColor := types.MakeAttr(0x0E, 0x02) // bright yellow on green
	textColor := types.MakeAttr(0x0F, 0x00)   // bright white on black overlay

	span := p.Max - p.Min
	percent := 0
	filledWidth := 0
	if span > 0 {
		percent = (p.Position - p.Min) * 100 / span
		filledWidth = (p.Position - p.Min) * p.Size.X / span
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	if filledWidth < 0 {
		filledWidth = 0
	}
	if filledWidth > p.Size.X {
		filledWidth = p.Size.X
	}

	for y := 0; y < p.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(p.Size.X)
		for x := 0; x < p.Size.X; x++ {
			if x < filledWidth {
				screen.DrawCell(buf, x, string(p.FilledChar), filledColor)
			} else {
				screen.DrawCell(buf, x, string(p.EmptyChar), emptyColor)
			}
		}
		if p.ShowPercent && y == p.Size.Y/2 {
			label := fmt.Sprintf("%d%%", percent)
			textPos := (p.Size.X - len(label)) / 2
			if textPos < 0 {
				textPos = 0
			}
			for i, r := range label {
				if textPos+i < p.Size.X {
					buf[textPos+i] = types.DrawCell{Ch: string(r), Attr: textColor}
				}
			}
		}
		p.WriteLine(0, y, p.Size.X, 1, buf)
	}
}
