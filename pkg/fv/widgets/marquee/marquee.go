// Package marquee provides Marquee — a horizontally scrolling text
// ticker. Drives its own redraw via the shared anim ticker.
package marquee

import (
	"strings"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Direction picks the scroll direction.
type Direction int

const (
	DirLeft  Direction = iota // text scrolls right-to-left (most common)
	DirRight                  // text scrolls left-to-right
)

// Marquee is a single-row text scroller.
type Marquee struct {
	views.Base

	Text      string
	Direction Direction
	Speed     time.Duration // tick interval
	Gap       int           // empty cells between repeats
	Paused    bool

	Color uint16

	offset int
}

// New constructs a Marquee with the given text.
func New(bounds geom.Rect, text string) *Marquee {
	m := &Marquee{
		Base:      views.NewBase(bounds),
		Text:      text,
		Direction: DirLeft,
		Speed:     150 * time.Millisecond,
		Gap:       4,
		Color:     types.MakeAttr(0x0E, 0x01),
	}
	m.SetSelf(m)
	anim.Register(m, m.Speed)
	return m
}

// GetTypeID for serial registry.
func (m *Marquee) GetTypeID() string { return "marquee" }

// SetText replaces the displayed string and resets the scroll offset.
func (m *Marquee) SetText(s string) {
	m.Text = s
	m.offset = 0
}

// Pause / Resume freeze and unfreeze the animation.
func (m *Marquee) Pause()  { m.Paused = true }
func (m *Marquee) Resume() { m.Paused = false }

// Tick advances offset by one cell.
func (m *Marquee) Tick(now time.Time) bool {
	if m.Paused {
		return false
	}
	if m.Direction == DirLeft {
		m.offset++
	} else {
		m.offset--
	}
	return true
}

// Draw paints the visible window of the looped string.
func (m *Marquee) Draw() {
	if m.Size.X <= 0 || m.Size.Y <= 0 {
		return
	}
	stripe := m.Text + strings.Repeat(" ", m.Gap)
	stripeW := utf8.StringDisplayWidth(stripe)
	if stripeW <= 0 {
		return
	}
	// Normalize offset to [0, stripeW).
	off := m.offset % stripeW
	if off < 0 {
		off += stripeW
	}
	visible := stripe + stripe // double so we can slice across the wrap
	out := utf8.CopyDisplayCells(visible, off, m.Size.X)
	for y := 0; y < m.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(m.Size.X)
		for x := 0; x < m.Size.X; x++ {
			screen.DrawCell(buf, x, " ", m.Color)
		}
		screen.DrawStr(buf, 0, out, m.Color)
		m.WriteLine(0, y, m.Size.X, 1, buf)
	}
}
