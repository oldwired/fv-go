// Package colorsel provides ColorSelector — a 16-color palette picker
// laid out as a 4×4 grid — and ShowDialog, a "pick a color" wrapper.
//
// The original Pascal unit also includes a monochrome attribute picker
// and a grouped "color set" editor; this port covers the common case
// (pick one of the 16 CGA colors). The other selectors can be added
// in a follow-up if the simpler picker isn't enough for a given app.
package colorsel

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// CGA color names indexed 0..15.
var ColorNames = [16]string{
	"Black", "Blue", "Green", "Cyan", "Red", "Magenta", "Brown", "Gray",
	"DkGray", "BrBlue", "BrGreen", "BrCyan", "BrRed", "BrMag", "Yellow", "White",
}

// ColorSelector is a 4×4 grid of color swatches.
type ColorSelector struct {
	views.Base

	Selected byte // 0..15
}

// New constructs a selector starting on the given color.
func New(bounds geom.Rect, initial byte) *ColorSelector {
	c := &ColorSelector{Base: views.NewBase(bounds), Selected: initial}
	c.SetSelf(c)
	c.Options |= consts.OfSelectable | consts.OfFirstClick
	c.State |= consts.SfCursorVis
	return c
}

// GetTypeID for serial registry.
func (c *ColorSelector) GetTypeID() string { return "colorselector" }

// swatch dimensions; tiles are adjacent (no inter-swatch gap).
const (
	swatchWidth  = 5
	swatchHeight = 2
)

// Draw paints a 4×4 grid of color tiles plus a one-line caption.
func (c *ColorSelector) Draw() {
	for row := 0; row < 4; row++ {
		for sub := 0; sub < swatchHeight; sub++ {
			y := row*swatchHeight + sub
			if y >= c.Size.Y {
				break
			}
			buf := screen.MakeDrawBuffer(c.Size.X)
			for col := 0; col < 4; col++ {
				color := byte(row*4 + col)
				x := col * swatchWidth
				// Pick a contrasting FG so the selection marker stays
				// readable on bright/dim backgrounds.
				fg := byte(0x00)
				if color < 8 {
					fg = 0x0F
				}
				attr := types.MakeAttr(fg, color)
				for i := 0; i < swatchWidth && x+i < c.Size.X; i++ {
					ch := " "
					if color == c.Selected && sub == 0 && i == swatchWidth/2 {
						ch = "◆"
					}
					buf[x+i] = types.DrawCell{Ch: ch, Attr: attr}
				}
			}
			c.WriteLine(0, y, c.Size.X, 1, buf)
		}
	}
	footY := 4 * swatchHeight
	if footY < c.Size.Y {
		row := screen.MakeDrawBuffer(c.Size.X)
		caption := types.MakeAttr(0x0F, 0x01)
		for x := 0; x < c.Size.X; x++ {
			screen.DrawCell(row, x, " ", caption)
		}
		text := fmt.Sprintf(" #%2d  %s", c.Selected, ColorNames[c.Selected])
		screen.DrawStr(row, 0, text, caption)
		c.WriteLine(0, footY, c.Size.X, 1, row)
	}
}

// HandleEvent: arrows + click pick a swatch.
func (c *ColorSelector) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		local := c.MakeLocal(ev.Where)
		col := local.X / swatchWidth
		row := local.Y / swatchHeight
		if col >= 0 && col < 4 && row >= 0 && row < 4 {
			c.Selected = byte(row*4 + col)
			c.Draw()
			c.ClearEvent(ev)
		}
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	row, col := int(c.Selected)/4, int(c.Selected)%4
	switch ev.KeyCode {
	case consts.KbLeft:
		if col > 0 {
			col--
		}
	case consts.KbRight:
		if col < 3 {
			col++
		}
	case consts.KbUp:
		if row > 0 {
			row--
		}
	case consts.KbDown:
		if row < 3 {
			row++
		}
	default:
		return
	}
	c.Selected = byte(row*4 + col)
	c.Draw()
	c.ClearEvent(ev)
}

// ShowDialog runs the picker; returns the chosen color and true on OK.
func ShowDialog(host *views.Group, initial byte) (byte, bool) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 30, 12), "Choose Color")
	sel := New(geom.NewRect(2, 2, 22, 8), initial)
	d.Insert(sel)
	d.Insert(dialogs.NewButton(geom.NewRect(4, 9, 14, 10), "O~K~", consts.CmOK, dialogs.BfDefault))
	d.Insert(dialogs.NewButton(geom.NewRect(16, 9, 26, 10), "Cancel", consts.CmCancel, 0))
	if host.ExecView(d) == consts.CmOK {
		return sel.Selected, true
	}
	return initial, false
}
