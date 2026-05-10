// Package popupmenu provides PopupMenu — a reusable filtered list
// dropdown, used by ComboBox, History, the menu bar, and ad-hoc
// "right-click context menu" patterns.
package popupmenu

import (
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// PopupMenu shows a list of items in a small framed box. The user
// types to filter incrementally; arrows / enter pick. Returns the
// chosen item's index from Run.
type PopupMenu struct {
	views.Base

	Items   []string
	current int
	filter  string
	visible []int // indices of items matching filter

	Color, FrameColor, SelColor uint16
}

// New constructs a popup at origin. The width is at most maxWidth (the
// caller passes it explicitly so the popup can flow off the right edge
// of the host), and the height is the smaller of len(items)+2 and 12.
func New(origin geom.Point, items []string, maxWidth int) *PopupMenu {
	w := 8
	for _, it := range items {
		if l := len(it) + 4; l > w {
			w = l
		}
	}
	if w > maxWidth {
		w = maxWidth
	}
	h := len(items) + 2
	if h > 12 {
		h = 12
	}
	bounds := geom.NewRect(origin.X, origin.Y, origin.X+w, origin.Y+h)
	p := &PopupMenu{
		Base:       views.NewBase(bounds),
		Items:      items,
		Color:      types.MakeAttr(0x00, 0x07),
		FrameColor: types.MakeAttr(0x00, 0x07),
		SelColor:   types.MakeAttr(0x0F, 0x02),
	}
	p.SetSelf(p)
	p.State |= consts.SfVisible | consts.SfExposed
	p.Options |= consts.OfPreProcess
	p.recalcVisible()
	return p
}

// GetTypeID for serial registry.
func (p *PopupMenu) GetTypeID() string { return "popupmenu" }

// Run inserts the popup as a child of host and runs a modal-style loop
// until the user picks (returns chosen item index) or cancels (-1).
func (p *PopupMenu) Run(host *views.Group) int {
	host.Insert(p)
	defer host.Delete(p)
	q := views.GetEventQueue()
	if q == nil {
		return -1
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
		idx, done := p.processEvent(&ev)
		if done {
			return idx
		}
	}
}

// processEvent returns (selected-index, done) where done=true closes
// the popup. selected-index is the original Items index, not the
// filtered index. -1 means cancelled.
func (p *PopupMenu) processEvent(ev *drivers.Event) (int, bool) {
	if ev.What == consts.EvMouseDown {
		local := p.MakeLocal(ev.Where)
		if local.Y >= 1 && local.Y-1 < len(p.visible) &&
			local.X > 0 && local.X < p.Size.X-1 {
			return p.visible[local.Y-1], true
		}
		return -1, true
	}
	if ev.What != consts.EvKeyDown {
		return 0, false
	}
	switch ev.KeyCode {
	case consts.KbEsc:
		return -1, true
	case consts.KbEnter:
		if p.current >= 0 && p.current < len(p.visible) {
			return p.visible[p.current], true
		}
	case consts.KbUp:
		if p.current > 0 {
			p.current--
		}
	case consts.KbDown:
		if p.current+1 < len(p.visible) {
			p.current++
		}
	case consts.KbHome:
		p.current = 0
	case consts.KbEnd:
		p.current = len(p.visible) - 1
	case consts.KbBack:
		if len(p.filter) > 0 {
			p.filter = p.filter[:len(p.filter)-1]
			p.recalcVisible()
		}
	default:
		if ev.UnicodeChar >= ' ' {
			p.filter += string(ev.UnicodeChar)
			p.recalcVisible()
		}
	}
	return 0, false
}

func (p *PopupMenu) recalcVisible() {
	p.visible = p.visible[:0]
	q := strings.ToLower(p.filter)
	for i, it := range p.Items {
		if q == "" || strings.Contains(strings.ToLower(it), q) {
			p.visible = append(p.visible, i)
		}
	}
	if p.current >= len(p.visible) {
		p.current = len(p.visible) - 1
	}
	if p.current < 0 {
		p.current = 0
	}
}

// Draw paints frame + items.
func (p *PopupMenu) Draw() {
	w, h := p.Size.X, p.Size.Y
	top := screen.MakeDrawBuffer(w)
	screen.DrawCell(top, 0, "┌", p.FrameColor)
	for i := 1; i < w-1; i++ {
		screen.DrawCell(top, i, "─", p.FrameColor)
	}
	screen.DrawCell(top, w-1, "┐", p.FrameColor)
	if p.filter != "" {
		screen.DrawStr(top, 2, "/"+p.filter, p.FrameColor)
	}
	p.WriteLine(0, 0, w, 1, top)
	for r := 0; r < h-2; r++ {
		row := screen.MakeDrawBuffer(w)
		c := p.Color
		if r == p.current {
			c = p.SelColor
		}
		screen.DrawCell(row, 0, "│", p.FrameColor)
		for x := 1; x < w-1; x++ {
			screen.DrawCell(row, x, " ", c)
		}
		screen.DrawCell(row, w-1, "│", p.FrameColor)
		if r < len(p.visible) {
			screen.DrawStr(row, 2, p.Items[p.visible[r]], c)
		}
		p.WriteLine(0, 1+r, w, 1, row)
	}
	bot := screen.MakeDrawBuffer(w)
	screen.DrawCell(bot, 0, "└", p.FrameColor)
	for i := 1; i < w-1; i++ {
		screen.DrawCell(bot, i, "─", p.FrameColor)
	}
	screen.DrawCell(bot, w-1, "┘", p.FrameColor)
	p.WriteLine(0, h-1, w, 1, bot)
}
