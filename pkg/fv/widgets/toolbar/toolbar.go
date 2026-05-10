// Package toolbar provides ToolBar — a horizontal button bar similar
// in spirit to the Pascal TToolBar (which mirrors the StatusLine
// linked-list-of-items pattern).
package toolbar

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Item is one button on the toolbar. Empty Text renders as a separator.
// Command 0 marks a non-clickable label.
type Item struct {
	Text    string
	Command uint16
	HelpCtx uint16
}

// Separator returns an item that renders as a vertical bar between
// other items rather than a clickable button.
func Separator() Item { return Item{} }

// ToolBar paints a row of buttons. Click → emits the corresponding
// command via PutEvent.
type ToolBar struct {
	views.Base

	Items []Item
}

// New constructs a toolbar.
func New(bounds geom.Rect, items []Item) *ToolBar {
	t := &ToolBar{Base: views.NewBase(bounds), Items: items}
	t.SetSelf(t)
	t.GrowMode = consts.GfGrowHiX
	t.Options |= consts.OfPreProcess
	t.EventMask |= consts.EvBroadcast
	return t
}

// GetTypeID for serial registry.
func (t *ToolBar) GetTypeID() string { return "toolbar" }

// Draw paints the bar.
func (t *ToolBar) Draw() {
	normal := types.MakeAttr(0x00, 0x07)
	hot := types.MakeAttr(0x04, 0x07)
	sep := types.MakeAttr(0x08, 0x07)

	w := t.Size.X
	buf := screen.MakeDrawBuffer(w)
	for x := 0; x < w; x++ {
		screen.DrawCell(buf, x, " ", normal)
	}
	x := 1
	for _, it := range t.Items {
		if x >= w {
			break
		}
		if it.Text == "" {
			screen.DrawCell(buf, x, "│", sep)
			x += 2
			continue
		}
		label := " " + it.Text + " "
		screen.DrawCStr(buf, x, label, normal, hot)
		x += utf8.CStrDisplayWidth(label) + 1
	}
	t.WriteLine(0, 0, w, 1, buf)
}

// HandleEvent routes mouse clicks to commands.
func (t *ToolBar) HandleEvent(ev *drivers.Event) {
	if ev.What != consts.EvMouseDown {
		return
	}
	local := t.MakeLocal(ev.Where)
	if local.Y != 0 {
		return
	}
	x := 1
	for _, it := range t.Items {
		if it.Text == "" {
			x += 2
			continue
		}
		w := utf8.CStrDisplayWidth(" "+it.Text+" ") + 1
		if local.X >= x && local.X < x+w-1 && it.Command != 0 {
			cmd := drivers.Event{What: consts.EvCommand, Command: it.Command}
			t.PutEvent(&cmd)
			t.ClearEvent(ev)
			return
		}
		x += w
	}
}
