package menus

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

func rawKeyMenuFixture(t *testing.T, raw, passThrough bool) (*views.Group, *MenuBar) {
	t.Helper()
	t.Cleanup(views.ResetForTest)

	host := views.NewGroup(geom.NewRect(0, 0, 80, 25))
	content := views.NewGroup(geom.NewRect(0, 1, 80, 24))
	focused := views.NewBase(geom.NewRect(0, 1, 80, 24))
	focused.SetSelf(&focused)
	focused.Options |= consts.OfSelectable
	if raw {
		focused.State |= consts.SfRawKeys
	}
	content.Insert(&focused)
	host.Insert(content)

	bar := NewMenuBar(geom.NewRect(0, 0, 80, 1), NewMenu(
		&Item{Name: "~File", Sub: NewMenu(&Item{Name: "Close", Command: consts.CmClose})},
	))
	bar.PassThroughRawKeys = passThrough
	host.Insert(bar)
	return host, bar
}

func keyboardActivationEvents() []struct {
	name string
	make func() drivers.Event
} {
	return []struct {
		name string
		make func() drivers.Event
	}{
		{"F10", func() drivers.Event {
			return drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbF10}
		}},
		{"Alt-F", func() drivers.Event {
			return drivers.Event{
				What: consts.EvKeyDown, KeyCode: consts.KbAltF,
				KeyShift: consts.KbAltShift, UnicodeChar: 'f',
			}
		}},
	}
}

func TestMenuBarPassesActivationKeysToRawFocus(t *testing.T) {
	host, _ := rawKeyMenuFixture(t, true, true)
	for _, tt := range keyboardActivationEvents() {
		t.Run(tt.name, func(t *testing.T) {
			ev := tt.make()
			host.HandleEvent(&ev)
			if ev.What != consts.EvKeyDown {
				t.Fatalf("raw-focus event was consumed: %+v", ev)
			}
		})
	}
}

func TestMenuBarStillActivatesForNormalFocus(t *testing.T) {
	host, _ := rawKeyMenuFixture(t, false, true)
	q := drivers.NewQueue()
	views.SetEventQueue(q)
	for _, tt := range keyboardActivationEvents() {
		t.Run(tt.name, func(t *testing.T) {
			// openSubmenu runs a modal loop. A queued Escape closes it after
			// activation without needing an application or terminal backend.
			q.Put(drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbEsc})
			ev := tt.make()
			host.HandleEvent(&ev)
			if ev.What != consts.EvNothing {
				t.Fatalf("normal-focus event was not consumed by menu: %+v", ev)
			}
		})
	}
}

func TestMenuBarRawPassThroughDefaultsOff(t *testing.T) {
	host, bar := rawKeyMenuFixture(t, true, false)
	if bar.PassThroughRawKeys {
		t.Fatal("PassThroughRawKeys default must preserve classic menu behavior")
	}
	q := drivers.NewQueue()
	views.SetEventQueue(q)
	for _, tt := range keyboardActivationEvents() {
		t.Run(tt.name, func(t *testing.T) {
			q.Put(drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbEsc})
			ev := tt.make()
			host.HandleEvent(&ev)
			if ev.What != consts.EvNothing {
				t.Fatalf("compatibility-default event was not consumed: %+v", ev)
			}
		})
	}
}
