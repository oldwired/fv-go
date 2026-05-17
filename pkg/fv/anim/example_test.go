package anim_test

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
)

// counter is a minimal Ticker that counts how often it fires.
type counter struct{ n int }

func (c *counter) Tick(_ time.Time) (redraw bool) {
	c.n++
	return true
}

// Register a Ticker on a short interval, drive it via Pulse, and
// check it fired. In production the Program's idle loop calls Pulse
// every wake-up; tests and alternate event loops do it themselves.
func ExamplePulse() {
	c := &counter{}
	anim.Register(c, 1*time.Microsecond)
	defer anim.Unregister(c)

	// Wait past the interval, then pulse to fire any due Tick.
	time.Sleep(2 * time.Millisecond)
	redraw := anim.Pulse()

	fmt.Println("fired:", c.n, "redraw:", redraw)
	// Output: fired: 1 redraw: true
}
