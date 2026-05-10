// Package colortxt provides ColoredText, a StaticText variant that
// paints with a caller-supplied attribute byte instead of the default
// dialog body color. Ported from ColorTxt.pas.
package colortxt

import (
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
)

// ColoredText is a StaticText with an explicit (fg, bg) palette.
type ColoredText struct {
	dialogs.StaticText
	Attr uint16 // packed FG/BG (use types.MakeAttr)
}

// New builds a ColoredText with the given bounds, text, and color
// attribute. The attribute is uint16 in the standard MakeAttr layout
// (low byte FG, high byte BG).
func New(bounds geom.Rect, text string, attr uint16) *ColoredText {
	c := &ColoredText{
		StaticText: *dialogs.NewStaticText(bounds, text),
		Attr:       attr,
	}
	c.SetSelf(c)
	return c
}

// GetTypeID for the serial registry.
func (c *ColoredText) GetTypeID() string { return "coloredtext" }

// Draw paints text with the per-instance attribute. Mirrors the line-
// breaking / centering behavior of TStaticText: '\n' starts a new line,
// a leading '\x03' centers the line.
func (c *ColoredText) Draw() {
	color := c.Attr
	y := 0
	start := 0
	text := c.Text
	flush := func(line string) {
		buf := screen.MakeDrawBuffer(c.Size.X)
		for x := 0; x < c.Size.X; x++ {
			screen.DrawCell(buf, x, " ", color)
		}
		x := 0
		if len(line) > 0 && line[0] == '\x03' {
			line = line[1:]
			x = (c.Size.X - len(line)) / 2
			if x < 0 {
				x = 0
			}
		}
		screen.DrawStr(buf, x, line, color)
		c.WriteLine(0, y, c.Size.X, 1, buf)
		y++
	}
	for i := 0; i <= len(text); i++ {
		if i == len(text) || text[i] == '\n' {
			if y >= c.Size.Y {
				return
			}
			flush(text[start:i])
			start = i + 1
		}
	}
	// Pad remaining rows with the chosen color.
	for ; y < c.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(c.Size.X)
		for x := 0; x < c.Size.X; x++ {
			screen.DrawCell(buf, x, " ", color)
		}
		c.WriteLine(0, y, c.Size.X, 1, buf)
	}
	_ = types.MakeAttr // keep import alive
}
