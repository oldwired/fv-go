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
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// PopupMenu shows a list of items in a small framed box. The user
// types to filter incrementally; arrows / enter pick. Returns the
// chosen item's index from Run.
//
// Two activation modes:
//   - Run(host) is a modal blocking loop — the popup owns the event
//     loop until the user picks or cancels. Use for one-shot menus
//     like ComboBox dropdowns, history pickers, and right-click
//     context menus.
//   - Open(host) is non-modal: the popup is just a regular child of
//     host, drawn on top, and events from the host's loop reach it
//     via OfPreProcess. Use for IntelliSense / autocomplete pop-ups
//     that need to coexist with continued typing in the underlying
//     editor — see Open's docstring for the call pattern.
type PopupMenu struct {
	views.Base

	Items   []string
	current int
	filter  string
	visible []int // indices of items matching filter

	// nonModal toggles Open's "let unhandled events fall through to
	// the editor underneath" behavior. Set by Open, never by Run.
	nonModal bool

	// OnSelect, when non-modal, fires when the user picks an item via
	// Enter or click. idx is the original Items index (not the
	// filtered position). Closing the popup after firing is the
	// host's job — call Close() inside the callback. Modal Run
	// ignores this (it returns the index instead).
	OnSelect func(idx int, label string)

	// OnCancel, when non-modal, fires when the user presses Esc or
	// otherwise dismisses without picking. Hosts typically call
	// Close() from this. Modal Run ignores this.
	OnCancel func()

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
		Color:      theme.Get().PopupMenuNormal,
		FrameColor: theme.Get().PopupMenuFrame,
		SelColor:   theme.Get().PopupMenuSelected,
	}
	p.SetSelf(p)
	p.State |= consts.SfVisible | consts.SfExposed
	p.Options |= consts.OfPreProcess
	p.recalcVisible()
	return p
}

// GetTypeID for serial registry.
func (p *PopupMenu) GetTypeID() string { return "popupmenu" }

// Open inserts the popup as a non-modal child of host and returns
// immediately. The host's main event loop drives the popup via the
// normal OfPreProcess path; the popup fires OnSelect / OnCancel when
// the user picks or dismisses, and the host is expected to call
// Close() from those callbacks (or whenever else it wants to dismiss
// the popup — focus loss, cursor leaving the trigger range, …).
//
// Unhandled events fall through to the host: typing letters that
// don't match a filter narrowing still reaches the underlying
// editor / input line, so an IntelliSense popup can stay open while
// the user keeps typing.
//
// Typical IntelliSense / autocomplete shape:
//
//	pm := popupmenu.New(anchor, items, maxW)
//	pm.OnSelect = func(idx int, label string) {
//	    editor.Insert(label[len(prefix):])
//	    pm.Close()
//	}
//	pm.OnCancel = func() { pm.Close() }
//	pm.Open(host)
//	// later, as the user types:
//	pm.Filter(currentPrefix)
func (p *PopupMenu) Open(host *views.Group) {
	p.nonModal = true
	host.Insert(p)
}

// Close removes the popup from its owner. Safe to call multiple times
// (no-op if already detached). Hosts should call this from OnSelect /
// OnCancel and from any other path that should dismiss the popup
// (focus moved away, cursor stepped out of the trigger range, the
// user pressed Esc on an enclosing dialog, …).
func (p *PopupMenu) Close() {
	if p.Owner != nil {
		p.Owner.Delete(p)
	}
}

// Filter narrows the visible items to those containing prefix
// (case-insensitively). Idempotent and cheap; call it on every
// keystroke from the host's HandleEvent.
func (p *PopupMenu) Filter(prefix string) {
	if p.filter == prefix {
		return
	}
	p.filter = prefix
	p.recalcVisible()
	views.MarkDirty()
}

// VisibleCount reports how many items currently survive the filter.
// Hosts use this to decide whether to auto-Close the popup (filter
// dropped to zero matches → nothing to pick; either show a "no
// results" state or dismiss).
func (p *PopupMenu) VisibleCount() int { return len(p.visible) }

// HandleEvent dispatches a single event when the popup is open
// non-modally. Mouse clicks inside the popup pick an item; Enter
// picks the currently-highlighted one; arrows / Home / End move the
// highlight; Esc cancels. Keystrokes that don't match any of those
// roles are left untouched so they continue to the underlying view —
// this is what lets an IntelliSense popup stay open while the user
// types into the editor below it.
//
// In modal mode (Run), this is bypassed: Run reads events from the
// queue directly and never calls back into HandleEvent.
func (p *PopupMenu) HandleEvent(ev *drivers.Event) {
	if !p.nonModal {
		return
	}
	switch ev.What {
	case consts.EvMouseDown:
		local := p.MakeLocal(ev.Where)
		if local.X < 0 || local.Y < 0 || local.X >= p.Size.X || local.Y >= p.Size.Y {
			// Click outside the popup is a cancel.
			if p.OnCancel != nil {
				p.OnCancel()
			}
			p.ClearEvent(ev)
			return
		}
		if local.Y >= 1 && local.Y-1 < len(p.visible) &&
			local.X > 0 && local.X < p.Size.X-1 {
			idx := p.visible[local.Y-1]
			if p.OnSelect != nil {
				p.OnSelect(idx, p.Items[idx])
			}
		}
		p.ClearEvent(ev)
	case consts.EvKeyDown:
		switch ev.KeyCode {
		case consts.KbEsc:
			if p.OnCancel != nil {
				p.OnCancel()
			}
			p.ClearEvent(ev)
		case consts.KbEnter:
			if p.current >= 0 && p.current < len(p.visible) {
				idx := p.visible[p.current]
				if p.OnSelect != nil {
					p.OnSelect(idx, p.Items[idx])
				}
			}
			p.ClearEvent(ev)
		case consts.KbUp:
			if p.current > 0 {
				p.current--
				views.MarkDirty()
			}
			p.ClearEvent(ev)
		case consts.KbDown:
			if p.current+1 < len(p.visible) {
				p.current++
				views.MarkDirty()
			}
			p.ClearEvent(ev)
		case consts.KbHome:
			p.current = 0
			views.MarkDirty()
			p.ClearEvent(ev)
		case consts.KbEnd:
			p.current = len(p.visible) - 1
			views.MarkDirty()
			p.ClearEvent(ev)
		}
	}
}

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
		views.MarkDirty()
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
