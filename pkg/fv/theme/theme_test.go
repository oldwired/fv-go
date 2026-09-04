package theme

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/types"
)

func TestDefaultFocusedButtonUsesDarkTextOnBrightBackground(t *testing.T) {
	if got, want := types.BG(Default.ButtonFocused), byte(0x0A); got != want {
		t.Fatalf("focused button background = %#x, want %#x", got, want)
	}
	if got, want := types.FG(Default.ButtonFocused), byte(0x00); got != want {
		t.Errorf("focused button foreground = %#x, want %#x", got, want)
	}
	if got, want := types.BG(Default.ButtonFocusedHot), byte(0x0A); got != want {
		t.Fatalf("focused button hotkey background = %#x, want %#x", got, want)
	}
	if got, want := types.FG(Default.ButtonFocusedHot), byte(0x01); got != want {
		t.Errorf("focused button hotkey foreground = %#x, want %#x", got, want)
	}
}
