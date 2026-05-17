package terminal

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/clipboard"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Terminal is the FV view. Spawns a child via PTY, owns a parser +
// buffer, and handles I/O on its own goroutine.
//
// Lifecycle: Run() is the typical entry point — it starts the child
// and blocks until exit. For non-blocking use, the caller can invoke
// Start() / Stop() directly; the view will keep painting until Stop()
// is called or the child exits.
type Terminal struct {
	views.Base

	DefaultFG, DefaultBG byte

	// ScrollbackLines, if non-zero, overrides the default 2000-line
	// scrollback cap. Honored by Start(); set before launching the
	// child. Mutating after Start() has no effect on the live buffer.
	ScrollbackLines int

	// Env is the late-bound environment for the spawned child. If nil
	// at Start() time, the current process environment is inherited
	// (with TERM patched to xterm-256color). Use SetEnv to set this
	// after construction but before Start().
	Env []string

	// WorkingDir is the late-bound working directory for the spawned
	// child. Empty means the parent's cwd. Use SetWorkingDir to set
	// this after construction but before Start().
	WorkingDir string

	// Title is the latest OSC 0/1/2 string the child sent. Empty
	// until the child sets one.
	Title string

	// CWD is the latest OSC 7 cwd string the child reported. Empty
	// until the shell emits one (zsh / bash with PROMPT_COMMAND or
	// equivalent integration).
	CWD string

	// OnTitle, if non-nil, fires from the reader goroutine whenever
	// Title changes — typically used to update the host window's
	// caption. Goroutine-safe: callbacks should not block.
	OnTitle func(string)

	// OnCWDChange fires when an OSC 7 sequence updates CWD. Used by
	// hosts (e.g., fvmux) to surface the focused pane's cwd in the
	// status bar.
	OnCWDChange func(string)

	// OnActivity fires from the reader goroutine when fresh PTY output
	// arrives, but no more often than once per 500ms. Hosts use this
	// to flash status-bar activity dots without redrawing on every
	// chunk.
	OnActivity func()

	// OnBell fires when the parser sees a BEL (0x07) byte in the
	// ground state (i.e., not as an OSC string terminator). Used for
	// audible / visible bell hooks.
	OnBell func()

	// OnExit fires once when the child process exits (also from the
	// reader goroutine). Used to auto-close the wrapping window.
	OnExit func(error)

	// OnFeed, if non-nil, runs synchronously on every PTY-output
	// chunk between the read and the parser. The callback returns
	// the bytes that are actually fed to the parser (and rendered);
	// returning the input unchanged is a pass-through. Hosts use
	// this for byte-stream effects: asciicast recording, deliberate
	// corruption (:rot13), in-flight transformation.
	//
	// PERF: runs on the hot read-path, once per chunk. Cheap callbacks
	// only — anything that blocks or allocates heavily will stall the
	// terminal's rendering.
	OnFeed func(in []byte) (out []byte)

	buf    *buffer
	par    *parser
	pty    *ptyHandle
	mu     sync.Mutex
	closed bool

	// lastActivity is the wall-clock time of the most recent
	// OnActivity fire. Read/written only by the reader goroutine
	// (no other path mutates it), so no lock is needed.
	lastActivity time.Time

	// mouseSuspended gates the mouse-forwarding paths. When true,
	// HandleEvent stops writing SGR-1006 sequences to the PTY,
	// letting fvmux's copy/resize modes intercept mouse without the
	// inner shell seeing it. Selection handling proceeds as normal.
	mouseSuspended bool

	// focused is the host's view of whether this pane is focused. When
	// false, Draw suppresses the cursor regardless of the buffer's
	// DECTCEM state — so an unfocused split doesn't blink. Defaults
	// to true so single-pane callers keep current behavior.
	focused bool

	// Scrollback search. Active while searching is true; the user
	// types into searchQuery, hits Enter to jump to the first match
	// at-or-above the current scroll position, n / N to move
	// between matches, Esc to cancel.
	searching   bool
	searchQuery []byte

	// Drag-to-select state. Coordinates are in viewport cells (0,0
	// at top-left of the terminal view). selecting=true while a
	// drag is in flight; once selDirty flips false, the rendered
	// selection is the committed one. selStartAbs / selEndAbs are
	// the absolute "row in scrollback + cells" indices so the
	// selection survives scrolling.
	selecting   bool
	selStartAbs cellPos
	selEndAbs   cellPos

	// Copy-mode state (tmux prefix-[ flow). EnterCopyMode parks a
	// movable cursor at copyCursor; MoveCopyCursor walks it through
	// scrollback + live region; ToggleCopyAnchor pins it as the
	// selection start so subsequent moves extend the selection.
	// Reuses selStartAbs / selEndAbs so the mouse-drag highlight and
	// the keyboard-driven highlight share rendering.
	copying    bool
	copyCursor cellPos
}

// cellPos is an absolute address inside the buffer's scrollback +
// live region. Row 0 = top of scrollback, increasing downward.
type cellPos struct{ row, col int }

// New constructs a Terminal view at bounds. Buffer size matches the
// view's cell dimensions.
func New(bounds geom.Rect) *Terminal {
	t := &Terminal{
		Base:      views.NewBase(bounds),
		DefaultFG: 0x07,
		DefaultBG: 0x00,
		focused:   true,
	}
	t.SetSelf(t)
	t.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	t.Options |= consts.OfSelectable
	t.EventMask = consts.EvKeyDown | consts.EvCommand |
		consts.EvMouseDown | consts.EvMouseUp | consts.EvMouseMove |
		consts.EvMouseWheel
	// Claim raw keyboard so Program.HandleEvent doesn't fold Ctrl+C
	// into Copy etc. while we're focused. Ctrl+C, Ctrl+X, Ctrl+V,
	// and the F-keys reach the inner shell instead.
	t.State |= consts.SfRawKeys
	t.buf = newBuffer(t.Size.X, t.Size.Y)
	t.par = newParser(t.buf)
	// IMPORTANT: OnTitle / OnCWD / OnBell fire from inside par.Feed,
	// which is always called with t.mu already held (either by
	// readLoop or by Write). Acquiring t.mu here would self-deadlock —
	// when we tried, zsh hung in tcsetattr (TCSADRAIN waiting on a PTY
	// buffer the reader could no longer drain) and the whole UI froze.
	// State is set under the caller's lock, no further sync needed.
	//
	// Host-supplied callbacks (t.OnTitle / t.OnCWDChange / t.OnBell)
	// are dispatched via views.CallSoon so they run on the UI
	// goroutine. CallSoon only enqueues an event — it does not
	// acquire t.mu — so calling it while holding t.mu is safe.
	t.par.OnTitle = func(title string) {
		t.Title = title
		cb := t.OnTitle // captured under caller's lock
		if cb != nil {
			views.CallSoon(func() { cb(title) })
		}
		views.MarkDirty()
	}
	t.par.OnCWD = func(cwd string) {
		t.CWD = cwd
		cb := t.OnCWDChange
		if cb != nil {
			views.CallSoon(func() { cb(cwd) })
		}
		views.MarkDirty()
	}
	t.par.OnBell = func() {
		cb := t.OnBell
		if cb != nil {
			views.CallSoon(cb)
		}
	}
	return t
}

// GetTypeID for serial registry.
func (t *Terminal) GetTypeID() string { return "terminal" }

// Start spawns the child process. The reader goroutine begins
// immediately; the view will start drawing the first output as soon
// as the child writes anything.
//
// If env is nil and t.Env is unset, the current process environment is
// used (with TERM patched to "xterm-256color" so curses-based programs
// negotiate reasonable capabilities). t.WorkingDir, if non-empty, is
// the child's initial cwd; empty means inherit. t.ScrollbackLines, if
// non-zero, replaces the default 2000-line cap.
func (t *Terminal) Start(name string, args []string, env []string) error {
	if t.ScrollbackLines > 0 {
		t.mu.Lock()
		t.buf.scrollbackCap = t.ScrollbackLines
		t.mu.Unlock()
	}
	if t.Env != nil {
		env = t.Env
	}
	if env == nil {
		env = append(env, os.Environ()...)
		env = append(env, "TERM=xterm-256color")
	}
	p, err := startPTY(name, args, env, t.WorkingDir, t.Size.X, t.Size.Y)
	if err != nil {
		return err
	}
	t.pty = p
	go t.readLoop()
	go t.waitLoop()
	return nil
}

// SetEnv stores env for the next Start() call. Has no effect after the
// child has been launched.
func (t *Terminal) SetEnv(env []string) { t.Env = env }

// SetWorkingDir stores dir for the next Start() call. Has no effect
// after the child has been launched.
func (t *Terminal) SetWorkingDir(dir string) { t.WorkingDir = dir }

// SetFocused tells the Terminal whether its parent considers this pane
// focused. Drives cursor suppression so unfocused panes don't blink.
// Defaults to true; single-pane callers needn't touch this.
func (t *Terminal) SetFocused(v bool) {
	t.mu.Lock()
	changed := t.focused != v
	t.focused = v
	t.mu.Unlock()
	if changed {
		views.MarkDirty()
	}
}

// SuspendMouseForwarding toggles forwarding of mouse events to the
// child PTY. While suspended, the local selection / scroll-wheel paths
// continue to work but no SGR-1006 sequences are written. Used by
// hosts that want to take over the mouse temporarily (e.g., a copy
// mode or pane-resize mode) without the inner shell seeing events.
func (t *Terminal) SuspendMouseForwarding(v bool) {
	t.mu.Lock()
	t.mouseSuspended = v
	t.mu.Unlock()
}

// BracketedPaste reports whether the inner program has enabled DEC
// mode ?2004 (bracketed paste). Hosts use this to decide whether to
// wrap a clipboard paste in the escape brackets.
func (t *Terminal) BracketedPaste() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.bracketedPaste
}

// Paste writes text to the PTY, wrapping in bracketed-paste escapes
// (\x1b[200~ … \x1b[201~) when the inner program has enabled DEC mode
// ?2004. Without bracketing, multiline pastes execute line-by-line in
// shells — broken UX.
func (t *Terminal) Paste(text string) error {
	if t.pty == nil {
		return nil
	}
	t.mu.Lock()
	wrap := t.buf.bracketedPaste
	t.mu.Unlock()
	var payload []byte
	if wrap {
		payload = make([]byte, 0, len(text)+12)
		payload = append(payload, "\x1b[200~"...)
		payload = append(payload, text...)
		payload = append(payload, "\x1b[201~"...)
	} else {
		payload = []byte(text)
	}
	_, err := t.pty.Write(payload)
	return err
}

// ScrollbackText returns the entire buffer (scrollback + live region)
// as plain text. Trailing blanks per row are trimmed; rows are
// separated by '\n'. Used by "save scrollback to file" commands.
func (t *Terminal) ScrollbackText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	total := len(t.buf.scrollback) + len(t.buf.cells)
	if total == 0 {
		return ""
	}
	lastCol := t.buf.W - 1
	if lastCol < 0 {
		lastCol = 0
	}
	return t.extractText(cellPos{row: 0, col: 0}, cellPos{row: total - 1, col: lastCol})
}

// SelectionText returns the currently selected region, or "" when
// nothing is selected.
func (t *Terminal) SelectionText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.extractSelectionText()
}

// EnterCopyMode parks a movable cursor at the bottom-right of the
// currently-visible viewport and flags the widget as in copy mode.
// Subsequent MoveCopyCursor calls walk the cursor through scrollback
// and the live region, optionally extending a selection anchored by
// ToggleCopyAnchor. The mouse-drag selection path is unaffected.
func (t *Terminal) EnterCopyMode() {
	t.mu.Lock()
	// Visible top row in absolute coords:
	// scrollbackLen - scrollOffset. Bottom is top + H - 1, clamped to
	// the last valid row index across scrollback + live cells.
	topAbs := len(t.buf.scrollback) - t.buf.scrollOffset
	bottomAbs := topAbs + t.buf.H - 1
	maxAbs := len(t.buf.scrollback) + len(t.buf.cells) - 1
	if bottomAbs > maxAbs {
		bottomAbs = maxAbs
	}
	if bottomAbs < 0 {
		bottomAbs = 0
	}
	col := t.buf.W - 1
	if col < 0 {
		col = 0
	}
	t.copyCursor = cellPos{row: bottomAbs, col: col}
	t.copying = true
	t.mu.Unlock()
	views.MarkDirty()
}

// ExitCopyMode clears the copy-mode flag. Selection state (anchor +
// endpoint) is left intact so a re-EnterCopyMode picks up where it
// left off; the cursor glyph just stops drawing in the meantime.
func (t *Terminal) ExitCopyMode() {
	t.mu.Lock()
	wasCopying := t.copying
	t.copying = false
	t.mu.Unlock()
	if wasCopying {
		views.MarkDirty()
	}
}

// MoveCopyCursor advances the copy-mode cursor by (dx, dy). Horizontal
// motion clamps at row boundaries (no wrap to next line). Vertical
// motion clamps to the scrollback extent on top and the last live row
// on bottom. If the cursor leaves the visible viewport, scrollOffset
// is adjusted so it stays in view. No-op when not in copy mode.
//
// When the selection anchor is set (selStartAbs != zero), selEndAbs
// follows the cursor so the highlight extends as the user moves.
func (t *Terminal) MoveCopyCursor(dx, dy int) {
	t.mu.Lock()
	if !t.copying {
		t.mu.Unlock()
		return
	}
	maxRow := len(t.buf.scrollback) + len(t.buf.cells) - 1
	if maxRow < 0 {
		maxRow = 0
	}
	// Vertical: clamp [0, maxRow].
	newRow := t.copyCursor.row + dy
	if newRow < 0 {
		newRow = 0
	}
	if newRow > maxRow {
		newRow = maxRow
	}
	// Horizontal: clamp within the row's width — no wrap.
	maxCol := t.buf.W - 1
	if maxCol < 0 {
		maxCol = 0
	}
	newCol := t.copyCursor.col + dx
	if newCol < 0 {
		newCol = 0
	}
	if newCol > maxCol {
		newCol = maxCol
	}
	t.copyCursor = cellPos{row: newRow, col: newCol}
	// Keep cursor in view: top visible row = len(scrollback) - scrollOffset,
	// bottom = top + H - 1. Adjust scrollOffset so the cursor sits inside.
	topAbs := len(t.buf.scrollback) - t.buf.scrollOffset
	bottomAbs := topAbs + t.buf.H - 1
	if newRow < topAbs {
		// Cursor moved above viewport — increase scrollOffset.
		t.buf.scrollOffset = len(t.buf.scrollback) - newRow
		if t.buf.scrollOffset > len(t.buf.scrollback) {
			t.buf.scrollOffset = len(t.buf.scrollback)
		}
	} else if newRow > bottomAbs {
		// Cursor moved below viewport — decrease scrollOffset.
		t.buf.scrollOffset = len(t.buf.scrollback) - newRow + t.buf.H - 1
		if t.buf.scrollOffset < 0 {
			t.buf.scrollOffset = 0
		}
	}
	// If an anchor exists, the selection's far end follows the cursor.
	if t.selecting {
		t.selEndAbs = t.copyCursor
	}
	t.mu.Unlock()
	views.MarkDirty()
}

// ToggleCopyAnchor pins the current copy cursor as the selection
// start (and starts extending the selection), or clears the anchor on
// a second call. No-op when not in copy mode.
func (t *Terminal) ToggleCopyAnchor() {
	t.mu.Lock()
	if !t.copying {
		t.mu.Unlock()
		return
	}
	if t.selecting {
		// Clear the anchor and the selection.
		t.selecting = false
		t.selStartAbs = cellPos{}
		t.selEndAbs = cellPos{}
	} else {
		t.selStartAbs = t.copyCursor
		t.selEndAbs = t.copyCursor
		t.selecting = true
	}
	t.mu.Unlock()
	views.MarkDirty()
}

// CopySelection extracts the currently anchored range, pushes it to
// the clipboard via OSC 52, and returns the text plus true. When
// there's no anchor (or the selection is empty), returns ("", false).
// Does NOT exit copy mode — the cursor stays put so the user can
// expand the selection and copy again.
func (t *Terminal) CopySelection() (string, bool) {
	t.mu.Lock()
	text := t.extractSelectionText()
	t.mu.Unlock()
	if text == "" {
		return "", false
	}
	clipboard.SetText(text)
	return text, true
}

// Stop kills the child and tears down the PTY. Safe to call multiple
// times.
func (t *Terminal) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	if t.pty != nil {
		_ = t.pty.Close()
	}
}

// PID returns the child process ID, or 0 if the PTY isn't running.
// Useful for diagnostic banners ("launched X as PID Y") so a silent
// child can be confirmed alive (or not) from another shell.
func (t *Terminal) PID() int {
	if t.pty == nil || t.pty.cmd == nil || t.pty.cmd.Process == nil {
		return 0
	}
	return t.pty.cmd.Process.Pid
}

// readLoop pumps PTY output into the parser. Runs until the PTY
// closes (typically because the child exited). On exit / error we
// inject a visible "[pty closed: <err>]" line so a silent child
// process doesn't look like a hung terminal.
//
// OnActivity fires at most once per 500ms — frequent enough for a
// flashing status-bar dot, sparse enough not to thrash the host.
func (t *Terminal) readLoop() {
	const activityDebounce = 500 * time.Millisecond
	buf := make([]byte, 4096)
	for {
		n, err := t.pty.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			// OnFeed runs synchronously between the PTY read and the
			// parser. Hosts can use it for asciicast recording or
			// byte-level effects (e.g., :rot13). The hook is on the
			// hot path — keep the callback cheap. nil-safe.
			if t.OnFeed != nil {
				chunk = t.OnFeed(chunk)
			}
			t.mu.Lock()
			t.par.Feed(chunk)
			// Snapshot callback + debounce timestamp under the lock
			// so concurrent assignment to t.OnActivity from host
			// code is race-free.
			now := time.Now()
			var activityCB func()
			if t.OnActivity != nil && now.Sub(t.lastActivity) >= activityDebounce {
				t.lastActivity = now
				activityCB = t.OnActivity
			}
			t.mu.Unlock()
			if activityCB != nil {
				views.CallSoon(activityCB)
			}
			views.MarkDirty()
		}
		if err != nil {
			msg := "\r\n\x1b[31m[pty closed"
			if err.Error() != "EOF" {
				msg += ": " + err.Error()
			}
			msg += "]\x1b[0m\r\n"
			t.mu.Lock()
			t.par.Feed([]byte(msg))
			t.mu.Unlock()
			views.MarkDirty()
			return
		}
	}
}

// waitLoop watches for child-process exit so we can fire OnExit.
//
// OnExit runs on the UI goroutine via CallSoon so host code can
// safely manipulate views (e.g., close the parent window) without
// having to marshal itself.
func (t *Terminal) waitLoop() {
	err := t.pty.Wait()
	t.mu.Lock()
	cb := t.OnExit
	t.mu.Unlock()
	if cb != nil {
		views.CallSoon(func() { cb(err) })
	}
	views.MarkDirty()
}

// HandleEvent dispatches FV events:
//
//   - CmClose triggers Stop() so a window close button reliably tears
//     down the child process instead of leaking a zombie.
//   - Shift+PageUp / Shift+PageDn / Home / End scroll the scrollback
//     view. Plain PageUp/Dn go to the PTY (apps like less expect them).
//   - Mouse wheel scrolls the scrollback when there's history to show;
//     otherwise the wheel falls through to mouse forwarding.
//   - Other key events translate to ANSI bytes and write to the PTY.
//     Any keystroke snaps the viewport back to the live cursor.
//   - Mouse events forward to the PTY as SGR-1006 sequences, but only
//     when the inner program has enabled a mouse-tracking DEC mode.
func (t *Terminal) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvCommand && ev.Command == consts.CmClose {
		t.Stop()
		return
	}
	if t.pty == nil {
		return
	}
	switch ev.What {
	case consts.EvKeyDown:
		t.handleKey(ev)
	case consts.EvMouseDown, consts.EvMouseUp, consts.EvMouseMove, consts.EvMouseWheel:
		t.handleMouse(ev)
	}
}

// handleKey is the keyboard branch of HandleEvent.
//
// Scrollback navigation uses Shift-modified nav keys: Shift+PageUp /
// Shift+PageDn / Shift+Home / Shift+End. Plain nav keys (incl. mouse
// wheel handled separately) are forwarded to the PTY because many TUIs
// genuinely consume them.
//
// While in scrollback (scrollOffset > 0), '/' opens a search mode
// that captures typed characters into searchQuery, jumps to the next
// match on Enter, and n / N step between matches.
func (t *Terminal) handleKey(ev *drivers.Event) {
	if t.searching {
		t.handleSearchKey(ev)
		return
	}
	shift := ev.KeyShift&consts.KbLeftShift != 0
	if shift {
		switch ev.KeyCode {
		case consts.KbPgUp:
			t.scrollBy(t.Size.Y / 2)
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		case consts.KbPgDn:
			t.scrollBy(-t.Size.Y / 2)
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		case consts.KbHome:
			t.mu.Lock()
			t.buf.scrollOffset = len(t.buf.scrollback)
			t.mu.Unlock()
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		case consts.KbEnd:
			t.mu.Lock()
			t.buf.snapToBottom()
			t.mu.Unlock()
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		}
	}
	// '/' while looking at scrollback enters search mode. While at
	// the live bottom, '/' is just a slash and goes to the PTY.
	t.mu.Lock()
	atScrollback := t.buf.scrollOffset > 0
	t.mu.Unlock()
	if atScrollback && ev.UnicodeChar == '/' && ev.KeyShift&consts.KbCtrlShift == 0 {
		t.searching = true
		t.searchQuery = t.searchQuery[:0]
		views.MarkDirty()
		t.ClearEvent(ev)
		return
	}
	// 'n' / 'N' step through previous results without re-entering
	// search mode. Only meaningful when we already have a query.
	if atScrollback && len(t.searchQuery) > 0 && ev.UnicodeChar > 0 {
		switch ev.UnicodeChar {
		case 'n':
			t.searchStep(+1)
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		case 'N':
			t.searchStep(-1)
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		}
	}
	// Live-typing event: snap the viewport back so the cursor is
	// visible while the user is typing.
	t.mu.Lock()
	t.buf.snapToBottom()
	t.mu.Unlock()
	bytes := keyToBytes(ev)
	if len(bytes) == 0 {
		return
	}
	_, _ = t.pty.Write(bytes)
	t.ClearEvent(ev)
}

// handleSearchKey is the input-line for scrollback search. Esc bails
// out, Enter commits and jumps, Backspace erases. Everything else
// printable is appended to the query — there's no per-keystroke
// preview (would interact awkwardly with the cursor-hidden scrollback
// state); a single Enter is the commit.
func (t *Terminal) handleSearchKey(ev *drivers.Event) {
	switch ev.KeyCode {
	case consts.KbEsc:
		t.searching = false
		t.searchQuery = t.searchQuery[:0]
		views.MarkDirty()
		t.ClearEvent(ev)
		return
	case consts.KbEnter:
		t.searching = false
		t.searchStep(0) // anchor at current position
		views.MarkDirty()
		t.ClearEvent(ev)
		return
	case consts.KbBack:
		if len(t.searchQuery) > 0 {
			t.searchQuery = t.searchQuery[:len(t.searchQuery)-1]
		}
		views.MarkDirty()
		t.ClearEvent(ev)
		return
	}
	if r := ev.UnicodeChar; r >= ' ' && r < 0x7F {
		t.searchQuery = append(t.searchQuery, byte(r))
		views.MarkDirty()
		t.ClearEvent(ev)
	}
}

// searchStep jumps scrollOffset to the next/prev line containing the
// current query. delta=+1 goes to an OLDER match (further into
// scrollback), -1 to a newer one, 0 finds the closest match to the
// current viewport top.
func (t *Terminal) searchStep(delta int) {
	if len(t.searchQuery) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	needle := strings.ToLower(string(t.searchQuery))
	// Build a virtual address space: scrollback rows 0..len-1, then
	// live rows len..len+H-1. The user is currently looking at row
	// (len(scrollback) - scrollOffset). We search relative to that.
	allRows := func(idx int) []cell {
		if idx < 0 {
			return nil
		}
		if idx < len(t.buf.scrollback) {
			return t.buf.scrollback[idx]
		}
		idx -= len(t.buf.scrollback)
		if idx < 0 || idx >= len(t.buf.cells) {
			return nil
		}
		return t.buf.cells[idx]
	}
	total := len(t.buf.scrollback) + len(t.buf.cells)
	start := len(t.buf.scrollback) - t.buf.scrollOffset
	if delta == 0 {
		// Anchor: start from the top of viewport, search OLDER first.
		for i := start - 1; i >= 0; i-- {
			if rowContains(allRows(i), needle) {
				t.buf.scrollOffset = len(t.buf.scrollback) - i
				return
			}
		}
		return
	}
	if delta > 0 {
		for i := start - 1; i >= 0; i-- {
			if rowContains(allRows(i), needle) {
				t.buf.scrollOffset = len(t.buf.scrollback) - i
				return
			}
		}
	} else {
		for i := start + 1; i < total; i++ {
			if rowContains(allRows(i), needle) {
				off := len(t.buf.scrollback) - i
				if off < 0 {
					off = 0
				}
				t.buf.scrollOffset = off
				return
			}
		}
	}
}

// cellRange is a [start, end) range of cell indices used for
// search-match highlights inside a single row.
type cellRange struct{ start, end int }

// searchMatchesInRow returns all non-overlapping occurrences of
// `needle` (already lower-cased) in the row's case-folded text, as
// cell-index ranges suitable for highlighting. Empty needle / empty
// row returns nil.
//
// Byte offsets into the haystack are mapped back to cell indices via
// a parallel byte→cell table built from the row's cell list — this
// lets us highlight matches that contain multi-byte (UTF-8) runes
// without throwing the offsets off.
func searchMatchesInRow(row []cell, needle string) []cellRange {
	if needle == "" || len(row) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.Grow(len(row))
	byteToCell := make([]int, 0, len(row)+8)
	for i, c := range row {
		startByte := sb.Len()
		if c.Ch == 0 {
			sb.WriteByte(' ')
		} else {
			sb.WriteRune(c.Ch)
		}
		for b := startByte; b < sb.Len(); b++ {
			byteToCell = append(byteToCell, i)
		}
	}
	hay := strings.ToLower(sb.String())
	var out []cellRange
	pos := 0
	for pos <= len(hay)-len(needle) {
		idx := strings.Index(hay[pos:], needle)
		if idx < 0 {
			break
		}
		matchByte := pos + idx
		lastByte := matchByte + len(needle) - 1
		if lastByte >= len(byteToCell) {
			break
		}
		out = append(out, cellRange{
			start: byteToCell[matchByte],
			end:   byteToCell[lastByte] + 1,
		})
		pos = matchByte + len(needle)
	}
	return out
}

// inAnyRange reports whether x falls inside any of the given ranges.
func inAnyRange(ranges []cellRange, x int) bool {
	for _, r := range ranges {
		if x >= r.start && x < r.end {
			return true
		}
	}
	return false
}

// rowContains reports whether the case-folded text of row contains
// needle (already lowercased). Skips empty rows quickly.
func rowContains(row []cell, needle string) bool {
	if len(row) == 0 {
		return false
	}
	var sb strings.Builder
	sb.Grow(len(row))
	for _, c := range row {
		if c.Ch == 0 {
			sb.WriteByte(' ')
		} else {
			sb.WriteRune(c.Ch)
		}
	}
	return strings.Contains(strings.ToLower(sb.String()), needle)
}

// handleMouse routes mouse events through three branches:
//
//  1. Mouse wheel: scrolls the scrollback while there's history.
//     Once at the bottom, wheel passes through to mouse-forwarding
//     for apps like less / vim.
//  2. Drag-to-select: when the inner program hasn't enabled mouse
//     tracking, OR the user holds Shift, a left-button drag
//     highlights cells and copies the text on release. Selection
//     state stays visible until the next plain click clears it.
//  3. Mouse forwarding: events go to the PTY as SGR-1006 sequences
//     when the inner program asked for mouse tracking (via DEC
//     private modes ?1000 / ?1002 / ?1003 / ?1006).
func (t *Terminal) handleMouse(ev *drivers.Event) {
	if ev.What == consts.EvMouseWheel {
		if ev.Buttons&consts.MbScrollWheelUp != 0 {
			t.scrollBy(3)
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		}
		// Wheel-down: scroll history toward the live tail when we
		// have any. At the live bottom, fall through so apps that
		// enabled mouse tracking (less, vim, htop, …) still see the
		// wheel notch — but encoded as a wheel event via SGR-1006.
		t.mu.Lock()
		atBottom := t.buf.scrollOffset == 0
		t.mu.Unlock()
		if !atBottom {
			t.scrollBy(-3)
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		}
		// Fall through into the mouse-forwarding block below so the
		// inner program receives the wheel notch.
	}

	// Selection logic. Triggers when EITHER the inner program isn't
	// reading mouse, OR the user has Shift held (which is the
	// conventional override). On non-Shift left-button down/move/up
	// in pure forwarding mode, fall through to the PTY.
	t.mu.Lock()
	tracking := t.buf.mouseX10 || t.buf.mouseBtnEv || t.buf.mouseAnyEv
	t.mu.Unlock()
	shift := ev.KeyShift&consts.KbLeftShift != 0
	selectMode := !tracking || shift
	if selectMode && ev.Buttons&(consts.MbLeftButton) != 0 || (t.selecting && ev.What != consts.EvMouseDown && ev.Buttons == 0) {
		t.handleSelectionEvent(ev)
		return
	}
	if selectMode && ev.What == consts.EvMouseDown && ev.Buttons == 0 {
		// A bare click in select mode without selection drag started:
		// clear any existing highlight.
		t.mu.Lock()
		had := t.selStartAbs != t.selEndAbs
		t.selStartAbs, t.selEndAbs = cellPos{}, cellPos{}
		t.selecting = false
		t.mu.Unlock()
		if had {
			views.MarkDirty()
		}
		t.ClearEvent(ev)
		return
	}

	// Mouse forwarding to the PTY.
	t.mu.Lock()
	defer t.mu.Unlock()
	if !tracking || t.mouseSuspended {
		return
	}
	if ev.What == consts.EvMouseMove && !t.buf.mouseBtnEv && !t.buf.mouseAnyEv {
		return
	}
	local := t.MakeLocal(ev.Where)
	if local.X < 0 || local.Y < 0 || local.X >= t.Size.X || local.Y >= t.Size.Y {
		return
	}
	seq := encodeMouseSGR(ev, local.X, local.Y, t.buf.mouseSGR)
	if seq == "" {
		return
	}
	_, _ = t.pty.Write([]byte(seq))
	t.ClearEvent(ev)
}

// handleSelectionEvent drives the drag-to-select state machine. Each
// event either starts a new drag (MouseDown), extends an in-flight
// drag (MouseMove / MouseDown with button still held), or finishes
// one (MouseUp) and copies the selection.
func (t *Terminal) handleSelectionEvent(ev *drivers.Event) {
	local := t.MakeLocal(ev.Where)
	if local.X < 0 {
		local.X = 0
	}
	if local.X >= t.Size.X {
		local.X = t.Size.X - 1
	}
	if local.Y < 0 {
		local.Y = 0
	}
	if local.Y >= t.Size.Y {
		local.Y = t.Size.Y - 1
	}
	t.mu.Lock()
	abs := cellPos{
		row: len(t.buf.scrollback) - t.buf.scrollOffset + local.Y,
		col: local.X,
	}
	switch ev.What {
	case consts.EvMouseDown:
		t.selStartAbs = abs
		t.selEndAbs = abs
		t.selecting = true
	case consts.EvMouseMove:
		if t.selecting {
			t.selEndAbs = abs
		}
	case consts.EvMouseUp:
		if t.selecting {
			t.selEndAbs = abs
			t.selecting = false
			text := t.extractSelectionText()
			t.mu.Unlock()
			if text != "" {
				clipboard.SetText(text)
			}
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		}
	}
	t.mu.Unlock()
	views.MarkDirty()
	t.ClearEvent(ev)
}

// extractSelectionText returns the current selection as text, or ""
// when nothing is selected. Must be called with t.mu held.
func (t *Terminal) extractSelectionText() string {
	if t.selStartAbs == t.selEndAbs {
		return ""
	}
	return t.extractText(t.selStartAbs, t.selEndAbs)
}

// extractText walks the cells inside the [a, b] rectangle (in absolute
// scrollback-plus-live coordinates) and joins them into a single
// multi-line string. a and b need not be normalized — extractText
// sorts them top-left → bottom-right. Trailing blanks per row are
// trimmed (except on the final row). Must be called with t.mu held.
func (t *Terminal) extractText(a, b cellPos) string {
	if a.row > b.row || (a.row == b.row && a.col > b.col) {
		a, b = b, a
	}
	rowOf := func(idx int) []cell {
		if idx < 0 {
			return nil
		}
		if idx < len(t.buf.scrollback) {
			return t.buf.scrollback[idx]
		}
		idx -= len(t.buf.scrollback)
		if idx < 0 || idx >= len(t.buf.cells) {
			return nil
		}
		return t.buf.cells[idx]
	}
	var sb strings.Builder
	for r := a.row; r <= b.row; r++ {
		row := rowOf(r)
		if row == nil {
			continue
		}
		startCol := 0
		endCol := len(row)
		if r == a.row {
			startCol = a.col
		}
		if r == b.row {
			endCol = b.col + 1
			if endCol > len(row) {
				endCol = len(row)
			}
		}
		// Trim trailing blanks on the last column of every row except
		// the bottom — terminal output usually pads with blanks past
		// EOL and the user doesn't want them in the clipboard.
		lineEnd := endCol
		if r != b.row {
			for lineEnd > startCol && (row[lineEnd-1].Ch == ' ' || row[lineEnd-1].Ch == 0) {
				lineEnd--
			}
		}
		for c := startCol; c < lineEnd; c++ {
			cl := row[c]
			if cl.Ch == 0 {
				sb.WriteByte(' ')
			} else {
				sb.WriteRune(cl.Ch)
			}
		}
		if r != b.row {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// scrollBy delegates to buffer under the lock and marks dirty.
func (t *Terminal) scrollBy(delta int) {
	t.mu.Lock()
	t.buf.scrollByLines(delta)
	t.mu.Unlock()
}

// StartScrollbackSearch puts the widget into the same "/" search mode
// the user gets by pressing '/' while viewing scrollback, but driven
// programmatically — for hosts (fvmux, …) that bind their own
// "find-in-scrollback" shortcut. After this returns, typed characters
// go into the search query, Enter jumps to the first match, and n/N
// step. CancelScrollbackSearch exits the mode without committing.
//
// If the viewport is currently at the live bottom (scrollOffset == 0),
// we nudge into scrollback by one line so the search-mode UI is
// visible — searching at the live tail has no history to scan.
func (t *Terminal) StartScrollbackSearch() {
	t.mu.Lock()
	if t.buf.scrollOffset == 0 {
		// Don't reuse scrollBy here: it acquires t.mu and would
		// deadlock. The buffer helper mutates without locking.
		t.buf.scrollByLines(1)
	}
	t.searching = true
	t.searchQuery = t.searchQuery[:0]
	t.mu.Unlock()
	views.MarkDirty()
}

// CancelScrollbackSearch exits search mode and clears the query.
// Safe to call when not searching (no-op).
func (t *Terminal) CancelScrollbackSearch() {
	t.mu.Lock()
	wasSearching := t.searching || len(t.searchQuery) > 0
	t.searching = false
	t.searchQuery = t.searchQuery[:0]
	t.mu.Unlock()
	if wasSearching {
		views.MarkDirty()
	}
}

// ChangeBounds rezises the buffer + the underlying PTY.
func (t *Terminal) ChangeBounds(r geom.Rect) {
	t.SetBounds(r)
	t.mu.Lock()
	t.buf.resize(t.Size.X, t.Size.Y)
	t.mu.Unlock()
	if t.pty != nil {
		_ = t.pty.Resize(t.Size.X, t.Size.Y)
	}
}

// Draw paints either the live buffer or a scrollback view, depending
// on the buffer's scrollOffset. When viewing scrollback the cursor is
// hidden — its position is in the live state and showing it inside
// historical content would be misleading.
func (t *Terminal) Draw() {
	t.mu.Lock()
	defer t.mu.Unlock()
	selA, selB, hasSel := t.normalizedSelection()
	// Pre-build the search needle once per Draw. Search highlights
	// fire when a query is present, regardless of whether the input
	// line (`searching`) is currently up — n/N navigation after Enter
	// clears `searching` but the user expects matches to stay lit so
	// they can spot subsequent hits as they step through.
	var needle string
	if len(t.searchQuery) > 0 {
		needle = strings.ToLower(string(t.searchQuery))
	}
	highlightAttr := types.MakeAttr(0x00, 0x0E) // black on yellow — the classic "found" cue
	for y := 0; y < t.Size.Y; y++ {
		row := screen.MakeDrawBuffer(t.Size.X)
		src := t.buf.rowAt(y)
		// Absolute row index for selection comparison.
		absRow := len(t.buf.scrollback) - t.buf.scrollOffset + y
		// Match ranges for this row's source cells. We compute them
		// up front because the [x] loop needs cell-index info that's
		// awkward to derive inside the per-cell switch.
		var matches []cellRange
		if needle != "" {
			matches = searchMatchesInRow(src, needle)
		}
		for x := 0; x < t.Size.X; x++ {
			var cl cell
			if x < len(src) {
				cl = src[x]
			} else {
				cl = blankCell()
			}
			row[x] = cl.toDrawCell(t.DefaultFG, t.DefaultBG)
			if row[x].Ch == "" {
				row[x].Ch = " "
			}
			// Search-match highlight FIRST so selection overrides it
			// on overlap — selection is user-driven and should win.
			if inAnyRange(matches, x) {
				row[x].Attr = highlightAttr
			}
			if hasSel && inSelection(absRow, x, selA, selB) {
				// Reverse video for selected cells. Preserve the
				// original glyph; just flip foreground/background
				// in the packed attr.
				row[x].Attr = reverseAttr(row[x].Attr)
			}
			// Copy-mode cursor: an extra reverse pass at the cursor
			// cell so it stays visible even on top of a selection
			// highlight. Two reversals cancel — so we OR-in a bright
			// fg via re-reversing the (already-reversed) cell.
			if t.copying && absRow == t.copyCursor.row && x == t.copyCursor.col {
				row[x].Attr = reverseAttr(row[x].Attr)
			}
		}
		t.WriteLine(0, y, t.Size.X, 1, row)
	}
	// Status row: search prompt while searching, scroll position
	// indicator otherwise (only when in scrollback).
	if t.searching {
		t.drawStatusRow("/" + string(t.searchQuery))
		t.Cursor = geom.Point{X: 1 + len(t.searchQuery), Y: t.Size.Y - 1}
		t.State |= consts.SfCursorVis
	} else if t.buf.scrollOffset > 0 {
		hint := "-- scrollback "
		if len(t.searchQuery) > 0 {
			hint += "(n / N to step, / to search again) "
		} else {
			hint += "(/ to search, Shift-End to bottom) "
		}
		t.drawStatusRow(hint)
		t.State &^= consts.SfCursorVis
	} else if t.buf.cursorVisible && t.focused {
		t.Cursor = geom.Point{X: t.buf.cursorC, Y: t.buf.cursorR}
		t.State |= consts.SfCursorVis
	} else {
		t.State &^= consts.SfCursorVis
	}
}

// normalizedSelection returns the selection sorted top-left → bottom-
// right, plus a bool indicating whether anything is selected at all
// (an unstarted or zero-extent selection returns hasSel=false). Must
// be called with t.mu held.
func (t *Terminal) normalizedSelection() (a, b cellPos, hasSel bool) {
	if t.selStartAbs == t.selEndAbs {
		return cellPos{}, cellPos{}, false
	}
	a, b = t.selStartAbs, t.selEndAbs
	if a.row > b.row || (a.row == b.row && a.col > b.col) {
		a, b = b, a
	}
	return a, b, true
}

// inSelection reports whether absolute (row, col) is inside the
// normalized [a, b] selection rectangle. Selection is linear (newline-
// wrapped) rather than block-rectangular — same shape as iTerm2/xterm.
func inSelection(row, col int, a, b cellPos) bool {
	if row < a.row || row > b.row {
		return false
	}
	if row == a.row && col < a.col {
		return false
	}
	if row == b.row && col > b.col {
		return false
	}
	return true
}

// reverseAttr flips the foreground and background nibbles of a TV-
// packed attribute byte, leaving any higher bits (e.g., bright) alone.
func reverseAttr(attr uint16) uint16 {
	fg := attr & 0x000F
	bg := (attr >> 8) & 0x000F
	// Preserve bright/intensity bits in the high nibble of fg.
	fgRest := attr & 0x00F0
	bgRest := (attr >> 8) & 0x00F0
	return bg | (bgRest) | (fg << 8) | (fgRest << 8)
}

// drawStatusRow overlays a one-line status string on the bottom row,
// reverse-video so it stands out against terminal output.
func (t *Terminal) drawStatusRow(s string) {
	if t.Size.Y <= 0 {
		return
	}
	w := t.Size.X
	buf := screen.MakeDrawBuffer(w)
	attr := types.MakeAttr(0x0F, 0x04) // bright white on red
	for x := 0; x < w; x++ {
		screen.DrawCell(buf, x, " ", attr)
	}
	screen.DrawStr(buf, 1, s, attr)
	t.WriteLine(0, t.Size.Y-1, w, 1, buf)
}

// Write injects bytes into the parser as if they came from the PTY.
// Useful for tests and for replaying a captured session — not used by
// the live view.
func (t *Terminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.par.Feed(p)
	return len(p), nil
}

// Ensure DrawCell satisfies the basic Ch-set assumption.
var _ = types.DrawCell{}
