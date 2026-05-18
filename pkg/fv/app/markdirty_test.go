package app

import (
	"testing"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/term"
)

// TestProgramMarkDirtyWakesLoop regression: the public
// (*Program).MarkDirty method only set the dirty flag, while the
// views.SetMarkDirty closure installed by NewProgram also nudged the
// wake channel. External goroutines calling Program.MarkDirty
// directly could leave waitOne parked until the next user keystroke
// arrived. The fix routes both paths through a shared markDirty
// helper so the wake nudge always fires.
func TestProgramMarkDirtyWakesLoop(t *testing.T) {
	backend := term.NewHeadless(40, 10)
	p := NewProgram(backend)

	// Drain any pre-existing wake (NewProgram itself might have set
	// dirty during construction).
	select {
	case <-p.wake:
	default:
	}

	p.MarkDirty()

	select {
	case <-p.wake:
		// success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("MarkDirty did not signal p.wake — async data won't trigger a repaint until the next keystroke")
	}

	if !p.dirty.Load() {
		t.Error("MarkDirty did not set p.dirty")
	}
}

// TestNewApplicationWiresOnDone regression: NewApplication used to
// rely on an interface assertion `any(p).(interface{Done()})` to
// reach Application.Done from Program.Run's recover block. But the
// receiver in Run is *Program, and Go's method sets don't promote
// outward from embedding — so the assertion always failed and
// Application.Done never ran from the panic path, leaving PTYs /
// terminal state half torn down on crash. The fix is an explicit
// OnDone hook; this test confirms NewApplication installs it.
func TestNewApplicationWiresOnDone(t *testing.T) {
	a, err := NewApplication()
	if err != nil {
		// NewApplication requires a TTY; skip on CI / non-tty.
		t.Skipf("NewApplication unavailable in this environment: %v", err)
	}
	defer a.Done()
	if a.Program.OnDone == nil {
		t.Fatal("NewApplication did not install OnDone")
	}
}

// TestCallSoonSurvivesQueuePressure regression: CallSoon used to
// route through the bounded 64-event input queue, so a burst of
// callbacks while the queue was full silently dropped some of them.
// The fix uses a dedicated callback channel; this test enqueues many
// callbacks and verifies they all run after a manual drain.
func TestCallSoonSurvivesQueuePressure(t *testing.T) {
	backend := term.NewHeadless(40, 10)
	p := NewProgram(backend)

	const N = 200 // well above the old 64-event queue cap
	var ran int
	for i := 0; i < N; i++ {
		i := i
		p.CallSoon(func() { _ = i; ran++ })
	}
	p.drainCallbacks()

	if ran != N {
		t.Errorf("CallSoon dropped %d/%d callbacks under burst (q-pressure regression)", N-ran, N)
	}
}
