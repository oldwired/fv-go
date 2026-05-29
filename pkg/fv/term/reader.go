package term

import (
	"io"
	"time"
	"unicode/utf8"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// reader incrementally consumes bytes from the tty and emits Events.
//
// The state machine handles:
//   - bare ASCII / control codes
//   - UTF-8 multi-byte sequences (decoded with the std lib once a
//     complete sequence is available)
//   - ESC followed by either:
//   - nothing in the scan buffer -> bare Esc emitted immediately.
//     A blocking 50ms timeout would delay every Esc keypress, which
//     hurts interactive use more than it helps Alt-prefix disambiguation.
//     Apps that need Alt-prefix chord handling should re-coalesce on
//     their own clock.
//   - '[' (CSI) or 'O' (SS3) -> parse params + final byte
//   - a printable char -> Alt-modified key
//   - mouse: SGR 1006 ("\x1b[<b;x;y(M|m)") preferred, X10 fallback
//   - bracketed paste: "\x1b[200~ ... \x1b[201~"
//   - focus: "\x1b[I" / "\x1b[O"
//
// Read() blocks until at least one byte is available, then drains the
// kernel buffer with a non-blocking read so a paste burst doesn't fan
// out into a thousand individual events.
type reader struct {
	in       io.Reader
	buf      []byte
	scan     []byte // unconsumed bytes from previous Read
	paste    bool
	pasteBuf []byte
	// pasteCapped marks the state after a bracketed-paste payload hit
	// the 4 MiB cap. The parser continues to consume bytes but
	// discards them until it sees the closing ESC[201~, at which
	// point it re-arms for normal parsing. Without this, the residual
	// bytes of an oversized paste would get re-parsed as keystrokes
	// and (worst case) inject command sequences after the truncation
	// boundary.
	pasteCapped bool

	// Double-click tracking. A press is "double" if the same button
	// is pressed within doubleClickWindow at the same cell as the
	// previous press.
	lastClickAt      time.Time
	lastClickButtons byte
	lastClickWhere   geom.Point

	// OnCellSize fires when the reader sees a CSI 16t response
	// ("ESC [ 6 ; H ; W t"). The backend uses this to feed
	// sixel.SetCellSize without having to drain b.events on a hot
	// path; the response itself is consumed (no Event emitted).
	OnCellSize func(w, h int)
}

func newReader(in io.Reader) *reader {
	return &reader{in: in, buf: make([]byte, 4096)}
}

// doubleClickWindow is how close in time two presses must be to count
// as a double-click. Standard GUI choice; tcell uses 500ms.
const doubleClickWindow = 400 * time.Millisecond

// maxPasteBytes caps the bracketed-paste accumulator. A pathological
// or malicious sender that opens ESC[200~ and never sends ESC[201~
// would otherwise allocate unbounded memory. 4 MiB is well above any
// realistic interactive paste; the truncation event tells hosts so
// they can react.
const maxPasteBytes = 4 << 20 // 4 MiB

// Next blocks until the underlying reader yields bytes, then returns
// every Event the parser can extract from what it has. Returns an error
// only for irrecoverable I/O failures.
func (r *reader) Next() ([]Event, error) {
	n, err := r.in.Read(r.buf)
	if n > 0 {
		r.scan = append(r.scan, r.buf[:n]...)
	}
	if err != nil && n == 0 {
		return nil, err
	}
	var evs []Event
	for {
		ev, consumed, ok := r.parseOne()
		if !ok {
			break
		}
		r.scan = r.scan[consumed:]
		if ev.Kind != EventNone {
			evs = append(evs, ev)
		}
	}
	if len(evs) == 0 && err != nil {
		return nil, err
	}
	return evs, nil
}

// parseOne tries to parse exactly one event from the head of r.scan.
// Returns ok=false if more bytes are needed.
func (r *reader) parseOne() (ev Event, consumed int, ok bool) {
	if len(r.scan) == 0 {
		return Event{}, 0, false
	}

	// Bracketed paste: collect bytes until the closing sequence.
	if r.paste {
		end := []byte("\x1b[201~")
		if i := indexBytes(r.scan, end); i >= 0 {
			if r.pasteCapped {
				// We already emitted the truncated payload; consume
				// everything up to and including the close sequence
				// without emitting another event and return to
				// normal parsing.
				r.paste = false
				r.pasteCapped = false
				return Event{}, i + len(end), true
			}
			r.pasteBuf = append(r.pasteBuf, r.scan[:i]...)
			r.paste = false
			payload := string(r.pasteBuf)
			r.pasteBuf = r.pasteBuf[:0]
			return Event{Kind: EventPaste, Paste: payload}, i + len(end), true
		}
		if r.pasteCapped {
			// Close sequence not visible yet; keep discarding. Keep
			// some lookback bytes in case ESC[201~ straddles two
			// reads — using len(end)-1 ensures we don't drop the
			// prefix of a split close sequence.
			keep := len(end) - 1
			if len(r.scan) <= keep {
				return Event{}, 0, false
			}
			return Event{}, len(r.scan) - keep, true
		}
		// Cap accumulation so a runaway or malicious sender can't
		// allocate unbounded memory between ESC[200~ and ESC[201~.
		// On overflow we synthesize an end-of-paste event with the
		// Truncated flag set so the host can decide what to do
		// (typically: warn, discard the rest until ESC[201~), then
		// stay in paste-mode with pasteCapped=true so further bytes
		// keep getting discarded until the actual close arrives.
		if len(r.pasteBuf)+len(r.scan) > maxPasteBytes {
			take := maxPasteBytes - len(r.pasteBuf)
			if take > 0 {
				r.pasteBuf = append(r.pasteBuf, r.scan[:take]...)
			}
			payload := string(r.pasteBuf)
			r.pasteBuf = r.pasteBuf[:0]
			r.pasteCapped = true
			consumed := take
			if consumed < 0 {
				consumed = 0
			}
			return Event{Kind: EventPaste, Paste: payload, Truncated: true}, consumed, true
		}
		r.pasteBuf = append(r.pasteBuf, r.scan...)
		return Event{}, len(r.scan), true
	}

	b0 := r.scan[0]
	switch {
	case b0 == 0x1B:
		return r.parseEscape()
	case b0 == 0x7F:
		return Event{Kind: EventKey, Key: KeyBackspace}, 1, true
	case b0 == '\r':
		return Event{Kind: EventKey, Key: KeyEnter}, 1, true
	case b0 == '\n':
		return Event{Kind: EventKey, Key: KeyEnter}, 1, true
	case b0 == '\t':
		return Event{Kind: EventKey, Key: KeyTab}, 1, true
	case b0 == ' ':
		return Event{Kind: EventKey, Key: KeySpace, Rune: ' '}, 1, true
	case b0 < 0x20:
		// Ctrl+letter: 1..26 = Ctrl+A..Ctrl+Z
		if b0 >= 1 && b0 <= 26 {
			return Event{Kind: EventKey, Rune: rune('a' + b0 - 1), Mods: ModCtrl}, 1, true
		}
		return Event{Kind: EventKey, Rune: rune(b0), Mods: ModCtrl}, 1, true
	case b0 < 0x80:
		return Event{Kind: EventKey, Rune: rune(b0)}, 1, true
	}

	// UTF-8 multi-byte sequence. If the buffer holds only a valid prefix
	// of a rune (a sequence split across reads), wait for the remaining
	// bytes instead of consuming the lead byte as a lone invalid byte —
	// otherwise the continuation bytes arrive orphaned and decode as
	// garbage. FullRune is false ONLY for a valid-but-incomplete prefix;
	// a genuinely invalid lead byte is "full" and still gets skipped.
	if !utf8.FullRune(r.scan) {
		return Event{}, 0, false
	}
	r2, size := utf8.DecodeRune(r.scan)
	if r2 == utf8.RuneError && size == 1 {
		// invalid byte; skip
		return Event{}, 1, true
	}
	return Event{Kind: EventKey, Rune: r2}, size, true
}

func (r *reader) parseEscape() (Event, int, bool) {
	if len(r.scan) == 1 {
		// Bare ESC. We can't peek into a future timeout from here, so
		// emit ESC immediately. Apps that care about ESC-vs-Alt-prefix
		// chord handling should do their own re-coalescing.
		return Event{Kind: EventKey, Key: KeyEsc}, 1, true
	}
	switch r.scan[1] {
	case '[':
		return r.parseCSI()
	case 'O':
		return r.parseSS3()
	}
	// Esc-letter == Alt-letter
	c := r.scan[1]
	if c < 0x80 {
		return Event{Kind: EventKey, Rune: rune(c), Mods: ModAlt}, 2, true
	}
	// Esc + UTF-8: treat as Alt+rune
	r2, size := utf8.DecodeRune(r.scan[1:])
	if r2 == utf8.RuneError && size == 1 {
		return Event{}, 2, true
	}
	if size == 0 {
		return Event{}, 0, false
	}
	return Event{Kind: EventKey, Rune: r2, Mods: ModAlt}, 1 + size, true
}

// parseCSI parses ESC '[' ... finalByte
func (r *reader) parseCSI() (Event, int, bool) {
	// We have at least r.scan[0]=ESC, r.scan[1]='['. Find the final byte.
	priv := byte(0)
	idx := 2
	if idx < len(r.scan) && (r.scan[idx] == '?' || r.scan[idx] == '<' || r.scan[idx] == '>') {
		priv = r.scan[idx]
		idx++
	}
	paramStart := idx
	for ; idx < len(r.scan); idx++ {
		c := r.scan[idx]
		if (c >= '0' && c <= '9') || c == ';' || c == ':' {
			continue
		}
		break
	}
	if idx >= len(r.scan) {
		// final byte not yet present
		return Event{}, 0, false
	}
	final := r.scan[idx]
	params := string(r.scan[paramStart:idx])
	consumed := idx + 1

	// Bracketed paste start. Real terminals emit "ESC [ 200 ~"
	// (no private marker) — note the params="200" final='~' shape.
	if priv == 0 && params == "200" && final == '~' {
		r.paste = true
		return Event{}, consumed, true
	}
	// Cell-size response: "ESC [ 6 ; H ; W t" (xterm CSI 16t reply).
	// Fire OnCellSize and consume — no event emitted.
	if priv == 0 && final == 't' {
		if a, b, c, ok := parseThreeParams(params); ok && a == 6 {
			if r.OnCellSize != nil {
				r.OnCellSize(c, b) // params are height, width
			}
			return Event{}, consumed, true
		}
	}
	// Focus events: ESC [ I / ESC [ O
	if priv == 0 && params == "" && final == 'I' {
		return Event{Kind: EventFocusIn}, consumed, true
	}
	if priv == 0 && params == "" && final == 'O' {
		return Event{Kind: EventFocusOut}, consumed, true
	}
	// Mouse SGR-1006: ESC [ < b ; x ; y M/m
	if priv == '<' && (final == 'M' || final == 'm') {
		ev, n, ok := parseSGRMouse(params, final == 'M', consumed)
		if ok && ev.Kind == EventMouse {
			r.maybeMarkDouble(&ev)
		}
		return ev, n, ok
	}
	// Mouse X10: ESC [ M b x y  (we may not have all bytes yet)
	if priv == 0 && params == "" && final == 'M' {
		if len(r.scan) < 6 {
			return Event{}, 0, false
		}
		ev := parseX10Mouse(r.scan[3], r.scan[4], r.scan[5])
		if ev.Kind == EventMouse {
			r.maybeMarkDouble(&ev)
		}
		return ev, 6, true
	}

	return parseCSIKey(params, final, priv), consumed, true
}

func (r *reader) parseSS3() (Event, int, bool) {
	if len(r.scan) < 3 {
		return Event{}, 0, false
	}
	switch r.scan[2] {
	case 'P':
		return Event{Kind: EventKey, Key: KeyF1}, 3, true
	case 'Q':
		return Event{Kind: EventKey, Key: KeyF2}, 3, true
	case 'R':
		return Event{Kind: EventKey, Key: KeyF3}, 3, true
	case 'S':
		return Event{Kind: EventKey, Key: KeyF4}, 3, true
	case 'A':
		return Event{Kind: EventKey, Key: KeyUp}, 3, true
	case 'B':
		return Event{Kind: EventKey, Key: KeyDown}, 3, true
	case 'C':
		return Event{Kind: EventKey, Key: KeyRight}, 3, true
	case 'D':
		return Event{Kind: EventKey, Key: KeyLeft}, 3, true
	case 'H':
		return Event{Kind: EventKey, Key: KeyHome}, 3, true
	case 'F':
		return Event{Kind: EventKey, Key: KeyEnd}, 3, true
	}
	return Event{Kind: EventKey, Rune: rune(r.scan[2]), Mods: ModAlt}, 3, true
}

func parseCSIKey(params string, final, priv byte) Event {
	if priv != 0 {
		return Event{Kind: EventKey, Rune: rune(final)}
	}
	// CSI 1;mods <letter>  modifies a navigation key
	mods := ModBits(0)
	num1, num2 := splitParams(params)
	if num2 > 1 {
		mods = decodeMods(num2)
	}

	// Modified F1-F4: xterm with modifyFunctionKeys=2 emits CSI 1;m P/Q/R/S
	// (this is the only form a modifier can ride with for the SS3 family).
	// Without a modifier these come through parseSS3 as ESC O P/Q/R/S.
	if num1 == 1 && num2 > 1 {
		switch final {
		case 'P':
			return Event{Kind: EventKey, Key: KeyF1, Mods: mods}
		case 'Q':
			return Event{Kind: EventKey, Key: KeyF2, Mods: mods}
		case 'R':
			return Event{Kind: EventKey, Key: KeyF3, Mods: mods}
		case 'S':
			return Event{Kind: EventKey, Key: KeyF4, Mods: mods}
		}
	}

	switch final {
	case 'A':
		return Event{Kind: EventKey, Key: KeyUp, Mods: mods}
	case 'B':
		return Event{Kind: EventKey, Key: KeyDown, Mods: mods}
	case 'C':
		return Event{Kind: EventKey, Key: KeyRight, Mods: mods}
	case 'D':
		return Event{Kind: EventKey, Key: KeyLeft, Mods: mods}
	case 'H':
		return Event{Kind: EventKey, Key: KeyHome, Mods: mods}
	case 'F':
		return Event{Kind: EventKey, Key: KeyEnd, Mods: mods}
	case 'Z':
		return Event{Kind: EventKey, Key: KeyTab, Mods: ModShift}
	}

	if final == '~' {
		switch num1 {
		case 1, 7:
			return Event{Kind: EventKey, Key: KeyHome, Mods: mods}
		case 2:
			return Event{Kind: EventKey, Key: KeyIns, Mods: mods}
		case 3:
			return Event{Kind: EventKey, Key: KeyDel, Mods: mods}
		case 4, 8:
			return Event{Kind: EventKey, Key: KeyEnd, Mods: mods}
		case 5:
			return Event{Kind: EventKey, Key: KeyPgUp, Mods: mods}
		case 6:
			return Event{Kind: EventKey, Key: KeyPgDn, Mods: mods}
		case 11, 15:
			return Event{Kind: EventKey, Key: KeyF1 + Key(num1-11), Mods: mods}
		case 17, 18, 19, 20, 21:
			// 17->F6, 18->F7, 19->F8, 20->F9, 21->F10
			return Event{Kind: EventKey, Key: KeyF6 + Key(num1-17), Mods: mods}
		case 23, 24:
			return Event{Kind: EventKey, Key: KeyF11 + Key(num1-23), Mods: mods}
		}
	}

	return Event{Kind: EventKey, Rune: rune(final), Mods: mods}
}

func splitParams(s string) (a, b int) {
	if s == "" {
		return 0, 0
	}
	// Cheap split on first ';'
	semi := -1
	for i := 0; i < len(s); i++ {
		if s[i] == ';' {
			semi = i
			break
		}
	}
	if semi < 0 {
		a = atoi(s)
		return a, 0
	}
	a = atoi(s[:semi])
	b = atoi(s[semi+1:])
	return
}

// parseThreeParams extracts the three semicolon-separated integers from
// strings like "6;18;9" — used to decode CSI t responses.
func parseThreeParams(s string) (a, b, c int, ok bool) {
	semi1 := -1
	semi2 := -1
	for i := 0; i < len(s); i++ {
		if s[i] == ';' {
			if semi1 < 0 {
				semi1 = i
			} else {
				semi2 = i
				break
			}
		}
	}
	if semi1 < 0 || semi2 < 0 {
		return 0, 0, 0, false
	}
	a = atoi(s[:semi1])
	b = atoi(s[semi1+1 : semi2])
	c = atoi(s[semi2+1:])
	if a == 0 && b == 0 && c == 0 {
		return 0, 0, 0, false
	}
	return a, b, c, true
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func decodeMods(n int) ModBits {
	// xterm modifier code: m-1 is a bitmask; bit0=Shift,bit1=Alt,bit2=Ctrl
	m := n - 1
	if m < 0 {
		return 0
	}
	var mods ModBits
	if m&1 != 0 {
		mods |= ModShift
	}
	if m&2 != 0 {
		mods |= ModAlt
	}
	if m&4 != 0 {
		mods |= ModCtrl
	}
	return mods
}

// maybeMarkDouble flags ev.Mouse.Double=true if it's a press of the
// same button at the same cell as the previous tracked press, within
// doubleClickWindow. Updates the tracking state for the next call.
func (r *reader) maybeMarkDouble(ev *Event) {
	if !ev.Mouse.Pressed || ev.Mouse.Motion || ev.Mouse.Buttons == 0 {
		return
	}
	now := time.Now()
	same := ev.Mouse.Buttons == r.lastClickButtons &&
		ev.Mouse.Where == r.lastClickWhere
	if same && !r.lastClickAt.IsZero() && now.Sub(r.lastClickAt) <= doubleClickWindow {
		ev.Mouse.Double = true
		// Reset so triple-click doesn't double again.
		r.lastClickAt = time.Time{}
		r.lastClickButtons = 0
		r.lastClickWhere = geom.Point{}
		return
	}
	r.lastClickAt = now
	r.lastClickButtons = ev.Mouse.Buttons
	r.lastClickWhere = ev.Mouse.Where
}

func parseSGRMouse(params string, pressed bool, consumed int) (Event, int, bool) {
	// b ; x ; y
	a, rest := splitOff(params, ';')
	xs, ys := splitOff(rest, ';')
	bcode := atoi(a)
	x := atoi(xs) - 1
	y := atoi(ys) - 1

	ev := Event{Kind: EventMouse}
	ev.Mouse.Where.X = x
	ev.Mouse.Where.Y = y
	ev.Mouse.Pressed = pressed
	ev.Mouse.Released = !pressed
	ev.Mouse.Motion = bcode&32 != 0
	// xterm SGR mouse: bits 2/3/4 encode Shift / Meta / Ctrl held
	// when the click happened. Plumb them through so view code can
	// treat Shift+click etc. like the corresponding keyboard chord.
	ev.Mods = sgrMouseMods(bcode)
	bbtn := bcode & 0x03
	switch bbtn {
	case 0:
		ev.Mouse.Buttons = 0x01 // left
	case 1:
		ev.Mouse.Buttons = 0x04 // middle
	case 2:
		ev.Mouse.Buttons = 0x02 // right
	}
	if bcode&64 != 0 {
		// wheel
		if bbtn == 0 {
			ev.Mouse.Buttons = 0x10 // wheel up
		} else if bbtn == 1 {
			ev.Mouse.Buttons = 0x08 // wheel down
		}
	}
	return ev, consumed, true
}

func parseX10Mouse(b, x, y byte) Event {
	if b < 32 {
		return Event{}
	}
	bcode := int(b) - 32
	ev := Event{Kind: EventMouse}
	ev.Mouse.Where.X = int(x) - 33
	ev.Mouse.Where.Y = int(y) - 33
	ev.Mouse.Pressed = (bcode & 0x03) != 3
	ev.Mouse.Released = (bcode & 0x03) == 3
	ev.Mouse.Motion = bcode&32 != 0
	ev.Mods = sgrMouseMods(bcode)
	switch bcode & 0x03 {
	case 0:
		ev.Mouse.Buttons = 0x01
	case 1:
		ev.Mouse.Buttons = 0x04
	case 2:
		ev.Mouse.Buttons = 0x02
	}
	return ev
}

// sgrMouseMods decodes the standard xterm modifier bits (Shift = 4,
// Meta/Alt = 8, Ctrl = 16) into ModBits. Same encoding for SGR and
// legacy X10 reports, so this helper is shared.
func sgrMouseMods(bcode int) ModBits {
	var m ModBits
	if bcode&0x04 != 0 {
		m |= ModShift
	}
	if bcode&0x08 != 0 {
		m |= ModAlt
	}
	if bcode&0x10 != 0 {
		m |= ModCtrl
	}
	return m
}

func splitOff(s string, sep byte) (head, tail string) {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

func indexBytes(haystack, needle []byte) int {
	if len(needle) == 0 {
		return 0
	}
	if len(needle) > len(haystack) {
		return -1
	}
outer:
	for i := 0; i <= len(haystack)-len(needle); i++ {
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
