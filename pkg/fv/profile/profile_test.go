package profile

import (
	"os"
	"testing"
)

func reset() {
	mu.Lock()
	current = Profile{}
	initialized = false
	vtProbeOk = false
	vtProbeSet = false
	mu.Unlock()
}

func TestNoColor(t *testing.T) {
	reset()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("COLORTERM", "")
	t.Setenv("TERM", "")
	SetVTProbe(true)
	p := Get()
	if p.ColorSystem != NoColors {
		t.Errorf("NO_COLOR: got %v want NoColors", p.ColorSystem)
	}
}

func TestTrueColorFromColorterm(t *testing.T) {
	reset()
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORTERM", "truecolor")
	SetVTProbe(true)
	p := Get()
	if p.ColorSystem != TrueColor {
		t.Errorf("got %v want TrueColor", p.ColorSystem)
	}
}

func TestEightBitFromTerm(t *testing.T) {
	reset()
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORTERM", "")
	t.Setenv("TERM", "xterm-256color")
	SetVTProbe(true)
	p := Get()
	if p.ColorSystem != EightBit {
		t.Errorf("got %v want EightBit", p.ColorSystem)
	}
}

func TestLegacyWhenVTUnavailable(t *testing.T) {
	reset()
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORTERM", "")
	t.Setenv("TERM", "")
	SetVTProbe(false)
	p := Get()
	if p.ColorSystem != Legacy {
		t.Errorf("got %v want Legacy", p.ColorSystem)
	}
}

func TestCIDetection(t *testing.T) {
	reset()
	t.Setenv("CI", "true")
	if !detectIsCI() {
		t.Error("CI=true should detect")
	}
	if os.Getenv("CI") == "" {
		t.Skip("Setenv didn't take")
	}
}
