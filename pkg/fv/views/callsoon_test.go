package views

import (
	"sync/atomic"
	"testing"
)

// TestCallSoonRoutesThroughInstalledScheduler verifies that the
// installed callSoonFn is invoked, not the inline fallback. This is
// the production path — app.NewProgram installs Program.CallSoon
// which posts a CmUserCallback event back to the loop.
func TestCallSoonRoutesThroughInstalledScheduler(t *testing.T) {
	prev := callSoonFn
	defer func() { callSoonFn = prev }()

	var fired atomic.Int32
	SetCallSoon(func(fn func()) {
		fired.Add(1)
		fn() // simulate the loop dispatching the callback
	})

	var callbackRan atomic.Bool
	CallSoon(func() { callbackRan.Store(true) })

	if fired.Load() != 1 {
		t.Errorf("scheduler not invoked: fired=%d", fired.Load())
	}
	if !callbackRan.Load() {
		t.Error("scheduled fn did not execute")
	}
}

// TestCallSoonInlineFallback covers the no-scheduler-installed path
// — explicitly documented as bootstrap-only, but must not deadlock
// or drop the callback.
func TestCallSoonInlineFallback(t *testing.T) {
	prev := callSoonFn
	callSoonFn = nil
	defer func() { callSoonFn = prev }()

	var ran atomic.Bool
	CallSoon(func() { ran.Store(true) })
	if !ran.Load() {
		t.Error("inline fallback did not execute fn")
	}
}

// TestCallSoonNilFnSafe — common pattern is `if cb != nil { CallSoon(cb) }`
// but defensive callers may pass nil; CallSoon must not panic.
func TestCallSoonNilFnSafe(t *testing.T) {
	prev := callSoonFn
	defer func() { callSoonFn = prev }()
	callSoonFn = func(fn func()) { t.Fatal("nil fn should not be scheduled") }
	CallSoon(nil) // must be a no-op
}
