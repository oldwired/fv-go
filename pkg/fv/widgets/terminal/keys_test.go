package terminal

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	termio "github.com/oldwired/fv-go/pkg/fv/term"
)

var keyboardModifierCases = []struct {
	name  string
	mods  termio.ModBits
	param int
}{
	{"shift", termio.ModShift, 2},
	{"alt", termio.ModAlt, 3},
	{"shift-alt", termio.ModShift | termio.ModAlt, 4},
	{"ctrl", termio.ModCtrl, 5},
	{"shift-ctrl", termio.ModShift | termio.ModCtrl, 6},
	{"alt-ctrl", termio.ModAlt | termio.ModCtrl, 7},
	{"shift-alt-ctrl", termio.ModShift | termio.ModAlt | termio.ModCtrl, 8},
}

func logicalEvent(key termio.Key, r rune, mods termio.ModBits) drivers.Event {
	return drivers.FromTermEvent(termio.Event{
		Kind: termio.EventKey, Key: key, Rune: r, Mods: mods,
	})
}

func TestModifiedCursorKeys(t *testing.T) {
	keys := []struct {
		name  string
		key   termio.Key
		final byte
	}{
		{"up", termio.KeyUp, 'A'},
		{"down", termio.KeyDown, 'B'},
		{"right", termio.KeyRight, 'C'},
		{"left", termio.KeyLeft, 'D'},
		{"home", termio.KeyHome, 'H'},
		{"end", termio.KeyEnd, 'F'},
	}
	for _, key := range keys {
		for _, mod := range keyboardModifierCases {
			t.Run(key.name+"/"+mod.name, func(t *testing.T) {
				ev := logicalEvent(key.key, 0, mod.mods)
				want := fmt.Sprintf("\x1b[1;%d%c", mod.param, key.final)
				if got := string(keyToBytes(&ev, false)); got != want {
					t.Fatalf("got %q, want %q", got, want)
				}
				if got := string(keyToBytes(&ev, true)); got != want {
					t.Fatalf("application mode changed modified key to %q, want %q", got, want)
				}
			})
		}
	}
}

func TestModifiedNavigationKeys(t *testing.T) {
	keys := []struct {
		name   string
		key    termio.Key
		number string
	}{
		{"insert", termio.KeyIns, "2"},
		{"delete", termio.KeyDel, "3"},
		{"page-up", termio.KeyPgUp, "5"},
		{"page-down", termio.KeyPgDn, "6"},
	}
	for _, key := range keys {
		for _, mod := range keyboardModifierCases {
			t.Run(key.name+"/"+mod.name, func(t *testing.T) {
				ev := logicalEvent(key.key, 0, mod.mods)
				want := fmt.Sprintf("\x1b[%s;%d~", key.number, mod.param)
				if got := string(keyToBytes(&ev, false)); got != want {
					t.Fatalf("got %q, want %q", got, want)
				}
			})
		}
	}
}

func TestModifiedFunctionKeys(t *testing.T) {
	keys := []struct {
		key   termio.Key
		first string
		final byte
	}{
		{termio.KeyF1, "1", 'P'}, {termio.KeyF2, "1", 'Q'},
		{termio.KeyF3, "1", 'R'}, {termio.KeyF4, "1", 'S'},
		{termio.KeyF5, "15", '~'}, {termio.KeyF6, "17", '~'},
		{termio.KeyF7, "18", '~'}, {termio.KeyF8, "19", '~'},
		{termio.KeyF9, "20", '~'}, {termio.KeyF10, "21", '~'},
		{termio.KeyF11, "23", '~'}, {termio.KeyF12, "24", '~'},
	}
	for i, key := range keys {
		for _, mod := range keyboardModifierCases {
			t.Run(fmt.Sprintf("F%d/%s", i+1, mod.name), func(t *testing.T) {
				ev := logicalEvent(key.key, 0, mod.mods)
				want := fmt.Sprintf("\x1b[%s;%d%c", key.first, mod.param, key.final)
				if got := string(keyToBytes(&ev, false)); got != want {
					t.Fatalf("got %q, want %q", got, want)
				}
			})
		}
	}
}

func TestRuneAndClassicKeyEncoding(t *testing.T) {
	tests := []struct {
		name string
		ev   drivers.Event
		want []byte
	}{
		{"plain-ascii", logicalEvent(termio.KeyNone, 'x', 0), []byte("x")},
		{"plain-utf8", logicalEvent(termio.KeyNone, '界', 0), []byte("界")},
		{"alt-ascii", logicalEvent(termio.KeyNone, 'x', termio.ModAlt), []byte("\x1bx")},
		{"alt-utf8", logicalEvent(termio.KeyNone, '界', termio.ModAlt), append([]byte{0x1b}, []byte("界")...)},
		{"ctrl-a", logicalEvent(termio.KeyNone, 'a', termio.ModCtrl), []byte{0x01}},
		{"ctrl-space-nul", logicalEvent(termio.KeyNone, 0, termio.ModCtrl), []byte{0x00}},
		{"space", logicalEvent(termio.KeySpace, 0, 0), []byte(" ")},
		{"shift-tab", logicalEvent(termio.KeyTab, 0, termio.ModShift), []byte("\x1b[Z")},
		{"legacy-ctrl-left", drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbCtrlLeft}, []byte("\x1b[1;5D")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyToBytes(&tt.ev, false); !bytes.Equal(got, tt.want) {
				t.Fatalf("got %q (%v), want %q (%v)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestPlainSpecialKeyEncoding(t *testing.T) {
	tests := []struct {
		key  termio.Key
		want string
	}{
		{termio.KeyEnter, "\r"}, {termio.KeyTab, "\t"},
		{termio.KeyBackspace, "\x7f"}, {termio.KeyEsc, "\x1b"},
		{termio.KeySpace, " "},
		{termio.KeyUp, "\x1b[A"}, {termio.KeyDown, "\x1b[B"},
		{termio.KeyLeft, "\x1b[D"}, {termio.KeyRight, "\x1b[C"},
		{termio.KeyHome, "\x1b[H"}, {termio.KeyEnd, "\x1b[F"},
		{termio.KeyPgUp, "\x1b[5~"}, {termio.KeyPgDn, "\x1b[6~"},
		{termio.KeyIns, "\x1b[2~"}, {termio.KeyDel, "\x1b[3~"},
		{termio.KeyF1, "\x1bOP"}, {termio.KeyF2, "\x1bOQ"},
		{termio.KeyF3, "\x1bOR"}, {termio.KeyF4, "\x1bOS"},
		{termio.KeyF5, "\x1b[15~"}, {termio.KeyF6, "\x1b[17~"},
		{termio.KeyF7, "\x1b[18~"}, {termio.KeyF8, "\x1b[19~"},
		{termio.KeyF9, "\x1b[20~"}, {termio.KeyF10, "\x1b[21~"},
		{termio.KeyF11, "\x1b[23~"}, {termio.KeyF12, "\x1b[24~"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("key-%d", tt.key), func(t *testing.T) {
			ev := logicalEvent(tt.key, 0, 0)
			if got := string(keyToBytes(&ev, false)); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEveryDefinedSpecialKeyHasAnEncoding(t *testing.T) {
	for key := termio.KeyEnter; key <= termio.KeyF12; key++ {
		for _, mods := range []termio.ModBits{0, termio.ModShift | termio.ModAlt | termio.ModCtrl} {
			ev := logicalEvent(key, 0, mods)
			if got := keyToBytes(&ev, false); len(got) == 0 {
				t.Errorf("key %v with modifiers %v silently produced no bytes", key, mods)
			}
		}
	}
}

func TestApplicationCursorModeThroughTerminalWriter(t *testing.T) {
	tests := []struct {
		name   string
		key    termio.Key
		normal string
		app    string
	}{
		{"up", termio.KeyUp, "\x1b[A", "\x1bOA"},
		{"down", termio.KeyDown, "\x1b[B", "\x1bOB"},
		{"right", termio.KeyRight, "\x1b[C", "\x1bOC"},
		{"left", termio.KeyLeft, "\x1b[D", "\x1bOD"},
		{"home", termio.KeyHome, "\x1b[H", "\x1bOH"},
		{"end", termio.KeyEnd, "\x1b[F", "\x1bOF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widget := New(geom.NewRect(0, 0, 20, 5))
			var child bytes.Buffer
			widget.input = &child

			ev := logicalEvent(tt.key, 0, 0)
			widget.HandleEvent(&ev)
			if got := child.String(); got != tt.normal {
				t.Fatalf("normal mode got %q, want %q", got, tt.normal)
			}

			child.Reset()
			widget.par.Feed([]byte("\x1b[?1h"))
			ev = logicalEvent(tt.key, 0, 0)
			widget.HandleEvent(&ev)
			if got := child.String(); got != tt.app {
				t.Fatalf("application mode got %q, want %q", got, tt.app)
			}

			child.Reset()
			widget.par.Feed([]byte("\x1b[?1l"))
			ev = logicalEvent(tt.key, 0, 0)
			widget.HandleEvent(&ev)
			if got := child.String(); got != tt.normal {
				t.Fatalf("reset normal mode got %q, want %q", got, tt.normal)
			}
		})
	}
}

func TestRawInputToChildRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want []byte
	}{
		{"ctrl-left", []byte("\x1b[1;5D"), []byte("\x1b[1;5D")},
		{"all-modifiers-up", []byte("\x1b[1;8A"), []byte("\x1b[1;8A")},
		{"alt-utf8", append([]byte{0x1b}, []byte("界")...), append([]byte{0x1b}, []byte("界")...)},
		{"ctrl-space", []byte{0}, []byte{0}},
		{"modified-f12", []byte("\x1b[24;7~"), []byte("\x1b[24;7~")},
		{"plain-f1", []byte("\x1bOP"), []byte("\x1bOP")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outer, consumed, ok := termio.DecodeInputSequence(tt.raw)
			if !ok || consumed != len(tt.raw) {
				t.Fatalf("outer decode: event=%+v consumed=%d ok=%v", outer, consumed, ok)
			}
			projected := drivers.FromTermEvent(outer)
			widget := New(geom.NewRect(0, 0, 20, 5))
			var child bytes.Buffer
			widget.input = &child
			widget.HandleEvent(&projected)
			if got := child.Bytes(); !bytes.Equal(got, tt.want) {
				t.Fatalf("child got %q (%v), want %q (%v)", got, got, tt.want, tt.want)
			}
		})
	}
}
