// Package menus ports Menus.pas: MenuBar, MenuBox (popup), StatusLine.
//
// MenuItem and Menu mirror the Pascal record + circular-list pattern,
// flattened into a slice tree (each Item has .Sub for nested items).
package menus

import (
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Item is a single menu entry. Submenu, command, or separator.
//
// Shortcut, when non-empty, renders right-aligned on the item row in
// the dim palette role — a discoverability hint for the chord key
// the user can press instead of navigating into the menu. (The string
// is purely cosmetic; bind the actual chord elsewhere.)
type Item struct {
	Name     string // text, '~' marks the hotkey letter
	Command  uint16 // 0 if Sub != nil (i.e., a submenu)
	HelpCtx  uint16
	Disabled bool
	Shortcut string // optional right-aligned hint, e.g. "Ctrl+S"
	Sub      *Menu  // non-nil if this is a submenu
}

// Separator returns a horizontal-rule menu item.
func Separator() *Item { return &Item{Name: ""} }

// IsSeparator reports whether this is a divider line.
func (i *Item) IsSeparator() bool { return i.Name == "" && i.Sub == nil && i.Command == 0 }

// IsSubmenu reports whether this is a submenu.
func (i *Item) IsSubmenu() bool { return i.Sub != nil }

// Menu is a list of items.
type Menu struct {
	Items []*Item
}

// NewMenu builds a Menu from items. Convenience.
func NewMenu(items ...*Item) *Menu { return &Menu{Items: items} }

// MenuBar is a horizontal one-line menu at the top of the program.
type MenuBar struct {
	views.Base

	Menu     *Menu
	hotIndex int // currently highlighted top-level item, -1 = none
}

// NewMenuBar builds a horizontal menu bar with the given menu tree.
// bounds is typically (0,0)-(cols,1).
func NewMenuBar(bounds geom.Rect, menu *Menu) *MenuBar {
	m := &MenuBar{Base: views.NewBase(bounds), Menu: menu, hotIndex: -1}
	m.SetSelf(m)
	m.GrowMode = consts.GfGrowHiX
	m.Options = consts.OfPreProcess
	m.EventMask = consts.EvMouseDown | consts.EvKeyDown | consts.EvCommand
	return m
}

// GetTypeID for serial registry.
func (m *MenuBar) GetTypeID() string { return "menubar" }

// Draw paints the bar.
func (m *MenuBar) Draw() {
	pal := theme.Get()
	normal := pal.MenuBarNormal
	hot := pal.MenuBarHot
	highlightNormal := pal.MenuItemSelected
	highlightHot := pal.MenuItemSelectedHot

	buf := screen.MakeDrawBuffer(m.Size.X)
	for x := 0; x < m.Size.X; x++ {
		screen.DrawCell(buf, x, " ", normal)
	}
	x := 1
	for i, it := range m.Menu.Items {
		label := " " + it.Name + " "
		n, h := normal, hot
		if i == m.hotIndex {
			n, h = highlightNormal, highlightHot
		}
		screen.DrawCStr(buf, x, label, n, h)
		x += utf8.CStrDisplayWidth(label) + 1
		if x >= m.Size.X {
			break
		}
	}
	m.WriteLine(0, 0, m.Size.X, 1, buf)
}

// HandleEvent reacts to:
//   - F10 → open the first submenu (mirrors TV behavior)
//   - Alt+letter → open the submenu whose hotkey matches
//   - cmMenu broadcast → also opens the first submenu (so other
//     widgets, e.g. the StatusLine F10 shortcut, can activate it)
//   - mouse-down on a top-level item → open that submenu.
func (m *MenuBar) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvCommand && ev.Command == consts.CmMenu {
		m.openSubmenu(0)
		m.ClearEvent(ev)
		return
	}
	if ev.What == consts.EvKeyDown {
		if ev.KeyShift&consts.KbAltShift != 0 {
			letter := byte(ev.UnicodeChar)
			if letter == 0 {
				letter = byte(ev.KeyCode & 0xFF)
			}
			if i := m.findHotKey(letter); i >= 0 {
				m.openSubmenu(i)
				m.ClearEvent(ev)
				return
			}
		}
		if ev.KeyCode == consts.KbF10 {
			m.openSubmenu(0)
			m.ClearEvent(ev)
			return
		}
	}
	if ev.What == consts.EvMouseDown {
		local := m.MakeLocal(ev.Where)
		if local.Y == 0 {
			x := 1
			for i, it := range m.Menu.Items {
				w := utf8.CStrDisplayWidth(" " + it.Name + " ")
				if local.X >= x && local.X < x+w {
					m.openSubmenu(i)
					m.ClearEvent(ev)
					return
				}
				x += w + 1
			}
		}
	}
}

func (m *MenuBar) findHotKey(letter byte) int {
	for i, it := range m.Menu.Items {
		if it.Sub == nil {
			continue
		}
		hot := hotkeyOf(it.Name)
		if hot == 0 {
			continue
		}
		if equalIgnoreCase(hot, letter) {
			return i
		}
	}
	return -1
}

// openSubmenu opens the top-level popup for items[idx]. While that
// popup runs it may bubble a nav result asking to move to the previous
// or next top-level menu — we loop here, closing the current popup and
// opening the adjacent one, until the user either picks a command or
// presses Esc (nav=0, cmd=0).
//
// Items without a submenu (rare in a real menu bar but legal) are
// skipped during navigation so Left/Right always lands on something
// that can open.
func (m *MenuBar) openSubmenu(idx int) {
	if m.Owner == nil {
		return
	}
	idx = m.nextSubmenu(idx, 0)
	if idx < 0 {
		return
	}
	for {
		it := m.Menu.Items[idx]
		m.hotIndex = idx

		x := 1
		for i := 0; i < idx; i++ {
			x += utf8.CStrDisplayWidth(" "+m.Menu.Items[i].Name+" ") + 1
		}
		mb := NewMenuBox(geom.Point{X: x, Y: 1}, it.Sub)
		mb.topLevel = true
		res := mb.runIn(m.Owner)
		m.hotIndex = -1

		if res.cmd != 0 {
			ev := drivers.Event{What: consts.EvCommand, Command: res.cmd}
			m.PutEvent(&ev)
			return
		}
		if res.nav == 0 {
			return
		}
		next := m.nextSubmenu(idx, res.nav)
		if next < 0 || next == idx {
			return
		}
		idx = next
	}
}

// nextSubmenu finds the nearest item with a non-nil Sub, starting from
// idx and stepping by dir (-1 / +1). With dir=0 it returns idx itself
// if that item has a submenu, otherwise scans forward. Wraps around at
// the ends. Returns -1 if no submenu items exist at all.
func (m *MenuBar) nextSubmenu(idx, dir int) int {
	n := len(m.Menu.Items)
	if n == 0 {
		return -1
	}
	if dir == 0 {
		if idx >= 0 && idx < n && m.Menu.Items[idx].Sub != nil {
			return idx
		}
		dir = 1
	}
	for tries := 0; tries < n; tries++ {
		idx = (idx + dir + n) % n
		if m.Menu.Items[idx].Sub != nil {
			return idx
		}
	}
	return -1
}

// hotkeyOf returns the byte after the first '~' in s, or 0 if none.
func hotkeyOf(s string) byte {
	idx := strings.Index(s, "~")
	if idx < 0 || idx+1 >= len(s) {
		return 0
	}
	return s[idx+1]
}

func equalIgnoreCase(a, b byte) bool {
	if a >= 'A' && a <= 'Z' {
		a += 32
	}
	if b >= 'A' && b <= 'Z' {
		b += 32
	}
	return a == b
}
