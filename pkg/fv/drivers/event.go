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
type Queue struct {
	mu    sync.Mutex
	items []Event
	cap   int
}

// NewQueue returns a queue with the FV-default capacity of 64.
func NewQueue() *Queue { return &Queue{cap: 64} }

// Put appends ev. Drops silently if the queue is at capacity (matching
// Pascal PutEventInQueue's boolean failure semantics — callers don't
// generally check).
func (q *Queue) Put(ev Event) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) >= q.cap {
		return false
	}
	q.items = append(q.items, ev)
	return true
}

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
