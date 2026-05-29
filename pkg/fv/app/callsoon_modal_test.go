package app

import (
	"sync/atomic"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// TestModalPumpDrainsCallbacks regression: only Program.Run drained the
// callbacks channel. Modal loops (ExecView, MenuBox.Run, popupmenu /
// fuzzyfinder / stddlg) drive the UI through the installed pump callback
// and never touch the channel, so a CallSoon enqueued while a dialog was
// open stalled until the modal exited. The fix drains callbacks at the
// top of idle (the installed pump), so any loop that pumps also drains.
func TestModalPumpDrainsCallbacks(t *testing.T) {
	backend := term.NewHeadless(40, 10)
	p := NewProgram(backend)

	var ran atomic.Bool
	p.CallSoon(func() { ran.Store(true) })

	pump := views.GetPump()
	if pump == nil {
		t.Fatal("NewProgram did not install a pump")
	}
	// Exactly what a modal loop does each iteration: pump, no Run drain.
	pump()

	if !ran.Load() {
		t.Error("CallSoon callback did not run from the modal pump path — would stall until Run resumes")
	}
}
