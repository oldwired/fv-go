// Package anim provides a tiny animation/ticker registry. Widgets that
// need timer-driven repaints (spinners, marquees, blinking indicators,
// auto-dismissing toasts) register a Tick callback on a fixed interval.
//
// The program loop calls Pulse() from its idle path. Pulse iterates the
// registry, fires Tick on any view whose interval has elapsed, and
// reports whether anyone wants a repaint.
package anim

import (
	"sync"
	"time"
)

// Ticker is the contract a widget implements to get periodic Tick
// callbacks. Returning true tells the program loop to redraw on this
// idle pass.
type Ticker interface {
	Tick(now time.Time) (redraw bool)
}

type entry struct {
	t        Ticker
	interval time.Duration
	last     time.Time
}

var (
	mu      sync.Mutex
	entries []*entry
)

// Register adds t to the registry, ticking every interval. Adding the
// same Ticker twice replaces the previous interval.
func Register(t Ticker, interval time.Duration) {
	if t == nil || interval <= 0 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, e := range entries {
		if e.t == t {
			e.interval = interval
			return
		}
	}
	entries = append(entries, &entry{t: t, interval: interval, last: time.Now()})
}

// Unregister removes t. No-op if t isn't registered.
func Unregister(t Ticker) {
	mu.Lock()
	defer mu.Unlock()
	for i, e := range entries {
		if e.t == t {
			entries = append(entries[:i], entries[i+1:]...)
			return
		}
	}
}

// Liveness lets a Ticker tell the registry "drop me, I'm detached".
// Views that embed views.Base inherit an Alive() method that returns
// false once the view is removed from its parent group, which lets
// long-lived gadgets clean themselves up automatically when their
// host window closes.
type Liveness interface {
	Alive() bool
}

// Pulse fires Tick on any due Ticker. Returns true if at least one
// returned redraw=true. Tickers that report Alive()==false are pruned
// before their interval is checked, so a closed window's gadgets stop
// burning CPU.
func Pulse() bool {
	now := time.Now()
	mu.Lock()
	live := entries[:0]
	due := make([]*entry, 0, len(entries))
	for _, e := range entries {
		if l, ok := e.t.(Liveness); ok && !l.Alive() {
			continue
		}
		live = append(live, e)
		if now.Sub(e.last) >= e.interval {
			e.last = now
			due = append(due, e)
		}
	}
	entries = live
	mu.Unlock()
	fired := false
	for _, e := range due {
		if e.t.Tick(now) {
			fired = true
		}
	}
	return fired
}

// MinInterval returns the shortest registered interval, or 0 if no
// tickers are registered. Useful for deciding how long the program
// loop is willing to block on input before pulsing again.
func MinInterval() time.Duration {
	mu.Lock()
	defer mu.Unlock()
	var m time.Duration
	for _, e := range entries {
		if m == 0 || e.interval < m {
			m = e.interval
		}
	}
	return m
}

// Count reports how many tickers are currently registered.
func Count() int {
	mu.Lock()
	defer mu.Unlock()
	return len(entries)
}
