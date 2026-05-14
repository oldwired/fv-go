// Package drivers provides the FV-style Event record, command set,
// palette types, and the event queue used by Program.GetEvent.
//
// Where Drivers.pas in the Pascal version owned the keyboard and mouse
// directly via the Win32 console API, here the same role is played by
// term.Backend — drivers reads term.Event values from the backend
// channel and projects them into the legacy FV Event shape used by
// every view.
package drivers

import (
	"sync"
	"sync/atomic"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// Event mirrors Pascal TEvent. The What discriminator selects which
// fields are valid; in Pascal these were a variant record.
type Event struct {
	What uint16

	// evMouse*
	Buttons   byte
	DoubleClk bool
	Where     geom.Point

	// evKeyDown
	KeyCode     uint16 // legacy combined scan/ASCII (high=scan, low=ASCII)
	KeyShift    uint16 // shift state (KbCtrlShift, KbAltShift, ...)
	UnicodeChar rune

	// evMessage / evCommand / evBroadcast
	Command  uint16
	InfoPtr  any
	InfoLong int32
	InfoWord uint16
	InfoInt  int16
	InfoByte byte
}

// Clear marks the event as consumed. Mirrors TView.ClearEvent.
func (e *Event) Clear() {
	e.What = consts.EvNothing
	e.InfoPtr = nil
}

// Queue is a FIFO of pending events. Producers push via Put; the main
// loop pulls via Get. Capped at 64 (matches Drivers.pas QueueMax).
//
// Overflow policy (when the queue is full):
//
//   - EvMouseMove: drop silently (noise; high-rate event from
//     `\x1b[?1003h` cell-motion mouse).
//   - EvCommand + CmResizeApp: coalesce — replace any pending resize
//     with the latest so the program sees the final size.
//   - Everything else (keys, paste, other commands): drop and bump
//     the dropped counter; the caller's Put returns false. App-level
//     code observes the false return to drive Program.OnEventDropped.
//
// "Briefly block" was considered and rejected: the only sane place to
// wait would be inside the existing mutex, which would starve consumers
// or require lock-release-retry gymnastics. Observability via Dropped()
// is the more practical answer.
type Queue struct {
	mu      sync.Mutex
	items   []Event
	cap     int
	dropped atomic.Uint64
}

// NewQueue returns a queue with the FV-default capacity of 64.
func NewQueue() *Queue { return &Queue{cap: 64} }

// Put appends ev. Returns false if the event was rejected (queue full
// AND not eligible for silent drop or coalescing). Mouse-move drops do
// NOT bump the dropped counter; everything else does.
func (q *Queue) Put(ev Event) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) < q.cap {
		q.items = append(q.items, ev)
		return true
	}
	// Mouse moves: noise, drop silently.
	if ev.What == consts.EvMouseMove {
		return false
	}
	// Resize is idempotent — replace the latest pending resize so
	// the program sees the final WINSIZE.
	if ev.What == consts.EvCommand && ev.Command == consts.CmResizeApp {
		for i := len(q.items) - 1; i >= 0; i-- {
			if q.items[i].What == consts.EvCommand &&
				q.items[i].Command == consts.CmResizeApp {
				q.items[i] = ev
				return true
			}
		}
	}
	q.dropped.Add(1)
	return false
}

// Dropped returns the total number of events the queue has rejected
// since construction (excluding silently-dropped mouse moves and
// successfully coalesced resizes). Goroutine-safe.
func (q *Queue) Dropped() uint64 { return q.dropped.Load() }

// Get pops the head event and returns true. If empty, returns the zero
// Event and false.
func (q *Queue) Get() (Event, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return Event{}, false
	}
	ev := q.items[0]
	q.items = q.items[1:]
	return ev, true
}

// Len returns the current queue length.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
