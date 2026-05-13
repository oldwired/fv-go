// Package sparkline provides Sparkline — an inline mini-chart that
// renders a series of values as 8-level Unicode block characters.
package sparkline

import (
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// blockChars indexes from "empty" (0/8) up to "full" (8/8).
var blockChars = [9]rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Sparkline holds a ring-buffer of values; oldest values fall off the
// left as new ones are pushed.
type Sparkline struct {
	views.Base

	Values    []float64
	MaxPoints int
	Min, Max  float64 // visible range; auto-fit when AutoScale = true
	AutoScale bool
	Color     uint16
}

// New creates a sparkline that holds up to maxPoints samples.
func New(bounds geom.Rect, maxPoints int) *Sparkline {
	s := &Sparkline{
		Base:      views.NewBase(bounds),
		MaxPoints: maxPoints,
		Min:       0,
		Max:       1,
		AutoScale: true,
		Color:     theme.Get().ChartBar2,
	}
	s.SetSelf(s)
	return s
}

// GetTypeID for serial registry.
func (s *Sparkline) GetTypeID() string { return "sparkline" }

// Push appends v, dropping the oldest sample if MaxPoints is exceeded.
func (s *Sparkline) Push(v float64) {
	s.Values = append(s.Values, v)
	if s.MaxPoints > 0 && len(s.Values) > s.MaxPoints {
		s.Values = s.Values[len(s.Values)-s.MaxPoints:]
	}
}

// SetValues replaces the entire series.
func (s *Sparkline) SetValues(v []float64) {
	s.Values = append([]float64(nil), v...)
	if s.MaxPoints > 0 && len(s.Values) > s.MaxPoints {
		s.Values = s.Values[len(s.Values)-s.MaxPoints:]
	}
}

// Draw paints the sparkline over the topmost row of bounds.
func (s *Sparkline) Draw() {
	min, max := s.Min, s.Max
	if s.AutoScale && len(s.Values) > 0 {
		min, max = s.Values[0], s.Values[0]
		for _, v := range s.Values {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
		if max == min {
			max = min + 1
		}
	}
	w := s.Size.X
	buf := screen.MakeDrawBuffer(w)
	// Right-align the most recent samples.
	start := len(s.Values) - w
	if start < 0 {
		start = 0
	}
	offset := w - (len(s.Values) - start)
	for x := 0; x < offset; x++ {
		screen.DrawCell(buf, x, " ", s.Color)
	}
	for i, v := range s.Values[start:] {
		level := int((v - min) * 8 / (max - min))
		if level < 0 {
			level = 0
		}
		if level > 8 {
			level = 8
		}
		buf[offset+i] = types.DrawCell{Ch: string(blockChars[level]), Attr: s.Color}
	}
	for y := 0; y < s.Size.Y; y++ {
		s.WriteLine(0, y, w, 1, buf)
	}
}
