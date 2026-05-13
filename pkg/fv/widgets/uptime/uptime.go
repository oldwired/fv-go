// Package uptime provides UptimeView — a simple "up since" display
// showing days/hours/minutes/seconds elapsed from a fixed start time.
package uptime

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// UptimeView paints "Dd HH:MM:SS" and refreshes every second.
type UptimeView struct {
	views.Base

	Start time.Time
	Color uint16
}

// New constructs an UptimeView starting now.
func New(bounds geom.Rect) *UptimeView {
	u := &UptimeView{
		Base:  views.NewBase(bounds),
		Start: time.Now(),
		Color: theme.Get().StatValue,
	}
	u.SetSelf(u)
	anim.Register(u, time.Second)
	return u
}

// SetStart resets the reference time.
func (u *UptimeView) SetStart(t time.Time) { u.Start = t }

// GetTypeID for serial registry.
func (u *UptimeView) GetTypeID() string { return "uptimeview" }

// Tick triggers a redraw every second.
func (u *UptimeView) Tick(now time.Time) bool { return true }

// Draw paints the elapsed duration.
func (u *UptimeView) Draw() {
	d := time.Since(u.Start)
	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	h := int(d / time.Hour)
	d -= time.Duration(h) * time.Hour
	m := int(d / time.Minute)
	d -= time.Duration(m) * time.Minute
	s := int(d / time.Second)

	text := fmt.Sprintf("%dd %02d:%02d:%02d", days, h, m, s)
	for y := 0; y < u.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(u.Size.X)
		for x := 0; x < u.Size.X; x++ {
			screen.DrawCell(buf, x, " ", u.Color)
		}
		if y == u.Size.Y/2 {
			x := (u.Size.X - len(text)) / 2
			if x < 0 {
				x = 0
			}
			screen.DrawStr(buf, x, text, u.Color)
		}
		u.WriteLine(0, y, u.Size.X, 1, buf)
	}
}
