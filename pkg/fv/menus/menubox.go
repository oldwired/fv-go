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

	Menu     *Menu
	current  int
	topLevel bool // true when this popup is directly under the menu bar; controls Left/Right cycling vs. close-one-level
}

// menuResult is what runIn returns to its caller. Exactly one of (cmd,
// nav) is non-zero, or both are zero meaning "cancelled at this level".
// Nav is only emitted by a top-level popup — nested levels swallow
// Left/Right into "close one level" and "open nested" respectively.
type menuResult struct {
	cmd uint16
	nav int // -1=move to previous top-level menu, +1=move to next
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
	hasSubmenu := false
	for _, it := range m.Items {
		c := utf8.CStrDisplayWidth(it.Name)
		if c > w {
			w = c
		}
		if it.IsSubmenu() {
			hasSubmenu = true
		}
	}
	// Borders + left padding (+ extra room for the "▶" submenu marker).
	w += 4
	if hasSubmenu {
		w += 2
	}
	h = len(m.Items) + 2
	return
}

// runIn opens the popup as a child of host, then runs a modal-style
// loop until the user picks an item or cancels. Returns the result —
// see menuResult.
func (mb *MenuBox) runIn(host *views.Group) menuResult {
	mb.current = 0
	host.Insert(mb)
	defer host.Delete(mb)

	q := views.GetEventQueue()
	if q == nil {
		return menuResult{}
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
		if res, done := mb.handleKey(&ev); done {
			return res
		}
		views.MarkDirty()
	}
}

// handleKey processes one event. Returns (result, true) when the popup
// should close. Items with a non-nil Sub recursively open a nested
// MenuBox to the right of the current row.
func (mb *MenuBox) handleKey(ev *drivers.Event) (menuResult, bool) {
	if ev.What == consts.EvMouseDown {
		local := mb.MakeLocal(ev.Where)
		if local.Y >= 1 && local.Y-1 < len(mb.Menu.Items) &&
			local.X > 0 && local.X < mb.Size.X-1 {
			it := mb.Menu.Items[local.Y-1]
			if !it.IsSeparator() && !it.Disabled {
				mb.current = local.Y - 1
				return mb.activate()
			}
			return menuResult{}, false
		}
		// Click outside the popup → cancel.
		return menuResult{}, true
	}
	if ev.What != consts.EvKeyDown {
		return menuResult{}, false
	}
	switch ev.KeyCode {
	case consts.KbEsc:
		return menuResult{}, true
	case consts.KbUp:
		mb.move(-1)
		return menuResult{}, false
	case consts.KbDown:
		mb.move(1)
		return menuResult{}, false
	case consts.KbHome:
		mb.current = 0
		return menuResult{}, false
	case consts.KbEnd:
		mb.current = len(mb.Menu.Items) - 1
		return menuResult{}, false
	case consts.KbLeft:
		// Top-level popup: signal "open the previous top-level menu".
		// Nested popup: close one level (parent loop resumes / re-displays).
		if mb.topLevel {
			return menuResult{nav: -1}, true
		}
		return menuResult{}, true
	case consts.KbRight:
		// If the focused item is a submenu, open it regardless of level.
		if it := mb.activeItem(); it != nil && it.IsSubmenu() {
			return mb.activate()
		}
		// Otherwise, top-level cycles to the next top-level menu.
		// Nested popups ignore Right on a leaf.
		if mb.topLevel {
			return menuResult{nav: +1}, true
		}
		return menuResult{}, false
	case consts.KbEnter:
		return mb.activate()
	}
	if ev.UnicodeChar != 0 {
		if it := mb.matchHotkey(byte(ev.UnicodeChar)); it != nil {
			// Move current onto the matched item so activate() sees it.
			for i, x := range mb.Menu.Items {
				if x == it {
					mb.current = i
					break
				}
			}
			return mb.activate()
		}
	}
	return menuResult{}, false
}

// activate is the "user picked the focused item" path — fires a
// command for leaf items, opens a nested MenuBox for submenus.
func (mb *MenuBox) activate() (menuResult, bool) {
	it := mb.activeItem()
	if it == nil || it.Disabled || it.IsSeparator() {
		return menuResult{}, false
	}
	if it.IsSubmenu() {
		res := mb.openNestedSubmenu(mb.current)
		// If the nested popup picked a command, the chain unwinds.
		// Nav signals from nested levels are absorbed here (Left in a
		// nested submenu just closes that level — the parent loop
		// resumes with our popup still visible).
		if res.cmd != 0 {
			return res, true
		}
		return menuResult{}, false
	}
	return menuResult{cmd: it.Command}, true
}

// openNestedSubmenu opens a MenuBox for items[idx].Sub anchored to the
// right of this popup at the row of the parent item, runs its modal
// loop, and returns whatever command came back.
func (mb *MenuBox) openNestedSubmenu(idx int) menuResult {
	if mb.Owner == nil {
		return menuResult{}
	}
	parent := mb.Menu.Items[idx]
	x := mb.Origin.X + mb.Size.X - 1
	y := mb.Origin.Y + idx + 1
	sub := NewMenuBox(geom.Point{X: x, Y: y}, parent.Sub)
	// Nested popups are never top-level: their Left closes back to us,
	// their Right opens deeper but does not cycle top-level.
	sub.topLevel = false
	return sub.runIn(mb.Owner)
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
			if it.IsSubmenu() && w >= 4 {
				screen.DrawCell(row, w-2, "▶", n)
			}
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
