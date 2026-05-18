package terminal

import (
	"errors"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// TestHandleEventCmCloseClearsEvent regression: the CmClose branch
// used to call Stop() and return without ClearEvent, so the command
// kept propagating to the window/owner and triggered a parallel
// close path. The fix consumes the event after Stop.
func TestHandleEventCmCloseClearsEvent(t *testing.T) {
	term := New(geom.NewRect(0, 0, 40, 10))
	ev := &drivers.Event{What: consts.EvCommand, Command: consts.CmClose}
	term.HandleEvent(ev)
	if ev.What != consts.EvNothing {
		t.Errorf("after CmClose: ev.What = %#x, want EvNothing (0). Bug: command kept propagating.", ev.What)
	}
}

// TestStartTwiceRejectsSecond: a Terminal's PTY is one-shot. A second
// Start used to silently overwrite t.pty while the previous readLoop
// / waitLoop were still alive — a recipe for double-OnExit, lost
// shutdown, and confusing close messages. The fix marks the
// instance as started after the first Start and rejects subsequent
// calls.
//
// We don't actually start a real shell here — instead we set the
// started flag manually to simulate the post-first-Start state, then
// verify the second call returns ErrTerminalAlreadyStarted without
// touching anything.
func TestStartTwiceRejectsSecond(t *testing.T) {
	term := New(geom.NewRect(0, 0, 40, 10))
	term.mu.Lock()
	term.started = true
	term.mu.Unlock()

	err := term.Start("/nonexistent", nil, nil)
	if !errors.Is(err, ErrTerminalAlreadyStarted) {
		t.Errorf("second Start: err = %v, want ErrTerminalAlreadyStarted", err)
	}
}
