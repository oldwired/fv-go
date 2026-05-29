package term

import (
	"bytes"
	"testing"
)

func parseAll(input []byte) []Event {
	r := newReader(bytes.NewReader(input))
	r.scan = append(r.scan, input...)
	var out []Event
	for {
		ev, n, ok := r.parseOne()
		if !ok {
			break
		}
		r.scan = r.scan[n:]
		if ev.Kind != EventNone {
			out = append(out, ev)
		}
		if len(r.scan) == 0 {
			break
		}
	}
	return out
}

func TestPlainAscii(t *testing.T) {
	got := parseAll([]byte("abc"))
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	for i, c := range []rune{'a', 'b', 'c'} {
		if got[i].Kind != EventKey || got[i].Rune != c {
			t.Errorf("[%d] got %+v", i, got[i])
		}
	}
}

func TestCtrlLetter(t *testing.T) {
	got := parseAll([]byte{0x01}) // Ctrl+A
	if len(got) != 1 || !got[0].Mods.Has(ModCtrl) || got[0].Rune != 'a' {
		t.Errorf("got %+v", got)
	}
}

func TestEnterTabBackspace(t *testing.T) {
	got := parseAll([]byte{'\r', '\t', 0x7F})
	want := []Key{KeyEnter, KeyTab, KeyBackspace}
	if len(got) != 3 {
		t.Fatalf("len: %d", len(got))
	}
	for i, k := range want {
		if got[i].Key != k {
			t.Errorf("[%d] got %v want %v", i, got[i].Key, k)
		}
	}
}

func TestArrowKeysCSI(t *testing.T) {
	cases := []struct {
		seq  string
		want Key
	}{
		{"\x1b[A", KeyUp},
		{"\x1b[B", KeyDown},
		{"\x1b[C", KeyRight},
		{"\x1b[D", KeyLeft},
		{"\x1b[H", KeyHome},
		{"\x1b[F", KeyEnd},
	}
	for _, c := range cases {
		got := parseAll([]byte(c.seq))
		if len(got) != 1 || got[0].Key != c.want {
			t.Errorf("%q: got %+v want %v", c.seq, got, c.want)
		}
	}
}

func TestFunctionKeys(t *testing.T) {
	cases := []struct {
		seq  string
		want Key
	}{
		{"\x1bOP", KeyF1},
		{"\x1b[15~", KeyF5},
		{"\x1b[21~", KeyF10},
		{"\x1b[24~", KeyF12},
		{"\x1b[5~", KeyPgUp},
		{"\x1b[6~", KeyPgDn},
	}
	for _, c := range cases {
		got := parseAll([]byte(c.seq))
		if len(got) != 1 || got[0].Key != c.want {
			t.Errorf("%q: got %+v want %v", c.seq, got, c.want)
		}
	}
}

func TestModifiedArrow(t *testing.T) {
	got := parseAll([]byte("\x1b[1;5A")) // Ctrl+Up
	if len(got) != 1 || got[0].Key != KeyUp || !got[0].Mods.Has(ModCtrl) {
		t.Errorf("got %+v", got)
	}
}

func TestAltLetter(t *testing.T) {
	got := parseAll([]byte{0x1B, 'x'})
	if len(got) != 1 || got[0].Rune != 'x' || !got[0].Mods.Has(ModAlt) {
		t.Errorf("got %+v", got)
	}
}

func TestSGRMouse(t *testing.T) {
	got := parseAll([]byte("\x1b[<0;10;5M"))
	if len(got) != 1 {
		t.Fatalf("len: %+v", got)
	}
	ev := got[0]
	if ev.Kind != EventMouse || ev.Mouse.Where.X != 9 || ev.Mouse.Where.Y != 4 || !ev.Mouse.Pressed {
		t.Errorf("got %+v", ev)
	}
}

func TestBracketedPaste(t *testing.T) {
	got := parseAll([]byte("\x1b[200~hi there\x1b[201~"))
	if len(got) != 1 || got[0].Kind != EventPaste || got[0].Paste != "hi there" {
		t.Errorf("got %+v", got)
	}
}

func TestUTF8Char(t *testing.T) {
	got := parseAll([]byte("é"))
	if len(got) != 1 || got[0].Rune != 'é' {
		t.Errorf("got %+v", got)
	}
}

func TestFocusEvents(t *testing.T) {
	got := parseAll([]byte("\x1b[I\x1b[O"))
	if len(got) != 2 || got[0].Kind != EventFocusIn || got[1].Kind != EventFocusOut {
		t.Errorf("got %+v", got)
	}
}

// TestSplitUTF8AcrossReads regression: a multi-byte rune whose bytes
// arrive in separate reads used to lose its lead byte — DecodeRune
// returned (RuneError,1) on the prefix, the lead byte was skipped, and
// the continuation bytes decoded as garbage. parseOne must now wait for
// the full rune.
func TestSplitUTF8AcrossReads(t *testing.T) {
	full := []byte("世") // 0xE4 0xB8 0x96

	r := newReader(bytes.NewReader(nil))
	r.scan = append(r.scan[:0], full[0]) // only the lead byte
	if ev, n, ok := r.parseOne(); ok {
		t.Fatalf("partial rune consumed: ev=%+v n=%d (should wait for more)", ev, n)
	}

	r.scan = append(r.scan, full[1:]...) // remaining bytes arrive
	ev, n, ok := r.parseOne()
	if !ok || n != len(full) || ev.Rune != '世' {
		t.Fatalf("after completion: ev=%+v n=%d ok=%v, want rune 世 with n=%d", ev, n, ok, len(full))
	}
}

// TestInvalidLeadByteSkipped: a genuinely invalid byte (lone
// continuation 0x80) is "full" per utf8.FullRune and must still be
// skipped, not treated as an incomplete prefix.
func TestInvalidLeadByteSkipped(t *testing.T) {
	r := newReader(bytes.NewReader(nil))
	r.scan = []byte{0x80, 'a'}
	ev, n, ok := r.parseOne()
	if !ok || n != 1 {
		t.Fatalf("invalid byte: ev=%+v n=%d ok=%v, want skip exactly 1 byte", ev, n, ok)
	}
	if ev.Kind != EventNone {
		t.Errorf("invalid byte should emit no event, got %+v", ev)
	}
}
