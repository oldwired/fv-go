package drivers

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/term"
)

func TestFromTermEventPreservesLogicalKey(t *testing.T) {
	wantMods := term.ModShift | term.ModAlt | term.ModCtrl
	ev := FromTermEvent(term.Event{
		Kind: term.EventKey,
		Key:  term.KeyF12,
		Mods: wantMods,
	})
	got := ev.EffectiveKey()
	if !got.Valid || got.Key != term.KeyF12 || got.Mods != wantMods {
		t.Fatalf("EffectiveKey = %+v, want F12 with all modifiers", got)
	}
	if ev.KeyCode != consts.KbF12 {
		t.Errorf("legacy KeyCode = %#x, want KbF12", ev.KeyCode)
	}
	if ev.KeyShift != consts.KbLeftShift|consts.KbAltShift|consts.KbCtrlShift {
		t.Errorf("legacy KeyShift = %#x, want all modifier flags", ev.KeyShift)
	}
}

func TestFromTermEventPreservesCtrlSpaceNUL(t *testing.T) {
	ev := FromTermEvent(term.Event{Kind: term.EventKey, Rune: 0, Mods: term.ModCtrl})
	got := ev.EffectiveKey()
	if !got.Valid || got.Rune != 0 || got.Mods != term.ModCtrl {
		t.Fatalf("EffectiveKey = %+v, want explicit Ctrl-Space/NUL", got)
	}
	if ev.What != consts.EvKeyDown || ev.KeyShift&consts.KbCtrlShift == 0 {
		t.Fatalf("legacy projection was not retained: %+v", ev)
	}
}

func TestEffectiveKeyNormalizesLegacyVariants(t *testing.T) {
	tests := []struct {
		name string
		ev   Event
		key  term.Key
		r    rune
		mods term.ModBits
	}{
		{"ctrl-left", Event{What: consts.EvKeyDown, KeyCode: consts.KbCtrlLeft}, term.KeyLeft, 0, term.ModCtrl},
		{"shift-alt-f4", Event{What: consts.EvKeyDown, KeyCode: consts.KbAltF4, KeyShift: consts.KbLeftShift}, term.KeyF4, 0, term.ModShift | term.ModAlt},
		{"alt-letter", Event{What: consts.EvKeyDown, KeyCode: consts.KbAltF}, term.KeyNone, 'f', term.ModAlt},
		{"unicode", Event{What: consts.EvKeyDown, UnicodeChar: '界', KeyShift: consts.KbAltShift}, term.KeyNone, '界', term.ModAlt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ev.EffectiveKey()
			if !got.Valid || got.Key != tt.key || got.Rune != tt.r || got.Mods != tt.mods {
				t.Fatalf("EffectiveKey() = %+v, want key=%v rune=%q mods=%v", got, tt.key, tt.r, tt.mods)
			}
		})
	}
}
