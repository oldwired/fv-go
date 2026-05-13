// Package notification provides Notification — a small non-modal
// "toast" window that auto-dismisses after a timeout.
package notification

import (
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Position picks where in the desktop the toast docks.
type Position int

const (
	PosTopRight Position = iota
	PosTopLeft
	PosBottomRight
	PosBottomLeft
	PosCenter
)

// Notification is a small popup window with a title bar, body, and an
// optional auto-dismiss timer.
type Notification struct {
	views.Base

	Title       string
	Body        string
	Timeout     time.Duration
	Color       uint16
	BorderColor uint16

	birth time.Time
}

// New constructs a toast pinned to the given position of host's extent.
// host is typically the Desktop. Width is required; height is computed
// from the body's line count plus borders.
func New(host *views.Group, title, body string, pos Position, width int, timeout time.Duration) *Notification {
	height := lineCount(body) + 4
	bounds := pinTo(host, pos, width, height)
	n := &Notification{
		Base:        views.NewBase(bounds),
		Title:       title,
		Body:        body,
		Timeout:     timeout,
		Color:       theme.Get().NotificationWarn,
		BorderColor: theme.Get().NotificationWarn,
		birth:       time.Now(),
	}
	n.SetSelf(n)
	n.State |= consts.SfShadow
	if timeout > 0 {
		anim.Register(n, 100*time.Millisecond)
	}
	return n
}

// GetTypeID for serial registry.
func (n *Notification) GetTypeID() string { return "notification" }

// Tick checks the timer; once Timeout has elapsed, the toast removes
// itself from its parent.
func (n *Notification) Tick(now time.Time) bool {
	if n.Timeout > 0 && now.Sub(n.birth) >= n.Timeout {
		anim.Unregister(n)
		if n.Owner != nil {
			n.Owner.Delete(n.Self())
		}
		return true
	}
	return false
}

// HandleEvent: any click dismisses.
func (n *Notification) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		anim.Unregister(n)
		if n.Owner != nil {
			n.Owner.Delete(n.Self())
		}
		n.ClearEvent(ev)
	}
}

// Draw paints frame + title + body.
func (n *Notification) Draw() {
	w, h := n.Size.X, n.Size.Y
	// Body fill.
	for y := 0; y < h; y++ {
		buf := screen.MakeDrawBuffer(w)
		for x := 0; x < w; x++ {
			screen.DrawCell(buf, x, " ", n.Color)
		}
		n.WriteLine(0, y, w, 1, buf)
	}
	// Border.
	top := screen.MakeDrawBuffer(w)
	bot := screen.MakeDrawBuffer(w)
	screen.DrawCell(top, 0, "┌", n.BorderColor)
	screen.DrawCell(bot, 0, "└", n.BorderColor)
	for i := 1; i < w-1; i++ {
		screen.DrawCell(top, i, "─", n.BorderColor)
		screen.DrawCell(bot, i, "─", n.BorderColor)
	}
	screen.DrawCell(top, w-1, "┐", n.BorderColor)
	screen.DrawCell(bot, w-1, "┘", n.BorderColor)
	if n.Title != "" {
		screen.DrawStr(top, 2, " "+n.Title+" ", n.BorderColor)
	}
	n.WriteLine(0, 0, w, 1, top)
	n.WriteLine(0, h-1, w, 1, bot)
	for y := 1; y < h-1; y++ {
		side := screen.DrawBuffer{{Ch: "│", Attr: n.BorderColor}}
		n.WriteLine(0, y, 1, 1, side)
		n.WriteLine(w-1, y, 1, 1, side)
	}
	// Body lines.
	y := 2
	start := 0
	for i := 0; i <= len(n.Body); i++ {
		if i == len(n.Body) || n.Body[i] == '\n' {
			if y >= h-1 {
				break
			}
			line := n.Body[start:i]
			row := screen.MakeDrawBuffer(w - 4)
			screen.DrawStr(row, 0, line, n.Color)
			n.WriteLine(2, y, w-4, 1, row)
			y++
			start = i + 1
		}
	}
}

func lineCount(s string) int {
	if s == "" {
		return 1
	}
	n := 1
	for _, r := range s {
		if r == '\n' {
			n++
		}
	}
	return n
}

func pinTo(host *views.Group, pos Position, w, h int) geom.Rect {
	hw, hh := host.Size.X, host.Size.Y
	x, y := 0, 0
	switch pos {
	case PosTopLeft:
		x, y = 1, 1
	case PosTopRight:
		x, y = hw-w-1, 1
	case PosBottomLeft:
		x, y = 1, hh-h-1
	case PosBottomRight:
		x, y = hw-w-1, hh-h-1
	case PosCenter:
		x, y = (hw-w)/2, (hh-h)/2
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return geom.NewRect(x, y, x+w, y+h)
}
