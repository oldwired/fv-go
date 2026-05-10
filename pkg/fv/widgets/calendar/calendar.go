// Package calendar provides Calendar — a text-mode month grid with
// keyboard / mouse date selection, and CalendarDialog — a modal
// "pick a date" wrapper.
package calendar

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Calendar paints one month with a focused date.
type Calendar struct {
	views.Base

	Selected time.Time
	View     time.Time // first of the visible month

	HeaderColor uint16
	WeekColor   uint16
	DayColor    uint16
	FocusColor  uint16
	OtherMonth  uint16
}

// New constructs a Calendar focused on today.
func New(bounds geom.Rect) *Calendar {
	now := time.Now()
	c := &Calendar{
		Base:        views.NewBase(bounds),
		Selected:    time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local),
		View:        time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local),
		HeaderColor: types.MakeAttr(0x0F, 0x01),
		WeekColor:   types.MakeAttr(0x0E, 0x01),
		DayColor:    types.MakeAttr(0x07, 0x01),
		FocusColor:  types.MakeAttr(0x0F, 0x06),
		OtherMonth:  types.MakeAttr(0x08, 0x01),
	}
	c.SetSelf(c)
	c.Options |= consts.OfSelectable | consts.OfFirstClick
	c.State |= consts.SfCursorVis
	return c
}

// GetTypeID for serial registry.
func (c *Calendar) GetTypeID() string { return "calendar" }

// SetDate moves the selected date and shifts the visible month if
// needed.
func (c *Calendar) SetDate(d time.Time) {
	c.Selected = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local)
	c.View = time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.Local)
}

// Move adds delta days to Selected.
func (c *Calendar) Move(deltaDays int) {
	c.Selected = c.Selected.AddDate(0, 0, deltaDays)
	if c.Selected.Year() != c.View.Year() || c.Selected.Month() != c.View.Month() {
		c.View = time.Date(c.Selected.Year(), c.Selected.Month(), 1, 0, 0, 0, 0, time.Local)
	}
}

// MoveMonth shifts the calendar by m months without changing the day.
func (c *Calendar) MoveMonth(m int) {
	c.View = c.View.AddDate(0, m, 0)
}

// Draw paints title row, weekday header, and the day grid.
func (c *Calendar) Draw() {
	w := c.Size.X
	// Title.
	title := fmt.Sprintf("%s %d", c.View.Month().String(), c.View.Year())
	row := screen.MakeDrawBuffer(w)
	for x := 0; x < w; x++ {
		screen.DrawCell(row, x, " ", c.HeaderColor)
	}
	startX := (w - len(title)) / 2
	screen.DrawStr(row, startX, title, c.HeaderColor)
	c.WriteLine(0, 0, w, 1, row)

	// Weekday header.
	hdr := screen.MakeDrawBuffer(w)
	for x := 0; x < w; x++ {
		screen.DrawCell(hdr, x, " ", c.WeekColor)
	}
	for i, d := range []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"} {
		screen.DrawStr(hdr, i*3, d, c.WeekColor)
	}
	c.WriteLine(0, 1, w, 1, hdr)

	// Days.
	first := c.View
	// Monday-start grid: weekday number 0=Sun in Go; convert.
	weekday := int(first.Weekday()) - 1
	if weekday < 0 {
		weekday = 6
	}
	day := 1
	daysInMonth := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.Local).Day()
	for r := 0; r < 6 && r+2 < c.Size.Y; r++ {
		drow := screen.MakeDrawBuffer(w)
		for x := 0; x < w; x++ {
			screen.DrawCell(drow, x, " ", c.DayColor)
		}
		for col := 0; col < 7; col++ {
			cellIdx := r*7 + col
			if cellIdx < weekday || day > daysInMonth {
				continue
			}
			label := fmt.Sprintf("%2d", day)
			attr := c.DayColor
			if first.Year() == c.Selected.Year() && first.Month() == c.Selected.Month() && day == c.Selected.Day() {
				attr = c.FocusColor
			}
			screen.DrawStr(drow, col*3, label, attr)
			day++
		}
		c.WriteLine(0, 2+r, w, 1, drow)
	}
}

// HandleEvent: arrows / pageup-down / Home / End for navigation, plus
// click-to-pick on any cell of the day grid.
func (c *Calendar) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		local := c.MakeLocal(ev.Where)
		// Day grid starts at row 2; columns are 3 cells wide.
		if local.Y >= 2 {
			row := local.Y - 2
			col := local.X / 3
			if col >= 0 && col < 7 && row >= 0 && row < 6 {
				if day := c.dayAt(row, col); day > 0 {
					c.Selected = time.Date(c.View.Year(), c.View.Month(), day, 0, 0, 0, 0, time.Local)
					c.Draw()
				}
			}
		}
		if c.Owner != nil {
			c.Owner.Focus(c.Self())
		}
		c.ClearEvent(ev)
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbLeft:
		c.Move(-1)
	case consts.KbRight:
		c.Move(+1)
	case consts.KbUp:
		c.Move(-7)
	case consts.KbDown:
		c.Move(+7)
	case consts.KbPgUp:
		c.MoveMonth(-1)
	case consts.KbPgDn:
		c.MoveMonth(+1)
	case consts.KbHome:
		c.Selected = time.Date(c.View.Year(), c.View.Month(), 1, 0, 0, 0, 0, time.Local)
	case consts.KbEnd:
		end := time.Date(c.View.Year(), c.View.Month()+1, 0, 0, 0, 0, 0, time.Local)
		c.Selected = end
	default:
		return
	}
	c.Draw()
	c.ClearEvent(ev)
}

// dayAt maps a (row, col) cell of the day grid to a calendar day, or
// 0 if the cell is outside the current month. row and col are 0-based.
// The grid lays out Monday-first, matching Calendar.Draw.
func (c *Calendar) dayAt(row, col int) int {
	first := c.View
	weekday := int(first.Weekday()) - 1
	if weekday < 0 {
		weekday = 6
	}
	idx := row*7 + col - weekday
	if idx < 0 {
		return 0
	}
	day := idx + 1
	last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.Local).Day()
	if day > last {
		return 0
	}
	return day
}

// ShowDialog runs a date picker dialog and returns the chosen date,
// or zero time if cancelled.
func ShowDialog(host *views.Group, initial time.Time, title string) (time.Time, bool) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 30, 12), title)
	cal := New(geom.NewRect(2, 2, 28, 9))
	if !initial.IsZero() {
		cal.SetDate(initial)
	}
	d.Insert(cal)
	d.Insert(dialogs.NewButton(geom.NewRect(4, 9, 14, 10), "O~K~", consts.CmOK, dialogs.BfDefault))
	d.Insert(dialogs.NewButton(geom.NewRect(16, 9, 26, 10), "Cancel", consts.CmCancel, 0))
	if host.ExecView(d) == consts.CmOK {
		return cal.Selected, true
	}
	return time.Time{}, false
}
