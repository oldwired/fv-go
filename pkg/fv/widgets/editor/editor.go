// Package editor provides Editor — a multi-line UTF-8 text editor view
// with insert/delete, cursor navigation, selection, clipboard
// integration, and encoding-aware load/save.
//
// Ported from Editors.pas (TEditor + TFileEditor). The Pascal version
// uses a gap-buffer for O(1) inserts at the caret; this Go port uses
// a simpler `[]byte` model with O(n) inserts at the caret. For files
// up to a few MB the simpler model is fast enough and easier to reason
// about; if a target file size demands it, swapping for a gap buffer
// or rope is a localized change.
//
// What's in: a working editor with file I/O, find, replace, clipboard,
// selection, scrolling, and the standard navigation bindings.
// What's deferred: undo/redo, word wrap, syntax highlighting,
// bookmarks. EditorGutter (line-numbers column) sits separately and
// would consume an Editor reference.
package editor

import (
	"bytes"
	"strconv"
	"strings"
	stdutf8 "unicode/utf8"

	"github.com/oldwired/fv-go/pkg/fv/clipboard"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/rivo/uniseg"
)

// LineColorer is the callback shape Editor uses for syntax coloring.
// Given a line of text, return Spans that color sub-ranges of it.
// nil result (or no spans) leaves the line in DefaultAttr.
type LineColorer interface {
	Tokenize(line string) []ColorSpan
}

// ColorSpan describes a colored region in a line; matches widgets/syntax
// without forcing this package to import that one (avoids a layering
// loop). The syntax package's Span happens to be the same shape.
type ColorSpan struct {
	Start, End int
	Attr       uint16
}

// Editor is the text view.
type Editor struct {
	views.Base

	Data      []byte // raw UTF-8 bytes
	Cursor    int    // byte index of caret in Data (always at a rune boundary)
	SelAnchor int    // -1 = no selection; else other end of selection
	Top       int    // line index of the first visible line
	LeftCol   int    // first visible column (in display cells)
	TabWidth  int    // expand tabs to this many spaces visually
	// ReadOnly gates USER-INPUT mutators only — Insert (called from
	// the typed-keystroke path in HandleEvent), Backspace,
	// DeleteForward, Reformat, ReplaceAll. Host-driven content APIs
	// (SetText, Append) intentionally bypass it: a transcript pane
	// can be ReadOnly=true and still receive programmatic Append
	// calls so the user sees streamed output without being able to
	// type into it.
	ReadOnly bool
	Modified bool
	encoding utf8.FileEncoding // remembered for save round-trip
	Filename string

	HScroll *views.ScrollBar
	VScroll *views.ScrollBar

	// Colorer, when non-nil, is consulted per visible line to obtain
	// syntax-coloring spans. Hand it a *syntax.Highlighter (which
	// implements LineColorer) — or any other colorer.
	Colorer LineColorer

	// FindState is preserved between Find / Replace / Search-Again
	// invocations.
	LastFind    string
	LastReplace string
	CaseSense   bool

	// Undo / redo stack. See undo.go for the change-coalescing rules.
	undoStack []editChange
	undoAt    int

	// Wrap configuration. Wrap = false renders identical to the
	// non-wrapping path; Wrap = true wraps long lines at RightMargin
	// (or, when RightMargin is 0, at the view's width). The wrap is
	// purely visual — the underlying buffer is unchanged. Reformat
	// (Ctrl+B) actually rewrites the paragraph at RightMargin.
	Wrap        bool
	RightMargin int

	// Bookmarks: indices 0..9 map to byte positions in Data. -1 means
	// the slot is empty. Set / jump via Ctrl+K + digit / Ctrl+Q + digit.
	bookmarks [10]int

	// Two-keystroke prefix state. When non-zero, the next key is the
	// digit operand for a Ctrl+K / Ctrl+Q chord.
	prefix prefixKey

	// ShowPosition, when true, paints a "line:col" indicator in the
	// bottom-right corner of the editor bounds (1-indexed, classical
	// Turbo-Pascal style). The overlay covers whatever editor content
	// would otherwise occupy those ~10 cells. Use Position() +
	// OnCursorMove if you'd rather route the indicator into your
	// app's status line.
	ShowPosition bool

	// OnCursorMove, if non-nil, fires whenever the caret moves to a
	// new (line, col). Both are 1-indexed. Useful for syncing a
	// status-line position indicator with the editor.
	OnCursorMove func(line, col int)

	// OnChange, if non-nil, fires after every buffer mutation routed
	// through applyChange (Insert, Backspace, DeleteForward, Paste,
	// Undo/Redo of an edit, ReplaceAll, Reformat, …). The version
	// argument is a monotonic per-Editor counter that increments by 1
	// on every fire — hosts can use it to drive LSP textDocument/
	// didChange notifications, debounce syntax recoloring, or
	// invalidate caches. Debouncing is the host's responsibility;
	// firing is the editor's.
	OnChange func(version int)

	// lastReportedLine / lastReportedCol track what we last sent to
	// OnCursorMove so we don't fire it on every Draw if the caret
	// hasn't actually moved.
	lastReportedLine int
	lastReportedCol  int

	// changeVersion is the monotonic counter emitted to OnChange. It
	// is not reset by SetText so consumers can tell "a brand-new
	// buffer was loaded" apart from "no edits since the editor was
	// constructed".
	changeVersion int
}

// prefixKey tracks the first key of a two-key chord. Zero = none.
type prefixKey int

const (
	prefixNone     prefixKey = 0
	prefixSetMark  prefixKey = 1 // Ctrl+K — next digit sets a mark
	prefixJumpMark prefixKey = 2 // Ctrl+Q — next digit jumps to a mark
)

// New constructs an empty Editor.
func New(bounds geom.Rect, h, v *views.ScrollBar) *Editor {
	e := &Editor{
		Base:      views.NewBase(bounds),
		SelAnchor: -1,
		TabWidth:  4,
		HScroll:   h,
		VScroll:   v,
		encoding:  utf8.EncUTF8,
	}
	for i := range e.bookmarks {
		e.bookmarks[i] = -1
	}
	e.SetSelf(e)
	e.Options |= consts.OfSelectable | consts.OfFirstClick
	e.State |= consts.SfCursorVis
	// Anchor top-left, stretch to fill. GfGrowAll would move the
	// origin too, sliding the body toward the bottom-right when the
	// host window grows.
	e.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	return e
}

// GetTypeID for serial registry.
func (e *Editor) GetTypeID() string { return "editor" }

// SetText replaces the buffer. Wipes the undo history because the
// pre-swap state isn't reachable anymore. Fires OnChange so consumers
// (LSP, syntax recolor, dirty-tab tracking) see the swap as one
// versioned event.
func (e *Editor) SetText(s string) {
	e.Data = []byte(s)
	e.Cursor = 0
	e.SelAnchor = -1
	e.Top = 0
	e.LeftCol = 0
	e.Modified = false
	e.ResetUndo()
	e.refreshScroll()
	e.notifyChange()
}

// Text returns the buffer as a string.
func (e *Editor) Text() string { return string(e.Data) }

// ViewState captures the on-screen state of an Editor so a caller can
// round-trip it across a SetText that would otherwise lose it. Use
// when swapping between buffers that share content (env A → env B →
// env A): the user expects to be back where they left off, not at
// line 1.
type ViewState struct {
	Cursor    int
	SelAnchor int
	Top       int
	LeftCol   int
}

// ViewState snapshots Cursor / SelAnchor / Top / LeftCol so a caller
// can restore it after a SetText.
func (e *Editor) ViewState() ViewState {
	return ViewState{
		Cursor:    e.Cursor,
		SelAnchor: e.SelAnchor,
		Top:       e.Top,
		LeftCol:   e.LeftCol,
	}
}

// RestoreViewState reapplies a snapshot taken via ViewState, clamping
// each field to the current buffer so a state saved against an older,
// larger buffer doesn't dangle past the end of a shorter one. Does
// not fire OnChange — the buffer didn't change.
//
// Typical use:
//
//	saved := ed.ViewState()
//	ed.SetText(otherBuffer)
//	// later, swap back:
//	ed.SetText(originalBuffer)
//	ed.RestoreViewState(saved)
func (e *Editor) RestoreViewState(v ViewState) {
	n := len(e.Data)
	clamp := func(p int) int {
		if p < 0 {
			return 0
		}
		if p > n {
			return n
		}
		return p
	}
	e.Cursor = clamp(v.Cursor)
	if v.SelAnchor < 0 {
		e.SelAnchor = -1
	} else {
		e.SelAnchor = clamp(v.SelAnchor)
	}
	e.Top = v.Top
	if e.Top < 0 {
		e.Top = 0
	}
	maxTop := e.LineCount() - 1
	if maxTop < 0 {
		maxTop = 0
	}
	if e.Top > maxTop {
		e.Top = maxTop
	}
	e.LeftCol = v.LeftCol
	if e.LeftCol < 0 {
		e.LeftCol = 0
	}
	e.refreshScroll()
}

// Append concatenates s onto the end of the buffer. Goes through the
// same applyChange path as Insert/Backspace so Undo covers it; hosts
// that drive a transcript pane (log streams, REPL output, network
// frames) and don't want unbounded undo growth should call ResetUndo
// periodically.
//
// Unlike Insert, Append does NOT move the caret unless the caret was
// already sitting at the very end of the buffer — in which case it
// follows the new tail, giving "stick to the bottom" behavior for
// transcript readers without overriding intentional cursor placement.
//
// Append is host-driven and intentionally bypasses ReadOnly — the
// same semantics as SetText. ReadOnly gates user-input mutators
// (typed keystrokes routed to Insert / Backspace / DeleteForward),
// not programmatic content updates. A transcript pane can therefore
// keep ReadOnly=true (so the user can't edit the streamed output)
// while the host continues to Append fresh frames.
func (e *Editor) Append(s string) {
	if s == "" {
		return
	}
	atTail := e.Cursor == len(e.Data)
	end := len(e.Data)
	cursorAfter := e.Cursor
	if atTail {
		cursorAfter = end + len(s)
	}
	e.applyChange(end, end, []byte(s), cursorAfter)
	if atTail {
		e.adjustScroll()
	}
}

// LoadFile reads path, detects its encoding (UTF-8 / UTF-16 / BOM /
// ANSI), and converts to UTF-8 in memory. Filename and encoding are
// recorded; the encoding is reported by Encoding() and used by
// SaveFile to preserve the UTF-8 BOM only — UTF-16 and ANSI files
// are saved as plain UTF-8 (see SaveFile's docstring).
func (e *Editor) LoadFile(path string) error {
	data, err := readFile(path)
	if err != nil {
		return err
	}
	enc := utf8.DetectEncoding(data)
	body := utf8.ConvertToUTF8(data, enc)
	e.encoding = enc
	e.Filename = path
	e.SetText(string(body))
	return nil
}

// Encoding returns the encoding LoadFile detected (or
// utf8.EncUTF8 for buffers populated via SetText / Append). Hosts
// that want to round-trip the original encoding on save must do the
// re-encoding themselves and then write the bytes directly; the
// editor's SaveFile only preserves the UTF-8 BOM.
func (e *Editor) Encoding() utf8.FileEncoding { return e.encoding }

// SetEncoding overrides the encoding hint. Currently only affects
// whether SaveFile prepends a UTF-8 BOM (when set to EncUTF8BOM).
// Other values are stored but SaveFile does not re-encode the
// buffer — see SaveFile.
func (e *Editor) SetEncoding(enc utf8.FileEncoding) { e.encoding = enc }

// SaveFile writes the buffer to path as UTF-8 bytes. If the recorded
// encoding is EncUTF8BOM the file is prefixed with the UTF-8 BOM;
// otherwise no transformation is applied — UTF-16 LE/BE and ANSI
// (CP1252) files loaded via LoadFile are silently downgraded to
// UTF-8 on save. Hosts that need byte-for-byte round-trip of those
// encodings must re-encode before writing.
func (e *Editor) SaveFile(path string) error {
	out := append([]byte(nil), e.Data...)
	if e.encoding == utf8.EncUTF8BOM {
		out = append([]byte{0xEF, 0xBB, 0xBF}, out...)
	}
	if err := writeFile(path, out); err != nil {
		return err
	}
	e.Filename = path
	e.Modified = false
	return nil
}

// HasSelection reports whether a non-empty range is selected.
func (e *Editor) HasSelection() bool {
	return e.SelAnchor >= 0 && e.SelAnchor != e.Cursor
}

// selRange returns [lo, hi) byte indices for the selection.
func (e *Editor) selRange() (int, int) {
	a, b := e.SelAnchor, e.Cursor
	if a > b {
		a, b = b, a
	}
	return a, b
}

// SelectAll marks the entire buffer as selected.
func (e *Editor) SelectAll() {
	e.SelAnchor = 0
	e.Cursor = len(e.Data)
}

// LineCount returns 1 + (count of '\n' bytes).
func (e *Editor) LineCount() int {
	if len(e.Data) == 0 {
		return 1
	}
	return 1 + bytes.Count(e.Data, []byte{'\n'})
}

// lineStart returns the byte index of the start of the line containing
// pos (i.e., the byte after the previous '\n', or 0).
func (e *Editor) lineStart(pos int) int {
	if pos > len(e.Data) {
		pos = len(e.Data)
	}
	for i := pos - 1; i >= 0; i-- {
		if e.Data[i] == '\n' {
			return i + 1
		}
	}
	return 0
}

// lineEnd returns the byte index of the next '\n' or len(Data).
func (e *Editor) lineEnd(pos int) int {
	for i := pos; i < len(e.Data); i++ {
		if e.Data[i] == '\n' {
			return i
		}
	}
	return len(e.Data)
}

// Position returns the caret's 1-indexed line and column for status-line
// integration. Matches the indicator style of classical TUI editors
// (Turbo Pascal et al.). Column is the display column — tabs expand,
// wide runes count for 2.
func (e *Editor) Position() (line, col int) {
	return e.lineNumber(e.Cursor) + 1, e.columnAt(e.Cursor) + 1
}

// lineNumber returns the 0-based line containing pos.
func (e *Editor) lineNumber(pos int) int {
	if pos > len(e.Data) {
		pos = len(e.Data)
	}
	return bytes.Count(e.Data[:pos], []byte{'\n'})
}

// lineByIndex returns the byte range [start, end) of the line at idx.
func (e *Editor) lineByIndex(idx int) (int, int) {
	start := 0
	for cur := 0; cur < idx; cur++ {
		nl := bytes.IndexByte(e.Data[start:], '\n')
		if nl < 0 {
			return len(e.Data), len(e.Data)
		}
		start += nl + 1
	}
	end := bytes.IndexByte(e.Data[start:], '\n')
	if end < 0 {
		return start, len(e.Data)
	}
	return start, start + end
}

// columnAt returns the display column of pos (within its line).
// Iterates grapheme clusters (so a wide ZWJ emoji advances col by 2,
// a regional-indicator flag also by 2, etc.). Tab expansion is
// preserved.
func (e *Editor) columnAt(pos int) int {
	ls := e.lineStart(pos)
	col := 0
	i := ls
	state := -1
	for i < pos {
		cluster, _, width, newState := uniseg.FirstGraphemeCluster(e.Data[i:pos], state)
		if len(cluster) == 0 {
			break
		}
		if len(cluster) == 1 && cluster[0] == '\t' {
			col += e.TabWidth - col%e.TabWidth
		} else {
			col += width
		}
		i += len(cluster)
		state = newState
	}
	return col
}

// posAtCol returns the byte index in [lineStart, lineEnd) closest to
// the given display column. Iterates grapheme clusters; a cluster is
// kept atomic — landing exactly at the end of a wide cluster returns
// the byte index after it, not in the middle.
func (e *Editor) posAtCol(lineStart, lineEnd, col int) int {
	cur := 0
	i := lineStart
	state := -1
	for i < lineEnd {
		cluster, _, width, newState := uniseg.FirstGraphemeCluster(e.Data[i:lineEnd], state)
		if len(cluster) == 0 {
			break
		}
		w := width
		if len(cluster) == 1 && cluster[0] == '\t' {
			w = e.TabWidth - cur%e.TabWidth
		}
		if cur+w > col {
			return i
		}
		cur += w
		i += len(cluster)
		state = newState
	}
	return lineEnd
}

// MoveCursor sets the cursor and clears selection (unless preserve is true).
func (e *Editor) MoveCursor(pos int, preserveSel bool) {
	if pos < 0 {
		pos = 0
	}
	if pos > len(e.Data) {
		pos = len(e.Data)
	}
	if !preserveSel {
		e.SelAnchor = -1
	} else if e.SelAnchor < 0 {
		e.SelAnchor = e.Cursor
	}
	e.Cursor = pos
	e.adjustScroll()
}

// Scroll shifts the viewport by deltaLines without moving the caret.
// Positive scrolls down (later lines into view), negative scrolls up.
// Clamps so Top stays in [0, max(0, LineCount-1)]. Hosts that want a
// host-driven wheel handler (e.g., to translate Shift+wheel into
// horizontal scroll, page-wheel, etc.) can call this directly instead
// of synthesizing arrow keys, which would also move the caret.
func (e *Editor) Scroll(deltaLines int) {
	if deltaLines == 0 {
		return
	}
	e.Top += deltaLines
	maxTop := e.LineCount() - 1
	if maxTop < 0 {
		maxTop = 0
	}
	if e.Top > maxTop {
		e.Top = maxTop
	}
	if e.Top < 0 {
		e.Top = 0
	}
	e.refreshScroll()
}

// wheelStepFor decodes a mouse-wheel event into a signed line delta.
// Three lines per wheel tick is the conventional terminal step.
func wheelStepFor(ev *drivers.Event) int {
	const step = 3
	if ev.Buttons&consts.MbScrollWheelUp != 0 {
		return -step
	}
	if ev.Buttons&consts.MbScrollWheelDown != 0 {
		return step
	}
	return 0
}

// adjustScroll ensures Cursor is visible.
func (e *Editor) adjustScroll() {
	line := e.lineNumber(e.Cursor)
	visibleRows := e.Size.Y
	if line < e.Top {
		e.Top = line
	} else if line >= e.Top+visibleRows {
		e.Top = line - visibleRows + 1
	}
	col := e.columnAt(e.Cursor)
	if col < e.LeftCol {
		e.LeftCol = col
	} else if col >= e.LeftCol+e.Size.X {
		e.LeftCol = col - e.Size.X + 1
	}
	e.refreshScroll()
}

// refreshScroll updates linked scroll bars.
func (e *Editor) refreshScroll() {
	if e.VScroll != nil {
		e.VScroll.SetRange(0, e.LineCount())
		e.VScroll.SetValue(e.Top)
	}
}

// Draw paints visible lines with selection highlighting and (if
// Colorer is set) per-line syntax coloring.
func (e *Editor) Draw() {
	pal := theme.Get()
	normal := pal.EditorText
	selColor := pal.InputArrow

	selLo, selHi := -1, -1
	if e.HasSelection() {
		selLo, selHi = e.selRange()
	}

	for r := 0; r < e.Size.Y; r++ {
		buf := screen.MakeDrawBuffer(e.Size.X)
		for x := 0; x < e.Size.X; x++ {
			screen.DrawCell(buf, x, " ", normal)
		}
		lineIdx := e.Top + r
		if lineIdx >= e.LineCount() {
			e.WriteLine(0, r, e.Size.X, 1, buf)
			continue
		}
		lstart, lend := e.lineByIndex(lineIdx)
		var spans []ColorSpan
		if e.Colorer != nil {
			spans = e.Colorer.Tokenize(string(e.Data[lstart:lend]))
		}
		col := 0
		i := lstart
		state := -1
		for i < lend && col-e.LeftCol < e.Size.X {
			cluster, _, width, newState := uniseg.FirstGraphemeCluster(e.Data[i:lend], state)
			if len(cluster) == 0 {
				break
			}
			state = newState
			isTab := len(cluster) == 1 && cluster[0] == '\t'
			w := width
			var ch string
			if isTab {
				w = e.TabWidth - col%e.TabWidth
				ch = strings.Repeat(" ", w)
			} else {
				ch = string(cluster)
				if w == 0 {
					// Defensive: shouldn't happen for well-formed
					// strings (uniseg gives w=0 only for an empty
					// cluster, which we already filtered). Keep one
					// cell so we don't loop forever.
					w = 1
				}
			}
			x := col - e.LeftCol
			if x >= 0 && x < e.Size.X {
				attr := normal
				rel := i - lstart
				for _, s := range spans {
					if rel >= s.Start && rel < s.End {
						attr = s.Attr
						break
					}
				}
				if i >= selLo && i < selHi {
					attr = selColor
				}
				if isTab {
					for j := 0; j < w && x+j < e.Size.X; j++ {
						buf[x+j] = types.DrawCell{Ch: " ", Attr: attr}
					}
				} else {
					buf[x] = types.DrawCell{Ch: ch, Attr: attr}
					// Wide cluster: emit a continuation cell so the
					// cellbuf advances 2 cells in lockstep with the
					// terminal. Without this the right half of a wide
					// glyph would diff-equal the previous frame and
					// stale content would linger after a scroll.
					if w == 2 && x+1 < e.Size.X {
						buf[x+1] = types.DrawCell{Ch: "", Attr: attr}
					}
				}
			}
			col += w
			i += len(cluster)
		}
		e.WriteLine(0, r, e.Size.X, 1, buf)
	}
	e.drawPositionOverlay()
	e.notifyCursorMove()
	e.placeCursor()
}

// drawPositionOverlay paints a "line:col" indicator in the bottom-right
// of the editor bounds when ShowPosition is true. Style matches the
// classical Turbo-Pascal IDE: a small block of inverse-video cells
// glued to the corner. We paint after the line loop so the indicator
// always wins over whatever text would have been there.
func (e *Editor) drawPositionOverlay() {
	if !e.ShowPosition || e.Size.Y <= 0 || e.Size.X <= 0 {
		return
	}
	line, col := e.Position()
	label := strconv.Itoa(line) + ":" + strconv.Itoa(col)
	// Width budget: label + 1 space of padding on each side. If the
	// editor is narrower than the label, paint as much as fits.
	pad := 1
	w := len(label) + pad*2
	if w > e.Size.X {
		w = e.Size.X
	}
	startX := e.Size.X - w
	row := e.Size.Y - 1
	buf := screen.MakeDrawBuffer(w)
	pal := theme.Get()
	attr := pal.EditorSelected // inverse-ish; visually distinct from body
	for x := 0; x < w; x++ {
		screen.DrawCell(buf, x, " ", attr)
	}
	screen.DrawStr(buf, pad, label, attr)
	e.WriteLine(startX, row, w, 1, buf)
}

// notifyCursorMove fires OnCursorMove when the caret's 1-indexed
// position has changed since the last Draw. The position-overlay
// repaint already happens unconditionally; this hook is for hosts
// that want to forward to a StatusLine entry, etc.
func (e *Editor) notifyCursorMove() {
	if e.OnCursorMove == nil {
		return
	}
	line, col := e.Position()
	if line == e.lastReportedLine && col == e.lastReportedCol {
		return
	}
	e.lastReportedLine = line
	e.lastReportedCol = col
	e.OnCursorMove(line, col)
}

func (e *Editor) placeCursor() {
	line := e.lineNumber(e.Cursor)
	col := e.columnAt(e.Cursor)
	cy := line - e.Top
	cx := col - e.LeftCol
	if cy < 0 || cy >= e.Size.Y || cx < 0 || cx >= e.Size.X {
		// Caret is scrolled off — negatives mark "no visible cursor",
		// which Program.placeCursor honors by hiding the terminal cursor.
		e.Base.Cursor = geom.Point{X: -1, Y: -1}
		return
	}
	e.Base.Cursor = geom.Point{X: cx, Y: cy}
}

// Insert puts s at the caret, replacing any active selection.
func (e *Editor) Insert(s string) {
	if e.ReadOnly || s == "" {
		return
	}
	if e.HasSelection() {
		e.deleteSelection()
	}
	pos := e.Cursor
	e.applyChange(pos, pos, []byte(s), pos+len(s))
	e.SelAnchor = -1
	e.adjustScroll()
}

// deleteSelection removes the selected range; returns true if anything
// was removed.
func (e *Editor) deleteSelection() bool {
	if !e.HasSelection() {
		return false
	}
	lo, hi := e.selRange()
	e.applyChange(lo, hi, nil, lo)
	e.SelAnchor = -1
	return true
}

// Backspace deletes the char or selection before the caret.
func (e *Editor) Backspace() {
	if e.ReadOnly {
		return
	}
	if e.deleteSelection() {
		e.adjustScroll()
		return
	}
	if e.Cursor == 0 {
		return
	}
	// Step back one rune.
	_, sz := stdutf8.DecodeLastRune(e.Data[:e.Cursor])
	e.applyChange(e.Cursor-sz, e.Cursor, nil, e.Cursor-sz)
	e.adjustScroll()
}

// DeleteForward removes the char or selection after the caret.
func (e *Editor) DeleteForward() {
	if e.ReadOnly {
		return
	}
	if e.deleteSelection() {
		e.adjustScroll()
		return
	}
	if e.Cursor >= len(e.Data) {
		return
	}
	_, sz := stdutf8.DecodeRune(e.Data[e.Cursor:])
	e.applyChange(e.Cursor, e.Cursor+sz, nil, e.Cursor)
	e.adjustScroll()
}

// Copy puts the selection (or the current line if no selection) onto
// the clipboard.
func (e *Editor) Copy() {
	if e.HasSelection() {
		lo, hi := e.selRange()
		clipboard.SetText(string(e.Data[lo:hi]))
		return
	}
	ls, le := e.lineByIndex(e.lineNumber(e.Cursor))
	clipboard.SetText(string(e.Data[ls:le]))
}

// Cut copies + deletes the selection.
func (e *Editor) Cut() {
	if !e.HasSelection() {
		return
	}
	e.Copy()
	e.deleteSelection()
	e.adjustScroll()
}

// Paste inserts the clipboard at the caret.
func (e *Editor) Paste(text string) {
	if text == "" {
		text = clipboard.GetText()
	}
	e.Insert(text)
}

// HandleEvent: navigation, editing, clipboard, paste.
func (e *Editor) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvCommand {
		switch ev.Command {
		case consts.CmCopy:
			e.Copy()
			e.ClearEvent(ev)
			return
		case consts.CmCut:
			e.Cut()
			e.Draw()
			e.ClearEvent(ev)
			return
		case consts.CmPaste:
			text, _ := ev.InfoPtr.(string)
			e.Paste(text)
			e.Draw()
			e.ClearEvent(ev)
			return
		}
	}
	if ev.What == consts.EvMouseDown {
		local := e.MakeLocal(ev.Where)
		line := e.Top + local.Y
		col := e.LeftCol + local.X
		if line >= e.LineCount() {
			e.MoveCursor(len(e.Data), false)
		} else {
			ls, le := e.lineByIndex(line)
			pos := e.posAtCol(ls, le, col)
			e.MoveCursor(pos, false)
		}
		if e.Owner != nil {
			e.Owner.Focus(e.Self())
		}
		e.Draw()
		e.ClearEvent(ev)
		return
	}
	if ev.What == consts.EvMouseWheel {
		// Scroll the viewport without moving the caret. Synthesizing
		// arrow keys here would shift Cursor as a side-effect — wrong
		// behavior for a wheel scroll (the user expects the text to
		// move under the caret, not the caret to chase the wheel).
		e.Scroll(wheelStepFor(ev))
		e.ClearEvent(ev)
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	// Two-keystroke chord handling: Ctrl+K then digit sets a mark;
	// Ctrl+Q then digit jumps to a mark. The prefix is cleared after
	// the second key, success or fail.
	if e.prefix != prefixNone {
		if r := ev.UnicodeChar; r >= '0' && r <= '9' {
			idx := int(r - '0')
			if e.prefix == prefixSetMark {
				e.bookmarks[idx] = e.Cursor
			} else {
				if pos := e.bookmarks[idx]; pos >= 0 && pos <= len(e.Data) {
					e.MoveCursor(pos, false)
				}
			}
		}
		e.prefix = prefixNone
		e.Draw()
		e.ClearEvent(ev)
		return
	}
	shift := ev.KeyShift&(consts.KbLeftShift|consts.KbRightShift) != 0
	switch ev.KeyCode {
	case consts.KbCtrlZ:
		e.Undo()
		e.Draw()
		e.ClearEvent(ev)
		return
	case consts.KbCtrlY:
		e.Redo()
		e.Draw()
		e.ClearEvent(ev)
		return
	case consts.KbCtrlK:
		e.prefix = prefixSetMark
		e.ClearEvent(ev)
		return
	case consts.KbCtrlQ:
		e.prefix = prefixJumpMark
		e.ClearEvent(ev)
		return
	case consts.KbCtrlG:
		// Caller wires CmEditorGoto; fall through to default if no
		// listener is set. We just emit a command for the host to
		// handle (the demo wires it to a Jump dialog).
		notify := drivers.Event{What: consts.EvCommand, Command: consts.CmEditorGoto, InfoPtr: e.Self()}
		e.PutEvent(&notify)
		e.ClearEvent(ev)
		return
	case consts.KbCtrlB:
		e.Reformat()
		e.Draw()
		e.ClearEvent(ev)
		return
	}
	switch ev.KeyCode {
	case consts.KbLeft:
		if e.Cursor > 0 {
			_, sz := stdutf8.DecodeLastRune(e.Data[:e.Cursor])
			e.MoveCursor(e.Cursor-sz, shift)
		}
	case consts.KbRight:
		if e.Cursor < len(e.Data) {
			_, sz := stdutf8.DecodeRune(e.Data[e.Cursor:])
			e.MoveCursor(e.Cursor+sz, shift)
		}
	case consts.KbUp:
		line := e.lineNumber(e.Cursor)
		if line > 0 {
			col := e.columnAt(e.Cursor)
			ls, le := e.lineByIndex(line - 1)
			e.MoveCursor(e.posAtCol(ls, le, col), shift)
		}
	case consts.KbDown:
		line := e.lineNumber(e.Cursor)
		if line+1 < e.LineCount() {
			col := e.columnAt(e.Cursor)
			ls, le := e.lineByIndex(line + 1)
			e.MoveCursor(e.posAtCol(ls, le, col), shift)
		}
	case consts.KbHome:
		e.MoveCursor(e.lineStart(e.Cursor), shift)
	case consts.KbEnd:
		e.MoveCursor(e.lineEnd(e.Cursor), shift)
	case consts.KbPgUp:
		e.MoveCursor(e.posAtVisible(e.lineNumber(e.Cursor)-e.Size.Y, e.columnAt(e.Cursor)), shift)
		e.Top -= e.Size.Y
		if e.Top < 0 {
			e.Top = 0
		}
	case consts.KbPgDn:
		e.MoveCursor(e.posAtVisible(e.lineNumber(e.Cursor)+e.Size.Y, e.columnAt(e.Cursor)), shift)
	case consts.KbCtrlHome:
		e.MoveCursor(0, shift)
	case consts.KbCtrlEnd:
		e.MoveCursor(len(e.Data), shift)
	case consts.KbCtrlA:
		e.SelectAll()
	case consts.KbBack:
		e.Backspace()
	case consts.KbDel:
		e.DeleteForward()
	case consts.KbEnter:
		e.Insert("\n")
	case consts.KbTab:
		e.Insert("\t")
	default:
		if ev.UnicodeChar >= ' ' || ev.UnicodeChar == '\t' {
			e.Insert(string(ev.UnicodeChar))
		} else {
			return
		}
	}
	e.Draw()
	e.ClearEvent(ev)
}

func (e *Editor) posAtVisible(line, col int) int {
	if line < 0 {
		line = 0
	}
	if line >= e.LineCount() {
		line = e.LineCount() - 1
	}
	ls, le := e.lineByIndex(line)
	return e.posAtCol(ls, le, col)
}

// Find searches forward for needle starting at the cursor. Returns
// the new caret position and true on match. If caseSense is false,
// matches are case-insensitive (ASCII case folding).
func (e *Editor) Find(needle string, caseSense bool) bool {
	if needle == "" {
		return false
	}
	hay := e.Data[e.Cursor:]
	var idx int
	if caseSense {
		idx = bytes.Index(hay, []byte(needle))
	} else {
		idx = bytes.Index(bytes.ToLower(hay), bytes.ToLower([]byte(needle)))
	}
	if idx < 0 {
		return false
	}
	pos := e.Cursor + idx
	e.SelAnchor = pos
	e.Cursor = pos + len(needle)
	e.adjustScroll()
	return true
}

// ReplaceAll replaces every needle with replacement and returns the
// number of replacements made. The whole replacement is one undo
// entry — Ctrl+Z reverses all instances at once.
func (e *Editor) ReplaceAll(needle, replacement string, caseSense bool) int {
	if needle == "" {
		return 0
	}
	hay := e.Data
	var newData []byte
	var count int
	if caseSense {
		count = bytes.Count(hay, []byte(needle))
		if count == 0 {
			return 0
		}
		newData = bytes.ReplaceAll(hay, []byte(needle), []byte(replacement))
	} else {
		// Case-insensitive: rebuild byte-by-byte to avoid corrupting
		// non-ASCII bytes.
		var out bytes.Buffer
		nLow := strings.ToLower(needle)
		hayStr := string(hay)
		for {
			i := strings.Index(strings.ToLower(hayStr), nLow)
			if i < 0 {
				out.WriteString(hayStr)
				break
			}
			out.WriteString(hayStr[:i])
			out.WriteString(replacement)
			hayStr = hayStr[i+len(needle):]
			count++
		}
		if count == 0 {
			return 0
		}
		newData = out.Bytes()
	}
	cursor := e.Cursor
	if cursor > len(newData) {
		cursor = len(newData)
	}
	e.applyChange(0, len(hay), newData, cursor)
	e.SelAnchor = -1
	e.adjustScroll()
	return count
}
