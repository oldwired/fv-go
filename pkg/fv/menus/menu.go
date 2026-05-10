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
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Item is a single menu entry. Submenu, command, or separator.
type Item struct {
	Name     string // text, '~' marks the hotkey letter
	Command  uint16 // 0 if Sub != nil (i.e., a submenu)
	HelpCtx  uint16
	Disabled bool
	Sub      *Menu // non-nil if this is a submenu
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
	// Black on light-gray; hotkey letter rendered red on the same bg.
	normal := types.MakeAttr(0x00, 0x07)
	hot := types.MakeAttr(0x04, 0x07)
	// Highlighted item flips to white-on-green, with bright-yellow hot.
	highlightNormal := types.MakeAttr(0x0F, 0x02)
	highlightHot := types.MakeAttr(0x0E, 0x02)

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

func (m *MenuBar) openSubmenu(idx int) {
	if idx < 0 || idx >= len(m.Menu.Items) {
		return
	}
	it := m.Menu.Items[idx]
	if it.Sub == nil {
		return
	}
	if m.Owner == nil {
		return
	}
	m.hotIndex = idx

	// Position the popup directly under the menu-bar item.
	x := 1
	for i := 0; i < idx; i++ {
		x += utf8.CStrDisplayWidth(" "+m.Menu.Items[i].Name+" ") + 1
	}
	mb := NewMenuBox(geom.Point{X: x, Y: 1}, it.Sub)
	cmd := mb.runIn(m.Owner)
	m.hotIndex = -1
	if cmd != 0 {
		ev := drivers.Event{What: consts.EvCommand, Command: cmd}
		m.PutEvent(&ev)
	}
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
