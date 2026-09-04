package term

import (
	"bytes"
	"io"
	"reflect"
	"testing"
	"time"
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

func TestCtrlAThroughZAndSpace(t *testing.T) {
	input := make([]byte, 27)
	for i := byte(0); i <= 26; i++ {
		input[i] = i
	}
	got := parseAll(input)
	if len(got) != len(input) {
		t.Fatalf("got %d events, want %d", len(got), len(input))
	}
	if got[0].Rune != 0 || got[0].Mods != ModCtrl {
		t.Errorf("Ctrl-Space/NUL = %+v", got[0])
	}
	for i := 1; i <= 26; i++ {
		// Classic byte streams cannot distinguish Ctrl-I from Tab or
		// Ctrl-J/Ctrl-M from line-feed/carriage-return Enter.
		if i == 9 {
			if got[i].Key != KeyTab {
				t.Errorf("control byte %d = %+v, want Tab", i, got[i])
			}
			continue
		}
		if i == 10 || i == 13 {
			if got[i].Key != KeyEnter {
				t.Errorf("control byte %d = %+v, want Enter", i, got[i])
			}
			continue
		}
		wantRune := rune('a' + i - 1)
		if got[i].Rune != wantRune || got[i].Mods != ModCtrl {
			t.Errorf("control byte %d = %+v, want rune %q + Ctrl", i, got[i], wantRune)
		}
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

type sequenceChunkReader struct {
	chunks [][]byte
	index  int
}

func (r *sequenceChunkReader) Read(p []byte) (int, error) {
	if r.index >= len(r.chunks) {
		return 0, io.EOF
	}
	chunk := r.chunks[r.index]
	r.index++
	return copy(p, chunk), nil
}

func readChunkedEvents(t *testing.T, chunks ...[]byte) []Event {
	t.Helper()
	r := newReader(&sequenceChunkReader{chunks: chunks})
	var events []Event
	for {
		evs, err := r.Next()
		events = append(events, evs...)
		if err == io.EOF {
			return events
		}
		if err != nil {
			t.Fatalf("Next() error: %v", err)
		}
	}
}

func TestEscapeSequencesAreReadBoundaryIndependent(t *testing.T) {
	sequences := []struct {
		name string
		seq  string
	}{
		{"up-csi", "\x1b[A"}, {"down-csi", "\x1b[B"},
		{"right-csi", "\x1b[C"}, {"left-csi", "\x1b[D"},
		{"home-csi", "\x1b[H"}, {"end-csi", "\x1b[F"},
		{"home-tilde", "\x1b[1~"}, {"end-tilde", "\x1b[4~"},
		{"insert", "\x1b[2~"}, {"delete", "\x1b[3~"},
		{"page-up", "\x1b[5~"}, {"page-down", "\x1b[6~"},
		{"f1-ss3", "\x1bOP"}, {"f2-ss3", "\x1bOQ"},
		{"f3-ss3", "\x1bOR"}, {"f4-ss3", "\x1bOS"},
		{"f5", "\x1b[15~"}, {"f6", "\x1b[17~"},
		{"f7", "\x1b[18~"}, {"f8", "\x1b[19~"},
		{"f9", "\x1b[20~"}, {"f10", "\x1b[21~"},
		{"f11", "\x1b[23~"}, {"f12", "\x1b[24~"},
		{"modified-arrow", "\x1b[1;8A"},
		{"modified-f1", "\x1b[1;6P"},
		{"modified-f12", "\x1b[24;7~"},
		{"ss3-up", "\x1bOA"}, {"ss3-home", "\x1bOH"},
		{"alt-ascii", "\x1bx"}, {"alt-utf8", "\x1b界"},
		{"sgr-mouse", "\x1b[<20;10;5M"},
		{"focus-in", "\x1b[I"}, {"focus-out", "\x1b[O"},
		{"bracketed-paste", "\x1b[200~hello 界\x1b[201~"},
	}

	for _, tt := range sequences {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte(tt.seq)
			want := readChunkedEvents(t, input)
			if len(want) == 0 {
				t.Fatal("unsplit input produced no event")
			}
			for split := 1; split < len(input); split++ {
				got := readChunkedEvents(t, input[:split], input[split:])
				if !reflect.DeepEqual(got, want) {
					t.Errorf("split %d: got %+v, want %+v", split, got, want)
				}
			}
		})
	}
}

type delayedContinuationReader struct {
	continuation <-chan struct{}
	second       []byte
	call         int
}

func (r *delayedContinuationReader) Read(p []byte) (int, error) {
	switch r.call {
	case 0:
		r.call++
		return copy(p, []byte{0x1b}), nil
	case 1:
		r.call++
		<-r.continuation
		return copy(p, r.second), nil
	default:
		return 0, io.EOF
	}
}

func TestBareEscapeExpiresThenNormalKeyRemains(t *testing.T) {
	continueRead := make(chan struct{})
	timerRequested := make(chan struct{})
	fireTimer := make(chan time.Time, 1)
	r := newReader(&delayedContinuationReader{continuation: continueRead, second: []byte("x")})
	r.after = func(time.Duration) <-chan time.Time {
		close(timerRequested)
		return fireTimer
	}

	type result struct {
		events []Event
		err    error
	}
	resultCh := make(chan result, 1)
	go func() {
		events, err := r.Next()
		resultCh <- result{events: events, err: err}
	}()
	<-timerRequested
	fireTimer <- time.Now()
	first := <-resultCh
	if first.err != nil || len(first.events) != 1 || first.events[0].Key != KeyEsc {
		t.Fatalf("expired Escape = events %+v, err %v", first.events, first.err)
	}

	close(continueRead)
	second, err := r.Next()
	if err != nil || len(second) != 1 || second[0].Rune != 'x' || second[0].Mods != 0 {
		t.Fatalf("post-deadline key = events %+v, err %v", second, err)
	}
}

func TestRepeatedEscapeProducesTwoEscapeEvents(t *testing.T) {
	got := readChunkedEvents(t, []byte{0x1b, 0x1b})
	if len(got) != 2 || got[0].Key != KeyEsc || got[1].Key != KeyEsc {
		t.Fatalf("got %+v, want two Escape key events", got)
	}
}

func TestShutdownInterruptsPendingEscape(t *testing.T) {
	continueRead := make(chan struct{})
	timerRequested := make(chan struct{})
	neverFire := make(chan time.Time)
	stop := make(chan struct{})
	r := newReader(&delayedContinuationReader{continuation: continueRead, second: []byte("x")})
	r.after = func(time.Duration) <-chan time.Time {
		close(timerRequested)
		return neverFire
	}

	done := make(chan error, 1)
	go func() {
		_, err := r.nextUntil(stop)
		done <- err
	}()
	<-timerRequested
	close(stop)
	if err := <-done; err != io.EOF {
		t.Fatalf("shutdown error = %v, want io.EOF", err)
	}
	close(continueRead)
}
