// Package logviewer provides LogViewer — an append-only display of
// timestamped log entries with severity coloring, level filtering,
// and auto-scroll-to-bottom behavior.
//
// Ported from LogViewer.pas. The Pascal version stores entries in a
// fixed-capacity ring buffer; the Go port uses a bounded slice with
// the same effect.
package logviewer

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Level is the severity of one log entry.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Entry is one line.
type Entry struct {
	Time    time.Time
	Level   Level
	Source  string // optional component / category tag
	Message string
}

// LogViewer renders a scrollable list of Entries.
type LogViewer struct {
	views.Base

	mu         sync.Mutex
	entries    []Entry
	MaxItems   int  // ring-buffer cap; older entries drop
	Top        int  // first visible entry index
	AutoScroll bool // when true, new entries scroll the view
	MinLevel   Level

	HScroll *views.ScrollBar
	VScroll *views.ScrollBar
}

// New constructs an empty LogViewer.
func New(bounds geom.Rect, h, v *views.ScrollBar) *LogViewer {
	l := &LogViewer{
		Base:       views.NewBase(bounds),
		MaxItems:   2000,
		AutoScroll: true,
		HScroll:    h,
		VScroll:    v,
	}
	l.SetSelf(l)
	l.Options |= consts.OfSelectable
	l.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	return l
}

// GetTypeID for serial registry.
func (l *LogViewer) GetTypeID() string { return "logviewer" }

// Append adds an entry with the current time.
func (l *LogViewer) Append(level Level, source, msg string) {
	l.AppendAt(time.Now(), level, source, msg)
}

// AppendAt adds an entry at a caller-supplied time.
func (l *LogViewer) AppendAt(t time.Time, level Level, source, msg string) {
	l.mu.Lock()
	l.entries = append(l.entries, Entry{Time: t, Level: level, Source: source, Message: msg})
	if len(l.entries) > l.MaxItems {
		drop := len(l.entries) - l.MaxItems
		l.entries = l.entries[drop:]
	}
	if l.AutoScroll {
		l.Top = max(0, l.visibleCount()-l.Size.Y)
	}
	l.mu.Unlock()
}

// Clear empties the log.
func (l *LogViewer) Clear() {
	l.mu.Lock()
	l.entries = l.entries[:0]
	l.Top = 0
	l.mu.Unlock()
}

// SetMinLevel filters out entries below threshold (Debug < Info < Warn < Error).
func (l *LogViewer) SetMinLevel(level Level) { l.MinLevel = level }

// visibleCount returns how many entries pass the MinLevel filter.
// Caller holds the mutex.
func (l *LogViewer) visibleCount() int {
	n := 0
	for _, e := range l.entries {
		if e.Level >= l.MinLevel {
			n++
		}
	}
	return n
}

// Draw paints visible entries.
func (l *LogViewer) Draw() {
	colors := map[Level]uint16{
		LevelDebug: types.MakeAttr(0x08, 0x00),
		LevelInfo:  types.MakeAttr(0x07, 0x00),
		LevelWarn:  types.MakeAttr(0x0E, 0x00),
		LevelError: types.MakeAttr(0x0C, 0x00),
	}
	timeColor := types.MakeAttr(0x06, 0x00)
	srcColor := types.MakeAttr(0x09, 0x00)

	l.mu.Lock()
	visible := l.visibleEntries()
	l.mu.Unlock()

	for r := 0; r < l.Size.Y; r++ {
		buf := screen.MakeDrawBuffer(l.Size.X)
		for x := 0; x < l.Size.X; x++ {
			screen.DrawCell(buf, x, " ", types.MakeAttr(0x07, 0x00))
		}
		idx := l.Top + r
		if idx >= 0 && idx < len(visible) {
			e := visible[idx]
			ts := e.Time.Format("15:04:05")
			screen.DrawStr(buf, 0, ts, timeColor)
			x := utf8.StringDisplayWidth(ts) + 1
			lvl := levelTag(e.Level)
			screen.DrawStr(buf, x, lvl, colors[e.Level])
			x += utf8.StringDisplayWidth(lvl) + 1
			if e.Source != "" {
				tag := "[" + e.Source + "]"
				screen.DrawStr(buf, x, tag, srcColor)
				x += utf8.StringDisplayWidth(tag) + 1
			}
			msg := strings.ReplaceAll(e.Message, "\n", " ")
			screen.DrawStr(buf, x, utf8.CopyDisplayCells(msg, 0, l.Size.X-x), colors[e.Level])
		}
		l.WriteLine(0, r, l.Size.X, 1, buf)
	}
	if l.VScroll != nil {
		l.VScroll.SetRange(0, len(visible))
		l.VScroll.SetValue(l.Top)
	}
}

// visibleEntries returns the slice filtered by MinLevel. Caller holds
// the mutex.
func (l *LogViewer) visibleEntries() []Entry {
	if l.MinLevel == LevelDebug {
		return l.entries
	}
	out := make([]Entry, 0, len(l.entries))
	for _, e := range l.entries {
		if e.Level >= l.MinLevel {
			out = append(out, e)
		}
	}
	return out
}

func levelTag(l Level) string {
	switch l {
	case LevelDebug:
		return "DEBG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERR "
	}
	return "????"
}

// HandleEvent: arrows / wheel / pageup / pagedown / home / end.
func (l *LogViewer) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		if ev.Buttons&(consts.MbScrollWheelUp|consts.MbScrollWheelDown) != 0 {
			step := 3
			if ev.Buttons&consts.MbScrollWheelUp != 0 {
				step = -step
			}
			l.scrollBy(step)
			l.ClearEvent(ev)
			return
		}
		if l.Owner != nil {
			l.Owner.Focus(l.Self())
		}
		l.ClearEvent(ev)
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbUp:
		l.scrollBy(-1)
	case consts.KbDown:
		l.scrollBy(+1)
	case consts.KbPgUp:
		l.scrollBy(-l.Size.Y)
	case consts.KbPgDn:
		l.scrollBy(+l.Size.Y)
	case consts.KbHome:
		l.Top = 0
		l.AutoScroll = false
	case consts.KbEnd:
		l.AutoScroll = true
		l.mu.Lock()
		l.Top = max(0, l.visibleCount()-l.Size.Y)
		l.mu.Unlock()
	default:
		return
	}
	l.Draw()
	l.ClearEvent(ev)
}

func (l *LogViewer) scrollBy(d int) {
	l.mu.Lock()
	l.Top += d
	if l.Top < 0 {
		l.Top = 0
	}
	max := l.visibleCount() - l.Size.Y
	if max < 0 {
		max = 0
	}
	if l.Top > max {
		l.Top = max
	}
	l.AutoScroll = l.Top >= max
	l.mu.Unlock()
	l.Draw()
}

// Logf is a fmt.Sprintf-style helper.
func (l *LogViewer) Logf(level Level, source, format string, args ...any) {
	l.Append(level, source, fmt.Sprintf(format, args...))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
