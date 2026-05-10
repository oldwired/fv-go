package menus

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// MenuBox is a popup menu that draws as a regular child of whatever
// group it's inserted into. The MenuBar inserts a MenuBox into its
// own owner (the Program) when a submenu is opened, then runs a
// modal-style loop that reads events and updates the box's state
// until the user picks an item or presses Esc.
type MenuBox struct {
	views.Base

	Menu    *Menu
	current int
}

// NewMenuBox builds a popup menu at the given screen position.
func NewMenuBox(origin geom.Point, menu *Menu) *MenuBox {
	w, h := menuBoxSize(menu)
	bounds := geom.NewRect(origin.X, origin.Y, origin.X+w, origin.Y+h)
	mb := &MenuBox{Base: views.NewBase(bounds), Menu: menu}
	mb.SetSelf(mb)
	mb.State |= consts.SfVisible | consts.SfExposed
	mb.Options |= consts.OfPreProcess
	return mb
}

// GetTypeID for serial registry.
func (mb *MenuBox) GetTypeID() string { return "menubox" }

func menuBoxSize(m *Menu) (w, h int) {
	w = 0
	for _, it := range m.Items {
		c := utf8.CStrDisplayWidth(it.Name)
		if c > w {
			w = c
		}
	}
	w += 4 // borders + padding
	h = len(m.Items) + 2
	return
}

// runIn opens the popup as a child of host, then runs a modal-style
// loop until the user picks an item or cancels. Returns the chosen
// command or 0.
func (mb *MenuBox) runIn(host *views.Group) uint16 {
	mb.current = 0
	host.Insert(mb)
	defer host.Delete(mb)

	q := views.GetEventQueue()
	if q == nil {
		return 0
	}
	for {
		if pump := views.GetPump(); pump != nil {
			pump()
		}
		ev, ok := q.Get()
		if !ok {
			if wait := views.GetWait(); wait != nil {
				wait()
			}
			continue
		}
		if cmd, done := mb.handleKey(&ev); done {
			return cmd
		}
	}
}

// handleKey processes one event. Returns (cmd, true) when the popup
// should close.
func (mb *MenuBox) handleKey(ev *drivers.Event) (uint16, bool) {
	if ev.What == consts.EvMouseDown {
		// Map screen coords to a row inside the box. Rows 1..len(Items)
		// correspond to items; clicks on the border or outside dismiss.
		local := mb.MakeLocal(ev.Where)
		if local.Y >= 1 && local.Y-1 < len(mb.Menu.Items) &&
			local.X > 0 && local.X < mb.Size.X-1 {
			it := mb.Menu.Items[local.Y-1]
			if !it.IsSeparator() && !it.Disabled {
				mb.current = local.Y - 1
				return it.Command, true
			}
			return 0, false
		}
		// Click outside the popup → cancel.
		return 0, true
	}
	if ev.What != consts.EvKeyDown {
		return 0, false
	}
	switch ev.KeyCode {
	case consts.KbEsc:
		return 0, true
	case consts.KbUp:
		mb.move(-1)
		return 0, false
	case consts.KbDown:
		mb.move(1)
		return 0, false
	case consts.KbHome:
		mb.current = 0
		return 0, false
	case consts.KbEnd:
		mb.current = len(mb.Menu.Items) - 1
		return 0, false
	case consts.KbEnter:
		if it := mb.activeItem(); it != nil && !it.Disabled && !it.IsSeparator() {
			return it.Command, true
		}
		return 0, false
	}
	if ev.UnicodeChar != 0 {
		if it := mb.matchHotkey(byte(ev.UnicodeChar)); it != nil {
			return it.Command, true
		}
	}
	return 0, false
}

func (mb *MenuBox) activeItem() *Item {
	if mb.current < 0 || mb.current >= len(mb.Menu.Items) {
		return nil
	}
	return mb.Menu.Items[mb.current]
}

func (mb *MenuBox) move(d int) {
	n := len(mb.Menu.Items)
	if n == 0 {
		return
	}
	for tries := 0; tries < n; tries++ {
		mb.current = (mb.current + d + n) % n
		it := mb.Menu.Items[mb.current]
		if !it.IsSeparator() && !it.Disabled {
			break
		}
	}
}

func (mb *MenuBox) matchHotkey(letter byte) *Item {
	for _, it := range mb.Menu.Items {
		if it.IsSeparator() || it.Disabled {
			continue
		}
		hot := hotkeyOf(it.Name)
		if hot != 0 && equalIgnoreCase(hot, letter) {
			return it
		}
	}
	return nil
}

// Draw renders the popup. Called by the program's draw cycle since
// MenuBox is a regular child of whatever group hosts it.
func (mb *MenuBox) Draw() {
	frame := types.MakeAttr(0x00, 0x07) // black on light gray
	normal := types.MakeAttr(0x00, 0x07)
	normalHot := types.MakeAttr(0x04, 0x07)   // red hot
	selected := types.MakeAttr(0x0F, 0x02)    // white on green
	selectedHot := types.MakeAttr(0x0E, 0x02) // bright yellow hot on green
	w, h := mb.Size.X, mb.Size.Y

	// Top border.
	top := screen.MakeDrawBuffer(w)
	screen.DrawCell(top, 0, "┌", frame)
	for i := 1; i < w-1; i++ {
		screen.DrawCell(top, i, "─", frame)
	}
	screen.DrawCell(top, w-1, "┐", frame)
	mb.WriteLine(0, 0, w, 1, top)

	for i, it := range mb.Menu.Items {
		row := screen.MakeDrawBuffer(w)
		n, hk, bg := normal, normalHot, normal
		if i == mb.current {
			n, hk, bg = selected, selectedHot, selected
		}
		screen.DrawCell(row, 0, "│", frame)
		for x := 1; x < w-1; x++ {
			screen.DrawCell(row, x, " ", bg)
		}
		if it.IsSeparator() {
			screen.DrawCell(row, 0, "├", frame)
			for x := 1; x < w-1; x++ {
				screen.DrawCell(row, x, "─", frame)
			}
			screen.DrawCell(row, w-1, "┤", frame)
		} else {
			screen.DrawCStr(row, 2, it.Name, n, hk)
		}
		if !it.IsSeparator() {
			screen.DrawCell(row, w-1, "│", frame)
		}
		mb.WriteLine(0, 1+i, w, 1, row)
	}

	bot := screen.MakeDrawBuffer(w)
	screen.DrawCell(bot, 0, "└", frame)
	for i := 1; i < w-1; i++ {
		screen.DrawCell(bot, i, "─", frame)
	}
	screen.DrawCell(bot, w-1, "┘", frame)
	mb.WriteLine(0, h-1, w, 1, bot)

	// Cast shadow: read the cell currently sitting at each shadow
	// position and rewrite it dimmed, preserving the glyph. Mirrors
	// Window.drawShadow.
	sx, sy := mb.ScreenOrigin()
	dim := func(cell types.DrawCell) types.DrawCell {
		if cell.Ch == "" {
			cell.Ch = " "
		}
		return types.DrawCell{Ch: cell.Ch, Attr: types.MakeAttr(0x08, 0x00)}
	}
	for y := 1; y <= h; y++ {
		for dx := 0; dx < 2; dx++ {
			cellX, cellY := sx+w+dx, sy+y
			out := dim(views.GetCell(cellX, cellY))
			mb.WriteLine(w+dx, y, 1, 1, screen.DrawBuffer{out})
		}
	}
	for dx := 2; dx < w+2; dx++ {
		cellX, cellY := sx+dx, sy+h
		out := dim(views.GetCell(cellX, cellY))
		mb.WriteLine(dx, h, 1, 1, screen.DrawBuffer{out})
	}
}
