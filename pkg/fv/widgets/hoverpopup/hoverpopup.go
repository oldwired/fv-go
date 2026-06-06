// Package hoverpopup provides HoverPopup — a non-modal, multi-line
// tooltip/popup positioned at an arbitrary cell of a host group. Built
// for LSP hover documentation: the host anchors it at a token (e.g.
// editor.CellOf), it never steals focus, and with AutoDismiss it goes
// away on the next click or keypress without swallowing that input.
//
// Anchor coordinates are host-local. A typical editor hover:
//
//	cell, ok := ed.CellOf(offset)          // editor-local
//	ex, ey := ed.BaseView().ScreenOrigin() // → screen
//	hx, hy := host.ScreenOrigin()          // → host-local
//	pop.Show(host, geom.Point{X: cell.X + ex - hx, Y: cell.Y + ey - hy}, text)
package hoverpopup

import (
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/rivo/uniseg"
)

// HoverPopup is the popup view. Show/Close manage insertion into the
// host; the host owns dismissal policy beyond the AutoDismiss net
// (e.g. closing on cursor movement).
type HoverPopup struct {
	views.Base

	// MaxWidth is the wrap width for content lines (default 60).
	MaxWidth int
	// AutoDismiss, when true (the default), inserts an invisible
	// full-host shield below the popup that closes it on any click,
	// wheel, or keypress — without consuming the click/keystroke
	// (except Esc, which is consumed).
	AutoDismiss bool
	Color       uint16
	FrameColor  uint16

	lines  []string
	host   *views.Group
	shield *shield
	anchor geom.Point
}

// New constructs an idle HoverPopup. Call Show to display it.
func New() *HoverPopup {
	h := &HoverPopup{
		Base:        views.NewBase(geom.Rect{}),
		MaxWidth:    60,
		AutoDismiss: true,
		Color:       theme.Get().HoverPopupNormal,
		FrameColor:  theme.Get().HoverPopupFrame,
	}
	h.SetSelf(h)
	return h
}

// GetTypeID for serial registry.
func (h *HoverPopup) GetTypeID() string { return "hoverpopup" }

// Show displays text (split on newlines, wrapped at MaxWidth) in a
// bordered popup anchored at the host-local cell `at` — preferring the
// row below it, flipping above when it would cross the host's bottom
// edge, and clamping horizontally. Re-Show while open just updates
// content and position.
func (h *HoverPopup) Show(host *views.Group, at geom.Point, text string) {
	h.ShowLines(host, at, strings.Split(text, "\n"))
}

// ShowLines is Show for pre-split content.
func (h *HoverPopup) ShowLines(host *views.Group, at geom.Point, lines []string) {
	if host == nil {
		return
	}
	maxW := h.MaxWidth
	if maxW < 8 {
		maxW = 8
	}
	var wrapped []string
	for _, l := range lines {
		wrapped = append(wrapped, wrapLine(l, maxW)...)
	}
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}
	if h.host != nil && h.host != host {
		h.Close()
	}
	h.host = host
	h.lines = wrapped
	h.anchor = at
	h.place()
	if h.Owner == nil {
		if h.AutoDismiss && h.shield == nil {
			h.shield = newShield(h, host)
			host.InsertPassive(h.shield)
		}
		host.InsertPassive(h)
	}
	h.State |= consts.SfVisible | consts.SfExposed
	views.MarkDirty()
}

// Move repositions an open popup to a new host-local anchor.
func (h *HoverPopup) Move(at geom.Point) {
	if !h.IsOpen() {
		return
	}
	h.anchor = at
	h.place()
	views.MarkDirty()
}

// place computes bounds from the wrapped content and the anchor.
func (h *HoverPopup) place() {
	contentW := 0
	for _, l := range h.lines {
		if w := uniseg.StringWidth(l); w > contentW {
			contentW = w
		}
	}
	w := contentW + 4 // border + one cell of padding per side
	hgt := len(h.lines) + 2
	hostW, hostH := h.host.Size.X, h.host.Size.Y
	if w > hostW {
		w = hostW
	}
	x := h.anchor.X
	if x+w > hostW {
		x = hostW - w
	}
	if x < 0 {
		x = 0
	}
	y := h.anchor.Y + 1
	if y+hgt > hostH {
		// Flip above the anchor; clamp when it doesn't fit there either.
		y = h.anchor.Y - hgt
		if y < 0 {
			y = 0
		}
		if y+hgt > hostH {
			hgt = hostH - y
		}
	}
	h.SetBounds(geom.NewRect(x, y, x+w, y+hgt))
}

// Close removes the popup (and its shield) from the host. Idempotent.
func (h *HoverPopup) Close() {
	if h.Owner != nil {
		h.Owner.Delete(h)
	}
	if h.shield != nil {
		if h.shield.Owner != nil {
			h.shield.Owner.Delete(h.shield)
		}
		h.shield = nil
	}
	h.host = nil
	views.MarkDirty()
}

// IsOpen reports whether the popup is currently inserted.
func (h *HoverPopup) IsOpen() bool { return h.Owner != nil }

// Draw paints the border and content.
func (h *HoverPopup) Draw() {
	if h.Size.X < 2 || h.Size.Y < 2 {
		return
	}
	w := h.Size.X
	for r := 0; r < h.Size.Y; r++ {
		buf := screen.MakeDrawBuffer(w)
		for x := 0; x < w; x++ {
			screen.DrawCell(buf, x, " ", h.Color)
		}
		switch r {
		case 0:
			screen.DrawCell(buf, 0, "┌", h.FrameColor)
			for x := 1; x < w-1; x++ {
				screen.DrawCell(buf, x, "─", h.FrameColor)
			}
			screen.DrawCell(buf, w-1, "┐", h.FrameColor)
		case h.Size.Y - 1:
			screen.DrawCell(buf, 0, "└", h.FrameColor)
			for x := 1; x < w-1; x++ {
				screen.DrawCell(buf, x, "─", h.FrameColor)
			}
			screen.DrawCell(buf, w-1, "┘", h.FrameColor)
		default:
			screen.DrawCell(buf, 0, "│", h.FrameColor)
			screen.DrawCell(buf, w-1, "│", h.FrameColor)
			if r-1 < len(h.lines) {
				screen.DrawStr(buf, 2, h.lines[r-1], h.Color)
			}
		}
		h.WriteLine(0, r, w, 1, buf)
	}
}

// HandleEvent: a click inside the popup dismisses it (consumed — the
// popup really was the click target).
func (h *HoverPopup) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown || ev.What == consts.EvMouseWheel {
		h.Close()
		h.ClearEvent(ev)
	}
}

// shield is the AutoDismiss net: an invisible, full-host child sitting
// just below the popup. Mouse events anywhere close the popup and are
// NOT consumed (the snapshot walk in Group.HandleEvent continues to
// the real target); keyboard events get a first look via OfPreProcess
// — Esc closes and is consumed, anything else closes and passes
// through.
type shield struct {
	views.Base
	popup *HoverPopup
}

func newShield(p *HoverPopup, host *views.Group) *shield {
	s := &shield{
		Base:  views.NewBase(geom.NewRect(0, 0, host.Size.X, host.Size.Y)),
		popup: p,
	}
	s.SetSelf(s)
	s.State |= consts.SfVisible
	s.Options |= consts.OfPreProcess
	s.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	return s
}

func (s *shield) GetTypeID() string { return "hoverpopupshield" }

func (s *shield) Draw() {}

func (s *shield) HandleEvent(ev *drivers.Event) {
	switch ev.What {
	case consts.EvMouseDown, consts.EvMouseWheel:
		s.popup.Close()
		// deliberately not consumed
	case consts.EvKeyDown:
		s.popup.Close()
		if ev.KeyCode == consts.KbEsc {
			s.ClearEvent(ev)
		}
	}
}

// wrapLine wraps one logical line at width display cells, breaking at
// spaces where possible (uniseg-aware so CJK/emoji widths count
// correctly). Leading indentation is preserved on every wrapped
// continuation so code blocks in hover docs keep their shape.
func wrapLine(line string, width int) []string {
	if uniseg.StringWidth(line) <= width {
		return []string{line}
	}
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	if indent != "" {
		iw := uniseg.StringWidth(strings.ReplaceAll(indent, "\t", "    "))
		inner := width - iw
		if inner < 1 {
			inner = 1
		}
		segs := wrapLine(strings.TrimLeft(line, " \t"), inner)
		out := make([]string, len(segs))
		for i, s := range segs {
			out[i] = indent + s
		}
		return out
	}
	var out []string
	var cur strings.Builder
	curW := 0
	for _, word := range strings.Split(line, " ") {
		wW := uniseg.StringWidth(word)
		switch {
		case curW == 0:
			// fallthrough to placement below
		case curW+1+wW <= width:
			cur.WriteByte(' ')
			curW++
		default:
			out = append(out, cur.String())
			cur.Reset()
			curW = 0
		}
		// A single word longer than the width hard-breaks.
		for wW > width {
			g := uniseg.NewGraphemes(word)
			taken, takenW := 0, 0
			for g.Next() {
				cw := g.Width()
				if takenW+cw > width {
					break
				}
				taken += len(g.Str())
				takenW += cw
			}
			if taken == 0 {
				break
			}
			out = append(out, word[:taken])
			word = word[taken:]
			wW = uniseg.StringWidth(word)
		}
		cur.WriteString(word)
		curW += wW
	}
	if cur.Len() > 0 || len(out) == 0 {
		out = append(out, cur.String())
	}
	return out
}
