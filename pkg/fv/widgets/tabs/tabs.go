// Package tabs provides Tabs — a tabbed container. Each Tab has a
// title shown in the strip across the top and a content view shown
// in the body when that tab is active.
package tabs

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Tab is a single labeled panel.
type Tab struct {
	Title string
	Body  views.View
}

// Tabs is a tab control. Row 0 is the strip; row 1 is a separator; the
// remainder of the bounds holds the active tab's body. Click a tab
// title to switch; Ctrl+Tab cycles forward.
type Tabs struct {
	views.Group

	Items   []*Tab
	current int
}

// New constructs an empty tab container.
func New(bounds geom.Rect) *Tabs {
	t := &Tabs{}
	views.InitGroup(&t.Group, bounds)
	t.SetSelf(t)
	t.Options |= consts.OfSelectable
	t.GrowMode = consts.GfGrowAll
	return t
}

// GetTypeID for serial registry.
func (t *Tabs) GetTypeID() string { return "tabs" }

// AddTab appends a tab and inserts its body into the container.
func (t *Tabs) AddTab(title string, body views.View) {
	t.Items = append(t.Items, &Tab{Title: title, Body: body})
	if body != nil {
		t.Insert(body)
	}
	if len(t.Items) == 1 {
		t.current = 0
	}
	t.layout()
}

// DeleteTab removes the i-th tab and its body. Adjusts current.
func (t *Tabs) DeleteTab(i int) {
	if i < 0 || i >= len(t.Items) {
		return
	}
	tab := t.Items[i]
	if tab.Body != nil {
		t.Delete(tab.Body)
	}
	t.Items = append(t.Items[:i], t.Items[i+1:]...)
	if t.current >= len(t.Items) {
		t.current = len(t.Items) - 1
	}
	if t.current < 0 {
		t.current = 0
	}
	t.layout()
}

// Current returns the active tab's index.
func (t *Tabs) Current() int { return t.current }

// SetCurrent activates the i-th tab.
func (t *Tabs) SetCurrent(i int) {
	if i < 0 || i >= len(t.Items) {
		return
	}
	t.current = i
	t.layout()
	if body := t.Items[i].Body; body != nil {
		t.Focus(body)
	}
}

// layout positions each body in the area enclosed by the tab frame.
// The frame occupies row 1 (top, with the active tab "broken"
// through), columns 0 and Size.X-1 (sides), and row Size.Y-1
// (bottom). The body inset is therefore (1, 2)–(Size.X-1, Size.Y-1).
func (t *Tabs) layout() {
	body := geom.NewRect(1, 2, t.Size.X-1, t.Size.Y-1)
	for i, tab := range t.Items {
		if tab.Body == nil {
			continue
		}
		if i == t.current {
			tab.Body.BaseView().State |= consts.SfVisible
			tab.Body.ChangeBounds(body)
		} else {
			tab.Body.BaseView().State &^= consts.SfVisible
		}
	}
}

// labelWidth returns the on-screen width of a tab label, including the
// padding the strip layout uses (one space on each side).
func labelWidth(title string) int {
	return utf8.CStrDisplayWidth(title) + 2
}

// Draw paints the tab strip, the separator, and delegates to Group
// for the body content.
func (t *Tabs) Draw() {
	stripBG := types.MakeAttr(0x07, 0x01)     // gray on blue
	inactive := types.MakeAttr(0x07, 0x01)    // gray on blue
	inactiveHot := types.MakeAttr(0x0E, 0x01) // yellow hotkey
	active := types.MakeAttr(0x00, 0x07)      // black on light gray
	activeHot := types.MakeAttr(0x04, 0x07)   // red hotkey on gray
	w := t.Size.X

	// Row 0: tab strip.
	buf := screen.MakeDrawBuffer(w)
	for x := 0; x < w; x++ {
		screen.DrawCell(buf, x, " ", stripBG)
	}
	x := 0
	for i, tab := range t.Items {
		label := " " + tab.Title + " "
		n, hk := inactive, inactiveHot
		if i == t.current {
			n, hk = active, activeHot
		}
		screen.DrawCStr(buf, x, label, n, hk)
		x += labelWidth(tab.Title)
		if x >= w {
			break
		}
	}
	t.WriteLine(0, 0, w, 1, buf)

	// Row 1 is the top edge of the body box, with the active tab
	// punched through so it visually connects to its content.
	sep := screen.MakeDrawBuffer(w)
	for x := 0; x < w; x++ {
		screen.DrawCell(sep, x, "─", stripBG)
	}
	// Box corners on the top row.
	if w > 0 {
		screen.DrawCell(sep, 0, "┌", stripBG)
	}
	if w > 1 {
		screen.DrawCell(sep, w-1, "┐", stripBG)
	}
	// Active tab range overwrites the line + corners with the tab's
	// background color and corner-eases on either side.
	if t.current >= 0 && t.current < len(t.Items) {
		x0 := 0
		for i := 0; i < t.current; i++ {
			x0 += labelWidth(t.Items[i].Title)
		}
		x1 := x0 + labelWidth(t.Items[t.current].Title)
		for px := x0; px < x1 && px < w; px++ {
			screen.DrawCell(sep, px, " ", active)
		}
		if x0-1 >= 0 && x0-1 < w {
			screen.DrawCell(sep, x0-1, "┘", stripBG)
		}
		if x1 < w {
			screen.DrawCell(sep, x1, "└", stripBG)
		}
	}
	t.WriteLine(0, 1, w, 1, sep)

	// Body box: side borders rows 2..Size.Y-2, plus the bottom row.
	leftCell := screen.DrawBuffer{{Ch: "│", Attr: stripBG}}
	rightCell := screen.DrawBuffer{{Ch: "│", Attr: stripBG}}
	for y := 2; y < t.Size.Y-1; y++ {
		t.WriteLine(0, y, 1, 1, leftCell)
		if w > 1 {
			t.WriteLine(w-1, y, 1, 1, rightCell)
		}
	}
	if t.Size.Y > 2 {
		bot := screen.MakeDrawBuffer(w)
		for i := 0; i < w; i++ {
			screen.DrawCell(bot, i, "─", stripBG)
		}
		if w > 0 {
			screen.DrawCell(bot, 0, "└", stripBG)
		}
		if w > 1 {
			screen.DrawCell(bot, w-1, "┘", stripBG)
		}
		t.WriteLine(0, t.Size.Y-1, w, 1, bot)
	}

	// Body content draws via the Group's child walk into the inset bounds.
	t.Group.Draw()
}

// HandleEvent: Ctrl+Tab cycles forward, Ctrl+Shift+Tab back; clicks on
// the strip switch tabs; everything else delegates.
func (t *Tabs) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		local := t.MakeLocal(ev.Where)
		if local.Y == 0 {
			x := 0
			for i, tab := range t.Items {
				w := labelWidth(tab.Title)
				if local.X >= x && local.X < x+w {
					t.SetCurrent(i)
					t.ClearEvent(ev)
					return
				}
				x += w
			}
		}
	}
	if ev.What == consts.EvKeyDown {
		switch ev.KeyCode {
		case consts.KbCtrlTab:
			next := (t.current + 1) % len(t.Items)
			t.SetCurrent(next)
			t.ClearEvent(ev)
			return
		}
	}
	t.Group.HandleEvent(ev)
}
