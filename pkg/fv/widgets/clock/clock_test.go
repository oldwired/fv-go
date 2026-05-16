package clock

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// TestNewDigitalDefaults confirms zero-value-sane construction.
func TestNewDigitalDefaults(t *testing.T) {
	c := NewDigital(geom.NewRect(0, 0, 10, 1))
	defer anim.Unregister(c)
	if c.Mode != Digital {
		t.Errorf("Mode: got %v, want Digital", c.Mode)
	}
	if c.Format != "15:04:05" {
		t.Errorf("Format default: got %q, want %q", c.Format, "15:04:05")
	}
	if c.Location == nil {
		t.Error("Location should default to time.Local, got nil")
	}
	if c.BaseView().Self() != c {
		t.Error("constructor forgot SetSelf")
	}
}

// TestNewAnalogDefaults: ShowSecondHand defaults true; cardinal numerals;
// line hand style.
func TestNewAnalogDefaults(t *testing.T) {
	c := NewAnalog(geom.NewRect(0, 0, 21, 11))
	defer anim.Unregister(c)
	if c.Mode != Analog {
		t.Errorf("Mode: got %v, want Analog", c.Mode)
	}
	if !c.ShowSecondHand {
		t.Error("ShowSecondHand should default true")
	}
	if c.Numerals != NumeralsCardinal {
		t.Errorf("Numerals default: got %v, want NumeralsCardinal", c.Numerals)
	}
	if c.Hands != HandsLine {
		t.Errorf("Hands default: got %v, want HandsLine", c.Hands)
	}
}

// TestSetSmoothSweepAdjustsInterval confirms the helper re-registers the
// clock with anim at the right cadence.
func TestSetSmoothSweepAdjustsInterval(t *testing.T) {
	c := NewAnalog(geom.NewRect(0, 0, 21, 11))
	defer anim.Unregister(c)

	c.SetSmoothSweep(true)
	if c.Interval != 100_000_000 { // 100 ms in nanoseconds
		t.Errorf("smooth-sweep Interval: got %v, want 100ms", c.Interval)
	}
	c.SetSmoothSweep(false)
	if c.Interval != 1_000_000_000 {
		t.Errorf("ticking Interval: got %v, want 1s", c.Interval)
	}
}

// TestBlankColonsBlinksColons regression for the BlinkColon helper.
func TestBlankColonsBlinksColons(t *testing.T) {
	if blankColons("12:34:56") != "12 34 56" {
		t.Errorf("blankColons: got %q, want %q", blankColons("12:34:56"), "12 34 56")
	}
}
