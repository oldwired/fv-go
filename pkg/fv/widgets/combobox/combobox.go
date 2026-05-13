// Package combobox provides ComboBox — an InputLine with a dropdown
// arrow that opens a PopupMenu of preset choices. The user can also
// type freely.
package combobox

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/popupmenu"
)

// ComboBox is a single-line input with a "▾" affordance on its right
// edge. Clicking the affordance (or pressing F4) opens a dropdown.
type ComboBox struct {
	dialogs.InputLine

	Items []string
}

// New constructs a combo box with the given list of choices.
func New(bounds geom.Rect, items []string, maxLen int) *ComboBox {
	c := &ComboBox{InputLine: *dialogs.NewInputLine(bounds, maxLen), Items: items}
	c.SetSelf(c)
	return c
}

// GetTypeID for serial registry.
func (c *ComboBox) GetTypeID() string { return "combobox" }

// SetItems replaces the dropdown items.
func (c *ComboBox) SetItems(items []string) { c.Items = items }

// Draw delegates to InputLine then overlays the dropdown arrow.
func (c *ComboBox) Draw() {
	c.InputLine.Draw()
	if c.Size.X >= 1 {
		pal := theme.Get()
		arrow := pal.InputUnfocused
		if c.GetState(consts.SfFocused) {
			arrow = pal.ComboButton
		}
		row := screen.DrawBuffer{{Ch: "▾", Attr: arrow}}
		c.WriteLine(c.Size.X-1, 0, 1, 1, row)
	}
}

// HandleEvent: F4 / Alt+Down / click on the arrow opens the dropdown.
// Otherwise behaves like an InputLine.
func (c *ComboBox) HandleEvent(ev *drivers.Event) {
	open := false
	if ev.What == consts.EvKeyDown {
		if ev.KeyCode == consts.KbAltDown {
			open = true
		}
		if ev.KeyCode == 0x3E00 /* F4 */ {
			open = true
		}
	}
	if ev.What == consts.EvMouseDown {
		local := c.MakeLocal(ev.Where)
		if local.Y == 0 && local.X == c.Size.X-1 {
			open = true
		}
	}
	if open {
		c.openDropdown()
		c.ClearEvent(ev)
		return
	}
	c.InputLine.HandleEvent(ev)
}

func (c *ComboBox) openDropdown() {
	if c.Owner == nil {
		return
	}
	host := topLevelGroup(c.Owner)
	if host == nil {
		return
	}
	x, y := c.ScreenOrigin()
	hx, hy := host.ScreenOrigin()
	pop := popupmenu.New(geom.Point{X: x - hx, Y: y - hy + 1}, c.Items, c.Size.X+8)
	idx := pop.Run(host)
	if idx >= 0 && idx < len(c.Items) {
		c.SetText(c.Items[idx])
	}
}

// topLevelGroup walks up from g to the outermost Group (the Program).
// PopupMenu.Run wants to insert into the program so it can paint
// over everything else, including modal dialogs.
func topLevelGroup(g *views.Group) *views.Group {
	for g != nil && g.Owner != nil {
		g = g.Owner
	}
	return g
}
