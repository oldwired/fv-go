package drivers

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
)

// fillQueue puts cap copies of ev so the next Put hits the overflow path.
func fillQueue(q *Queue, ev Event) {
	for q.Len() < q.cap {
		q.Put(ev)
	}
}

// TestQueueMouseMoveOverflowDrops silently — does not bump Dropped.
func TestQueueMouseMoveOverflowDrops(t *testing.T) {
	q := NewQueue()
	fillQueue(q, Event{What: consts.EvKeyDown})
	if ok := q.Put(Event{What: consts.EvMouseMove}); ok {
		t.Error("expected MouseMove overflow to return false")
	}
	if q.Dropped() != 0 {
		t.Errorf("MouseMove overflow incorrectly bumped Dropped: got %d", q.Dropped())
	}
}

// TestQueueResizeCoalesces — when the queue is full of resizes,
// pushing another should replace, not append.
func TestQueueResizeCoalesces(t *testing.T) {
	q := NewQueue()
	first := Event{What: consts.EvCommand, Command: consts.CmResizeApp, InfoInt: 1}
	fillQueue(q, first)

	// Push a different resize and confirm Dropped did not bump.
	latest := Event{What: consts.EvCommand, Command: consts.CmResizeApp, InfoInt: 99}
	if ok := q.Put(latest); !ok {
		t.Error("resize coalescing should return true")
	}
	if q.Dropped() != 0 {
		t.Errorf("coalesce shouldn't bump Dropped: got %d", q.Dropped())
	}

	// Drain and confirm the LAST item in the queue is the most-recent
	// resize. Earlier items are the prior fills; the tail was replaced.
	var saw int16
	for {
		ev, ok := q.Get()
		if !ok {
			break
		}
		if ev.What == consts.EvCommand && ev.Command == consts.CmResizeApp {
			saw = ev.InfoInt
		}
	}
	if saw != 99 {
		t.Errorf("tail resize InfoInt: got %d, want 99", saw)
	}
}

// TestQueueKeyOverflowBumpsDropped is the real overflow signal —
// non-coalescable events count, callers see false, the counter is
// observable for monitoring.
func TestQueueKeyOverflowBumpsDropped(t *testing.T) {
	q := NewQueue()
	fillQueue(q, Event{What: consts.EvKeyDown})
	before := q.Dropped()
	if ok := q.Put(Event{What: consts.EvKeyDown, KeyCode: 42}); ok {
		t.Error("key overflow should return false")
	}
	if q.Dropped() != before+1 {
		t.Errorf("Dropped didn't bump: was %d, now %d", before, q.Dropped())
	}
}
