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
	"sync"
)

var (
	mu     sync.RWMutex
	buf    string
	writer func(string) error
)

// SetWriter installs the OSC-52 writer (typically a closure over
// term.Backend.WriteRaw). Pass nil to disable host-clipboard sync.
// app.NewApplication wires this automatically.
func SetWriter(w func(string) error) {
	mu.Lock()
	writer = w
	mu.Unlock()
}

// SetText replaces the clipboard contents. The new value is also
// pushed to the host clipboard via OSC 52 if a writer is installed.
func SetText(s string) {
	mu.Lock()
	buf = s
	w := writer
	mu.Unlock()
	if w != nil && len(s) > 0 {
		// OSC 52 selection codes: c = clipboard, p = primary, q = secondary,
		// s = "selection". "c" works on the most terminals.
		enc := base64.StdEncoding.EncodeToString([]byte(s))
		_ = w("\x1b]52;c;" + enc + "\x07")
	}
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
