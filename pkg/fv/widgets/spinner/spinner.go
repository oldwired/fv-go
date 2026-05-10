// Package spinner provides Spinner — a small animated busy indicator
// with selectable frame sets.
package spinner

import (
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// FrameSet groups a list of frame strings and a default tick interval.
type FrameSet struct {
	Frames   []string
	Interval time.Duration
}

// Built-in frame sets, ported from SpinnerView.pas.
var (
	Dots        = FrameSet{Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}, Interval: 80 * time.Millisecond}
	Line        = FrameSet{Frames: []string{"-", "\\", "|", "/"}, Interval: 130 * time.Millisecond}
	Arc         = FrameSet{Frames: []string{"◜", "◠", "◝", "◞", "◡", "◟"}, Interval: 100 * time.Millisecond}
	BouncingBar = FrameSet{Frames: []string{"[    ]", "[=   ]", "[==  ]", "[=== ]", "[ ===]", "[  ==]", "[   =]", "[    ]", "[   =]", "[  ==]", "[ ===]", "[====]", "[=== ]", "[==  ]", "[=   ]"}, Interval: 80 * time.Millisecond}
	Triangle    = FrameSet{Frames: []string{"◢", "◣", "◤", "◥"}, Interval: 250 * time.Millisecond}
	Pipe        = FrameSet{Frames: []string{"┤", "┘", "┴", "└", "├", "┌", "┬", "┐"}, Interval: 100 * time.Millisecond}
	Pulse       = FrameSet{Frames: []string{"◯", "◉", "●", "◉"}, Interval: 200 * time.Millisecond}
)

// Spinner cycles through Frames at Interval.
type Spinner struct {
	views.Base

	Set    FrameSet
	Color  uint16
	frame  int
	active bool
}

// New constructs a spinner with the dots frame set.
func New(bounds geom.Rect) *Spinner {
	s := &Spinner{
		Base:  views.NewBase(bounds),
		Set:   Dots,
		Color: types.MakeAttr(0x0E, 0x03),
	}
	s.SetSelf(s)
	return s
}

// GetTypeID for serial registry.
func (s *Spinner) GetTypeID() string { return "spinner" }

// Start begins animation.
func (s *Spinner) Start() {
	if s.active {
		return
	}
	s.active = true
	anim.Register(s, s.Set.Interval)
}

// Stop pauses animation.
func (s *Spinner) Stop() {
	s.active = false
	anim.Unregister(s)
}

// SetSet swaps frame sets, reusing the active flag.
func (s *Spinner) SetSet(set FrameSet) {
	s.Set = set
	s.frame = 0
	if s.active {
		anim.Register(s, set.Interval) // re-register with new interval
	}
}

// Tick advances frame index.
func (s *Spinner) Tick(now time.Time) bool {
	if !s.active || len(s.Set.Frames) == 0 {
		return false
	}
	s.frame = (s.frame + 1) % len(s.Set.Frames)
	return true
}

// Draw paints the current frame.
func (s *Spinner) Draw() {
	if len(s.Set.Frames) == 0 {
		return
	}
	frame := s.Set.Frames[s.frame]
	for y := 0; y < s.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(s.Size.X)
		for x := 0; x < s.Size.X; x++ {
			screen.DrawCell(buf, x, " ", s.Color)
		}
		if y == s.Size.Y/2 {
			screen.DrawStr(buf, 0, frame, s.Color)
		}
		s.WriteLine(0, y, s.Size.X, 1, buf)
	}
}
