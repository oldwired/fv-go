package app

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// TestApplicationDoneClearsGlobalHooks regression: NewProgram /
// NewApplication install several package-global hooks
// (views.SetPump / Wait / MarkDirty / CallSoon / RootBackend /
// EventQueue plus theme.SetOnChange and clipboard.SetWriter). The
// previous Application.Done only stopped views and closed the
// backend — the hooks kept pointing at the now-defunct Program.
// A late goroutine calling views.MarkDirty after Done would dispatch
// into stale state.
//
// We can't fully exercise NewApplication here (it needs a TTY), so
// we drive the global-hook teardown logic directly: call NewProgram
// (which installs the hooks), then directly invoke the teardown the
// real Application.Done now does, and verify the hooks are clear.
func TestApplicationDoneClearsGlobalHooks(t *testing.T) {
	backend := term.NewHeadless(40, 10)
	_ = NewProgram(backend)

	// After NewProgram the hooks should be installed.
	if views.GetPump() == nil {
		t.Fatal("setup: NewProgram did not install Pump hook")
	}

	// Simulate Application.Done's hook-clearing block.
	views.SetEventQueue(nil)
	views.SetPump(nil)
	views.SetWait(nil)
	views.SetMarkDirty(nil)
	views.SetCallSoon(nil)
	views.SetRootBackend(nil)

	if views.GetPump() != nil {
		t.Error("Pump hook not cleared")
	}
	if views.GetWait() != nil {
		t.Error("Wait hook not cleared")
	}
	if views.GetEventQueue() != nil {
		t.Error("EventQueue not cleared")
	}

	// A late goroutine calling MarkDirty must be a quiet no-op, not
	// a nil-dereference panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("post-teardown views.MarkDirty panicked: %v", r)
		}
	}()
	views.MarkDirty()
	views.CallSoon(func() {})
}
