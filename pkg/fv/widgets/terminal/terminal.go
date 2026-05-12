package terminal

import (
	"os"
	"strings"
	"sync"

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

	// Title is the latest OSC 0/1/2 string the child sent. Empty
	// until the child sets one.
	Title string

	// OnTitle, if non-nil, fires from the reader goroutine whenever
	// Title changes — typically used to update the host window's
	// caption. Goroutine-safe: callbacks should not block.
	OnTitle func(string)

	// OnExit fires once when the child process exits (also from the
	// reader goroutine). Used to auto-close the wrapping window.
	OnExit func(error)

	buf    *buffer
	par    *parser
	pty    *ptyHandle
	mu     sync.Mutex
	closed bool

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
	}
	t.SetSelf(t)
	t.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	t.Options |= consts.OfSelectable
	t.EventMask = consts.EvKeyDown | consts.EvCommand | consts.EvMouseDown | consts.EvMouseUp | consts.EvMouseMove
	// Claim raw keyboard so Program.HandleEvent doesn't fold Ctrl+C
	// into Copy etc. while we're focused. Ctrl+C, Ctrl+X, Ctrl+V,
	// and the F-keys reach the inner shell instead.
	t.State |= consts.SfRawKeys
	t.buf = newBuffer(t.Size.X, t.Size.Y)
	t.par = newParser(t.buf)
	// IMPORTANT: OnTitle fires from inside par.Feed, which is always
	// called with t.mu already held (either by readLoop or by Write).
	// Acquiring t.mu here would self-deadlock — when we tried, zsh
	// hung in tcsetattr (TCSADRAIN waiting on a PTY buffer the reader
	// could no longer drain) and the whole UI froze. Title is set
	// under the caller's lock, no further sync needed.
	t.par.OnTitle = func(title string) {
		t.Title = title
		if t.OnTitle != nil {
			t.OnTitle(title)
		}
		views.MarkDirty()
	}
	return t
}

// GetTypeID for serial registry.
func (t *Terminal) GetTypeID() string { return "terminal" }

// Start spawns the child process. The reader goroutine begins
// immediately; the view will start drawing the first output as soon
// as the child writes anything.
//
// If env is nil, the current process environment is used (with TERM
// patched to "xterm-256color" so curses-based programs negotiate
// reasonable capabilities).
func (t *Terminal) Start(name string, args []string, env []string) error {
	if env == nil {
		env = append(env, os.Environ()...)
		env = append(env, "TERM=xterm-256color")
	}
	p, err := startPTY(name, args, env, t.Size.X, t.Size.Y)
	if err != nil {
		return err
	}
	t.pty = p
	go t.readLoop()
	go t.waitLoop()
	return nil
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
func (t *Terminal) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := t.pty.Read(buf)
		if n > 0 {
			t.mu.Lock()
			t.par.Feed(buf[:n])
			t.mu.Unlock()
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
func (t *Terminal) waitLoop() {
	err := t.pty.Wait()
	if t.OnExit != nil {
		t.OnExit(err)
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
	case consts.EvMouseDown, consts.EvMouseUp, consts.EvMouseMove:
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
	if ev.What == consts.EvMouseDown {
		if ev.Buttons&consts.MbScrollWheelUp != 0 {
			t.scrollBy(3)
			views.MarkDirty()
			t.ClearEvent(ev)
			return
		}
		if ev.Buttons&consts.MbScrollWheelDown != 0 {
			t.mu.Lock()
			atBottom := t.buf.scrollOffset == 0
			t.mu.Unlock()
			if !atBottom {
				t.scrollBy(-3)
				views.MarkDirty()
				t.ClearEvent(ev)
				return
			}
			// Falls through to forwarding below.
		}
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
	if !tracking {
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

// extractSelectionText walks the cells inside the current selection
// rectangle and joins them into a single multi-line string. Must be
// called with t.mu held. Returns "" for an empty / degenerate
// selection (start == end on the same cell).
func (t *Terminal) extractSelectionText() string {
	a, b := t.selStartAbs, t.selEndAbs
	if a == b {
		return ""
	}
	// Normalize: a is upper-left, b is lower-right by row order.
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
	for y := 0; y < t.Size.Y; y++ {
		row := screen.MakeDrawBuffer(t.Size.X)
		src := t.buf.rowAt(y)
		// Absolute row index for selection comparison.
		absRow := len(t.buf.scrollback) - t.buf.scrollOffset + y
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
			if hasSel && inSelection(absRow, x, selA, selB) {
				// Reverse video for selected cells. Preserve the
				// original glyph; just flip foreground/background
				// in the packed attr.
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
	} else if t.buf.cursorVisible {
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

// Buffer returns the underlying buffer for tests/inspection. Not
// goroutine-safe: callers should hold no lock and assume the buffer
// can mutate from the read loop concurrently.
func (t *Terminal) Buffer() *buffer { return t.buf }

// Ensure DrawCell satisfies the basic Ch-set assumption.
var _ = types.DrawCell{}
