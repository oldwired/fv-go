package drivers

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
)

// TestWheelProjectsToMouseWheel: a term-layer mouse event that
// carries a MbScrollWheel button bit must project to EvMouseWheel
// (NOT EvMouseDown). This is the schema-level guarantee widgets rely
// on to keep clicks and wheel separate.
func TestWheelProjectsToMouseWheel(t *testing.T) {
	cases := []struct {
		name    string
		buttons byte
	}{
		{"wheel up", consts.MbScrollWheelUp},
		{"wheel down", consts.MbScrollWheelDown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev := FromTermEvent(term.Event{
				Kind: term.EventMouse,
				Mouse: term.MouseState{
					Where:   geom.Point{X: 1, Y: 2},
					Buttons: c.buttons,
					Pressed: true, // xterm protocol emits wheel as "press"
				},
			})
			if ev.What != consts.EvMouseWheel {
				t.Errorf("ev.What = %#x, want EvMouseWheel (%#x)", ev.What, consts.EvMouseWheel)
			}
			if ev.Buttons != c.buttons {
				t.Errorf("button bit dropped: got %#x, want %#x", ev.Buttons, c.buttons)
			}
		})
	}
}

// TestRegularClickStillProjectsToMouseDown ensures we didn't
// accidentally route every press through EvMouseWheel.
func TestRegularClickStillProjectsToMouseDown(t *testing.T) {
	ev := FromTermEvent(term.Event{
		Kind: term.EventMouse,
		Mouse: term.MouseState{
			Where:   geom.Point{X: 5, Y: 5},
			Buttons: consts.MbLeftButton,
			Pressed: true,
		},
	})
	if ev.What != consts.EvMouseDown {
		t.Errorf("regular click projected to %#x, want EvMouseDown", ev.What)
	}
}

// TestMouseMaskCoversWheel: EvMouseWheel must satisfy the existing
// "any mouse event" check used by Window for click-through swallowing.
func TestMouseMaskCoversWheel(t *testing.T) {
	if consts.EvMouseWheel&consts.EvMouse == 0 {
		t.Errorf("EvMouseWheel (%#x) not inside EvMouse mask (%#x)",
			consts.EvMouseWheel, consts.EvMouse)
	}
	// And sanity: every other mouse kind still in the mask.
	for _, k := range []uint16{
		consts.EvMouseDown, consts.EvMouseUp,
		consts.EvMouseMove, consts.EvMouseAuto,
	} {
		if k&consts.EvMouse == 0 {
			t.Errorf("%#x missing from EvMouse mask", k)
		}
	}
}

// TestMouseWheelNoCollisionWithKeyboard: the new bit must not overlap
// EvKeyDown or EvKeyUp — that would route wheel events to keyboard
// handlers.
func TestMouseWheelNoCollisionWithKeyboard(t *testing.T) {
	if consts.EvMouseWheel&consts.EvKeyDown != 0 {
		t.Errorf("EvMouseWheel collides with EvKeyDown")
	}
	if consts.EvMouseWheel&consts.EvKeyUp != 0 {
		t.Errorf("EvMouseWheel collides with EvKeyUp")
	}
}
