// Package barchart provides BarChart — a simple labeled bar chart
// (horizontal bars only, with auto-scaling and value annotations).
package barchart

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Bar is a labeled value.
type Bar struct {
	Label string
	Value float64
	Attr  uint16 // 0 = use default
}

// BarChart renders a stack of horizontal bars filling a column block.
// Label is left-aligned, then the bar fills the remaining width
// proportional to Value / max(Values), then a numeric annotation.
type BarChart struct {
	views.Base

	Bars      []Bar
	ShowValue bool
	BarChar   rune
	BackChar  rune
	BarAttr   uint16
	BackAttr  uint16
	LabelAttr uint16
	ValueAttr uint16
}

// New constructs a BarChart with no bars.
func New(bounds geom.Rect) *BarChart {
	b := &BarChart{
		Base:      views.NewBase(bounds),
		ShowValue: true,
		BarChar:   '█',
		BackChar:  '░',
		BarAttr:   types.MakeAttr(0x0B, 0x01),
		BackAttr:  types.MakeAttr(0x08, 0x01),
		LabelAttr: types.MakeAttr(0x0F, 0x01),
		ValueAttr: types.MakeAttr(0x0E, 0x01),
	}
	b.SetSelf(b)
	return b
}

// GetTypeID for serial registry.
func (b *BarChart) GetTypeID() string { return "barchart" }

// SetBars replaces the data.
func (b *BarChart) SetBars(bars []Bar) { b.Bars = bars }

// Draw paints the bars.
func (b *BarChart) Draw() {
	maxVal := 0.0
	for _, br := range b.Bars {
		if br.Value > maxVal {
			maxVal = br.Value
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}
	const labelWidth = 12
	const valueWidth = 8
	for y := 0; y < b.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(b.Size.X)
		for x := 0; x < b.Size.X; x++ {
			screen.DrawCell(buf, x, " ", b.LabelAttr)
		}
		if y < len(b.Bars) {
			br := b.Bars[y]
			label := br.Label
			if len(label) > labelWidth-1 {
				label = label[:labelWidth-1]
			}
			screen.DrawStr(buf, 0, label, b.LabelAttr)
			barAttr := b.BarAttr
			if br.Attr != 0 {
				barAttr = br.Attr
			}
			barArea := b.Size.X - labelWidth - valueWidth
			if barArea < 1 {
				barArea = 1
			}
			fill := int(br.Value * float64(barArea) / maxVal)
			if fill < 0 {
				fill = 0
			}
			if fill > barArea {
				fill = barArea
			}
			for x := 0; x < barArea; x++ {
				ch := b.BackChar
				attr := b.BackAttr
				if x < fill {
					ch = b.BarChar
					attr = barAttr
				}
				if labelWidth+x < b.Size.X {
					buf[labelWidth+x] = types.DrawCell{Ch: string(ch), Attr: attr}
				}
			}
			if b.ShowValue {
				v := fmt.Sprintf("%6.1f", br.Value)
				screen.DrawStr(buf, b.Size.X-valueWidth, v, b.ValueAttr)
			}
		}
		b.WriteLine(0, y, b.Size.X, 1, buf)
	}
}
