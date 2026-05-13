// Package asciitab provides ASCIIChart — a 16×16 grid of the first
// 256 codepoints, useful for picking a glyph or just for reference.
package asciitab

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// ASCIIChart paints a 16-column × 16-row grid of code points 0..255.
// Click or arrow-keys move the focused cell; Enter dispatches a
// "selected" event with InfoInt = code point.
type ASCIIChart struct {
	views.Base

	Selected int // 0..255

	Grid, FocusColor uint16
}

// New constructs an ASCIIChart.
func New(bounds geom.Rect) *ASCIIChart {
	c := &ASCIIChart{
		Base:       views.NewBase(bounds),
		Grid:       theme.Get().AsciiTabInactive,
		FocusColor: theme.Get().InputArrow,
	}
	c.SetSelf(c)
	c.Options |= consts.OfSelectable | consts.OfFirstClick
	c.State |= consts.SfCursorVis
	return c
}

// GetTypeID for serial registry.
func (c *ASCIIChart) GetTypeID() string { return "asciichart" }

// Draw paints the grid (16x16 cells, each 2 wide for spacing).
func (c *ASCIIChart) Draw() {
	for y := 0; y < 16 && y < c.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(c.Size.X)
		for x := 0; x < c.Size.X; x++ {
			screen.DrawCell(buf, x, " ", c.Grid)
		}
		for x := 0; x < 16; x++ {
			cp := y*16 + x
			ch := drawableChar(cp)
			attr := c.Grid
			if cp == c.Selected {
				attr = c.FocusColor
			}
			pos := x * 2
			if pos+1 < c.Size.X {
				buf[pos] = types.DrawCell{Ch: string(ch), Attr: attr}
				buf[pos+1] = types.DrawCell{Ch: " ", Attr: c.Grid}
			}
		}
		c.WriteLine(0, y, c.Size.X, 1, buf)
	}
}

// drawableChar returns a printable substitute for control characters.
func drawableChar(cp int) rune {
	if cp < 0x20 || cp == 0x7F {
		return '·'
	}
	return rune(cp)
}

// HandleEvent: arrows move selection; Enter / double-click commits.
func (c *ASCIIChart) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		local := c.MakeLocal(ev.Where)
		col := local.X / 2
		row := local.Y
		if col >= 0 && col < 16 && row >= 0 && row < 16 {
			c.Selected = row*16 + col
			if ev.DoubleClk {
				c.commit()
			}
			c.Draw()
			c.ClearEvent(ev)
		}
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	row, col := c.Selected/16, c.Selected%16
	switch ev.KeyCode {
	case consts.KbLeft:
		if col > 0 {
			col--
		}
	case consts.KbRight:
		if col < 15 {
			col++
		}
	case consts.KbUp:
		if row > 0 {
			row--
		}
	case consts.KbDown:
		if row < 15 {
			row++
		}
	case consts.KbHome:
		col = 0
	case consts.KbEnd:
		col = 15
	case consts.KbEnter:
		c.commit()
		c.ClearEvent(ev)
		return
	default:
		return
	}
	c.Selected = row*16 + col
	c.Draw()
	c.ClearEvent(ev)
}

func (c *ASCIIChart) commit() {
	notify := drivers.Event{
		What:    consts.EvBroadcast,
		Command: consts.CmListItemSelected,
		InfoInt: int16(c.Selected),
	}
	c.PutEvent(&notify)
}

// Show opens an ASCIIChart inside a dialog and returns the chosen
// code point or -1 on cancel.
func Show(host *views.Group) int {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 38, 22), "ASCII Chart")
	chart := New(geom.NewRect(2, 2, 34, 18))
	d.Insert(chart)
	d.Insert(dialogs.NewButton(geom.NewRect(8, 19, 18, 20), "O~K~", consts.CmOK, dialogs.BfDefault))
	d.Insert(dialogs.NewButton(geom.NewRect(20, 19, 30, 20), "Cancel", consts.CmCancel, 0))
	if host.ExecView(d) == consts.CmOK {
		return chart.Selected
	}
	return -1
}

var _ = fmt.Sprint // keep import alive
