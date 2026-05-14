// Package clipboard is the cut/copy/paste backbone for fv-go widgets.
//
// It maintains a process-local string buffer (so widgets within the
// same app share clipboard data even when the host clipboard is
// unavailable) and, when wired to a terminal writer, emits OSC 52
// sequences so the host's clipboard receives Set operations too.
//
// The "get" path relies on the host's bracketed-paste mechanism: when
// the user types Cmd-V / Ctrl-V, the terminal sends the pasted text
// inside ESC[200~ … ESC[201~, which the term backend turns into
// EvCommand+CmPaste with InfoPtr=string. Widgets read that directly
// rather than asking the clipboard package — OSC 52 "query" is async
// and patchily supported, so it's not worth the complexity.
package clipboard

import (
	"encoding/base64"
	"errors"
	"sync"
)

// Policy bounds OSC 52 emission. Zero value is sane (OSC 52 enabled,
// 100 KB default) — DisableOSC52 inverts the bool so callers don't
// need to remember to set EnableOSC52:true.
type Policy struct {
	DisableOSC52 bool // zero value: OSC 52 enabled
	MaxBytes     int  // zero value: normalized to 100 KB
}

// defaultPolicy is the initial setting before any SetPolicy call.
var defaultPolicy = Policy{MaxBytes: 100_000}

// ErrClipboardTooLarge is returned by TrySetText when the payload
// would exceed Policy.MaxBytes. The internal buffer is still updated;
// only the OSC 52 emission is skipped.
var ErrClipboardTooLarge = errors.New("clipboard: payload exceeds policy MaxBytes")

var (
	mu     sync.RWMutex
	buf    string
	writer func(string) error
	policy = defaultPolicy
)

// normalizePolicy fills in a zero MaxBytes with the default. Called
// from SetPolicy so callers can pass Policy{DisableOSC52: true} and
// not have to specify a cap they don't care about.
func normalizePolicy(p Policy) Policy {
	if p.MaxBytes <= 0 {
		p.MaxBytes = defaultPolicy.MaxBytes
	}
	return p
}

// SetPolicy installs new OSC 52 policy. Goroutine-safe.
func SetPolicy(p Policy) {
	p = normalizePolicy(p)
	mu.Lock()
	policy = p
	mu.Unlock()
}

// SetWriter installs the OSC-52 writer (typically a closure over
// term.Backend.WriteRaw). Pass nil to disable host-clipboard sync.
// app.NewApplication wires this automatically.
func SetWriter(w func(string) error) {
	mu.Lock()
	writer = w
	mu.Unlock()
}

// SetText replaces the clipboard contents. The new value is also
// pushed to the host clipboard via OSC 52 if a writer is installed
// and the current policy permits it. Errors are swallowed — most
// widget copy paths don't want to handle clipboard errors; use
// TrySetText if you do.
func SetText(s string) { _ = TrySetText(s) }

// TrySetText is the error-returning variant of SetText. The internal
// buffer is always updated; the returned error reflects the OSC 52
// emission path only:
//
//   - nil if the payload was written or OSC 52 is disabled by policy
//   - ErrClipboardTooLarge if len(s) > policy.MaxBytes
//   - the writer's error if the writer returned one
func TrySetText(s string) error {
	mu.Lock()
	buf = s
	w := writer
	p := policy
	mu.Unlock()
	if p.DisableOSC52 || w == nil || len(s) == 0 {
		return nil
	}
	if len(s) > p.MaxBytes {
		return ErrClipboardTooLarge
	}
	// OSC 52 selection codes: c = clipboard, p = primary, q = secondary,
	// s = "selection". "c" works on the most terminals.
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	return w("\x1b]52;c;" + enc + "\x07")
}

// GetText returns the current internal buffer. Bracketed-paste events
// are the canonical path for pulling host-clipboard contents — call
// this when you need the cached app-local clipboard (e.g., to
// reproduce a recent Copy without round-tripping through the host).
func GetText() string {
	mu.RLock()
	defer mu.RUnlock()
	return buf
}
