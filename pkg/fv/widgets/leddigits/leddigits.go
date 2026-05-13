// Package leddigits provides LEDDigits — a 7-segment numeric display
// rendered with ASCII '_' and '|'.
package leddigits

import (
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// segmentPatterns: bit 0 top, 1 top-right, 2 bot-right, 3 bottom,
// 4 bot-left, 5 top-left, 6 middle. Standard 7-seg LUT for 0..9.
var segmentPatterns = [10]byte{
	0x3F, // 0
	0x06, // 1
	0x5B, // 2
	0x4F, // 3
	0x66, // 4
	0x6D, // 5
	0x7D, // 6
	0x07, // 7
	0x7F, // 8
	0x6F, // 9
}

// LEDDigits displays an integer as N 3-row 7-segment digits.
type LEDDigits struct {
	views.Base

	Value        int64
	DigitCount   int
	LeadingZeros bool
	Color        uint16
}

// New constructs an LED display showing digitCount digits.
func New(bounds geom.Rect, digitCount int) *LEDDigits {
	l := &LEDDigits{
		Base:       views.NewBase(bounds),
		DigitCount: digitCount,
		Color:      theme.Get().LedDigit,
	}
	l.SetSelf(l)
	return l
}

// GetTypeID for serial registry.
func (l *LEDDigits) GetTypeID() string { return "leddigits" }

// SetValue updates the displayed number.
func (l *LEDDigits) SetValue(v int64) { l.Value = v }

// Draw paints up to 3 rows of segment characters.
func (l *LEDDigits) Draw() {
	digits := make([]int, l.DigitCount)
	v := l.Value
	if v < 0 {
		v = -v
	}
	for d := l.DigitCount - 1; d >= 0; d-- {
		digits[d] = int(v % 10)
		v /= 10
	}
	maxRows := 3
	if maxRows > l.Size.Y {
		maxRows = l.Size.Y
	}
	for row := 0; row < maxRows; row++ {
		buf := screen.MakeDrawBuffer(l.Size.X)
		for x := 0; x < l.Size.X; x++ {
			screen.DrawCell(buf, x, " ", l.Color)
		}
		x := 0
		nonzeroSeen := false
		for d := 0; d < l.DigitCount && x+3 <= l.Size.X; d++ {
			blank := false
			if !l.LeadingZeros && d < l.DigitCount-1 && !nonzeroSeen && digits[d] == 0 {
				blank = true
			} else if digits[d] != 0 {
				nonzeroSeen = true
			}
			c1, c2, c3 := ' ', ' ', ' '
			if !blank {
				seg := segmentPatterns[digits[d]]
				switch row {
				case 0:
					if seg&0x01 != 0 {
						c2 = '_'
					}
				case 1:
					if seg&0x20 != 0 {
						c1 = '|'
					}
					if seg&0x40 != 0 {
						c2 = '_'
					}
					if seg&0x02 != 0 {
						c3 = '|'
					}
				case 2:
					if seg&0x10 != 0 {
						c1 = '|'
					}
					if seg&0x08 != 0 {
						c2 = '_'
					}
					if seg&0x04 != 0 {
						c3 = '|'
					}
				}
			}
			buf[x] = types.DrawCell{Ch: string(c1), Attr: l.Color}
			buf[x+1] = types.DrawCell{Ch: string(c2), Attr: l.Color}
			buf[x+2] = types.DrawCell{Ch: string(c3), Attr: l.Color}
			x += 4 // 3 chars + 1 space gap
		}
		l.WriteLine(0, row, l.Size.X, 1, buf)
	}
}
