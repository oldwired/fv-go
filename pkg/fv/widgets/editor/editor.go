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
// The text model (content, undo stack, file identity) lives in Buffer;
// Editor is the view over it (caret, selection, scroll, coloring).
// Several Editors can share one Buffer — see NewShared — giving split
// panes over a single document with shared undo. EditorGutter
// (line-numbers column) sits separately and consumes an Editor
// reference.
package editor

import (
	"bytes"
	"strconv"
	"strings"

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

// Editor is the text view. The content, undo stack, and file identity
// live in Buf — a *Buffer that may be shared between several Editors
// (split panes over one document). Everything else here is per-view
// state: caret, selection, scroll, colorer.
type Editor struct {
	views.Base

	Buf       *Buffer
	Cursor    int // byte index of caret in Buf (always at a rune boundary)
	SelAnchor int // -1 = no selection; else other end of selection
	// extraCarets are the secondary carets in multi-cursor mode; the
	// primary stays in Cursor/SelAnchor. See multicursor.go.
	extraCarets []Caret
	Top         int // line index of the first visible line
	LeftCol     int // first visible column (in display cells)
	TabWidth    int // expand tabs to this many spaces visually
	// ReadOnly gates USER-INPUT mutators only — Insert (called from
	// the typed-keystroke path in HandleEvent), Backspace,
	// DeleteForward, Reformat, TrimTrailingWS, ReplaceAll,
	// InsertSnippet, and Undo/Redo (a read-only pane over a shared
	// Buffer must not revert another pane's edits). Host-driven
	// content APIs (SetText, Append, ReplaceRange, Buffer.Undo)
	// intentionally bypass it: a transcript pane can be ReadOnly=true
	// and still receive programmatic Append calls so the user sees
	// streamed output without being able to type into it.
	ReadOnly bool

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

	// RightMargin is the column Reformat (Ctrl+B) reflows paragraphs to;
	// 0 means "use the view width". Visual soft-wrap of long lines is
	// not implemented — long lines clip at the right edge.
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

	// OnChange, if non-nil, fires after every buffer mutation (Insert,
	// Backspace, DeleteForward, Paste, Undo/Redo, ReplaceAll,
	// ReplaceRange, Reformat, …) — including edits made through
	// another Editor sharing the same Buffer. A grouped multi-splice
	// operation fires once. The version argument is the Buffer's
	// monotonic change counter — hosts can use it to drive LSP
	// textDocument/didChange notifications, debounce syntax
	// recoloring, or invalidate caches. Debouncing is the host's
	// responsibility; firing is the editor's.
	OnChange func(version int)

	// lastReportedLine / lastReportedCol track what we last sent to
	// OnCursorMove so we don't fire it on every Draw if the caret
	// hasn't actually moved.
	lastReportedLine int
	lastReportedCol  int

	// Code-folding state (per-view; panes over a shared Buffer fold
	// independently). See fold.go.
	folds       []foldRegion
	hiddenCache []lineInterval
	hiddenValid bool

	// SnippetVars, if non-nil, resolves LSP snippet variables
	// ($TM_FILENAME, …) during InsertSnippet. Unresolved variables
	// fall back to their ${VAR:default} text, else empty.
	SnippetVars func(name string) (value string, ok bool)

	// snippet is the active tab-stop session, nil when idle. See
	// snippet.go.
	snippet *snippetSession

	// Transient host-supplied highlight overlays. See decorations.go.
	decorations map[string][]Decoration
	decoCache   []Decoration
	decoValid   bool
}

// prefixKey tracks the first key of a two-key chord. Zero = none.
type prefixKey int

const (
	prefixNone     prefixKey = 0
	prefixSetMark  prefixKey = 1 // Ctrl+K — next digit sets a mark
	prefixJumpMark prefixKey = 2 // Ctrl+Q — next digit jumps to a mark
)

// New constructs an empty Editor with its own Buffer.
func New(bounds geom.Rect, h, v *views.ScrollBar) *Editor {
	return NewShared(bounds, h, v, nil)
}

// NewShared constructs an Editor presenting buf — pass the Buf of an
// existing Editor to get a second pane over the same document (shared
// text and undo, independent cursor/selection/scroll). nil creates a
// fresh Buffer. Call Detach when discarding the pane so the Buffer
// stops notifying it.
func NewShared(bounds geom.Rect, h, v *views.ScrollBar, buf *Buffer) *Editor {
	if buf == nil {
		buf = NewBuffer()
	}
	e := &Editor{
		Base:      views.NewBase(bounds),
		Buf:       buf,
		SelAnchor: -1,
		TabWidth:  4,
		HScroll:   h,
		VScroll:   v,
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
	buf.addListener(e)
	return e
}

// GetTypeID for serial registry.
func (e *Editor) GetTypeID() string { return "editor" }

// Detach unregisters this Editor from its Buffer's listener list.
// Call when permanently discarding one pane of a shared-buffer split;
// harmless for sole owners.
func (e *Editor) Detach() { e.Buf.removeListener(e) }

// SetText replaces the buffer content. Wipes the undo history because
// the pre-swap state isn't reachable anymore. Fires OnChange so
// consumers (LSP, syntax recolor, dirty-tab tracking) see the swap as
// one versioned event.
func (e *Editor) SetText(s string) { e.Buf.SetText(s) }

// Text returns the buffer content as a string.
func (e *Editor) Text() string { return e.Buf.Text() }

// Len returns the buffer length in bytes.
func (e *Editor) Len() int { return e.Buf.Len() }

// IsModified reports whether the buffer has unsaved changes.
func (e *Editor) IsModified() bool { return e.Buf.Modified }

// ReplaceRange replaces the byte range [start, end) with text as one
// undo entry, keeping this editor's caret (and every other attached
// editor's) in place via remapping. See Buffer.ReplaceRange.
func (e *Editor) ReplaceRange(start, end int, text string) {
	e.Buf.ReplaceRange(start, end, text)
}

// OffsetAt returns the byte offset of a 0-based (line, byte-col)
// position, clamped. See Buffer.OffsetAt.
func (e *Editor) OffsetAt(line, col int) int { return e.Buf.OffsetAt(line, col) }

// PositionFor returns the 0-based line and byte column of offset.
// See Buffer.PositionFor.
func (e *Editor) PositionFor(offset int) (line, col int) { return e.Buf.PositionFor(offset) }

// CellOf returns the editor-local cell where the byte offset is
// rendered and whether it is currently on screen (scroll- and
// fold-aware). Hosts use it to anchor hover popups at a token.
func (e *Editor) CellOf(offset int) (geom.Point, bool) {
	offset = e.Buf.clampToRuneStart(offset)
	cy := e.RowOfLine(e.lineNumber(offset))
	cx := e.columnAt(offset) - e.LeftCol
	if cy < 0 || cy >= e.Size.Y || cx < 0 || cx >= e.Size.X {
		return geom.Point{X: -1, Y: -1}, false
	}
	return geom.Point{X: cx, Y: cy}, true
}

// bufferSpliced remaps this editor's positional state across an edit.
// The acting editor (sp.Origin == e) places its own caret, so only
// foreign splices move Cursor/SelAnchor; bookmarks and scroll anchor
// always follow the text.
func (e *Editor) bufferSpliced(sp Splice) {
	if sp.Origin != e {
		e.Cursor = e.Buf.clampToRuneStart(adjustCaret(e.Cursor, sp))
		if e.SelAnchor >= 0 {
			e.SelAnchor = e.Buf.clampToRuneStart(adjustCaret(e.SelAnchor, sp))
			if e.SelAnchor == e.Cursor {
				e.SelAnchor = -1
			}
		}
		if sp.StartLine < e.Top {
			top := e.Top + sp.LinesDelta
			if top < sp.StartLine {
				top = sp.StartLine
			}
			if top < 0 {
				top = 0
			}
			e.Top = top
		}
	}
	if sp.Origin != e {
		for i := range e.extraCarets {
			c := &e.extraCarets[i]
			c.Pos = e.Buf.clampToRuneStart(adjustCaret(c.Pos, sp))
			if c.Anchor >= 0 {
				c.Anchor = e.Buf.clampToRuneStart(adjustCaret(c.Anchor, sp))
				if c.Anchor == c.Pos {
					c.Anchor = -1
				}
			}
		}
		e.normalizeCarets()
	}
	for i, bm := range e.bookmarks {
		if bm >= 0 {
			e.bookmarks[i] = adjustCaret(bm, sp)
		}
	}
	e.adjustFoldsForSplice(sp)
	e.adjustSnippetForSplice(sp)
	e.adjustDecorationsForSplice(sp)
	e.refreshScroll()
}

// bufferReset reacts to a full content swap (SetText / LoadFile):
// every attached pane returns to the top with no selection.
func (e *Editor) bufferReset() {
	e.Cursor = 0
	e.SelAnchor = -1
	e.extraCarets = nil
	e.Top = 0
	e.LeftCol = 0
	e.folds = nil
	e.invalidateFolds()
	e.snippet = nil
	e.decorations = nil
	e.decoValid = false
	// Bookmarks track positions through splices, but a full content
	// swap leaves nothing for them to point at.
	for i := range e.bookmarks {
		e.bookmarks[i] = -1
	}
	e.refreshScroll()
}

// bufferChanged forwards the Buffer's version bump to OnChange and
// schedules a repaint so sibling panes redraw the shared content.
func (e *Editor) bufferChanged(version int) {
	if e.OnChange != nil {
		e.OnChange(version)
	}
	views.MarkDirty()
}

// bufferSaved schedules a repaint so dirty-tab indicators reading
// IsModified refresh. Not a version bump — saving isn't an edit.
func (e *Editor) bufferSaved() {
	views.MarkDirty()
}

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
	n := len(e.Buf.data)
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
	atTail := e.Cursor == len(e.Buf.data)
	end := len(e.Buf.data)
	cursorAfter := e.Cursor
	if atTail {
		cursorAfter = end + len(s)
	}
	e.applyChange(end, end, []byte(s), cursorAfter)
	if atTail {
		e.adjustScroll()
	}
}

// LoadFile reads path into the buffer, detecting its encoding.
// See Buffer.LoadFile.
func (e *Editor) LoadFile(path string) error { return e.Buf.LoadFile(path) }

// Encoding returns the encoding LoadFile detected (or utf8.EncUTF8
// for buffers populated via SetText / Append). Hosts that want to
// round-trip the original encoding on save must do the re-encoding
// themselves and then write the bytes directly; SaveFile only
// preserves the UTF-8 BOM.
func (e *Editor) Encoding() utf8.FileEncoding { return e.Buf.Encoding() }

// SetEncoding overrides the encoding hint. Currently only affects
// whether SaveFile prepends a UTF-8 BOM (when set to EncUTF8BOM).
func (e *Editor) SetEncoding(enc utf8.FileEncoding) { e.Buf.SetEncoding(enc) }

// SaveFile writes the buffer to path. See Buffer.SaveFile.
func (e *Editor) SaveFile(path string) error { return e.Buf.SaveFile(path) }

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

// SelectAll marks the entire buffer as selected (collapsing any
// secondary carets — one all-covering selection subsumes them).
func (e *Editor) SelectAll() {
	e.CollapseCarets()
	e.SelAnchor = 0
	e.Cursor = len(e.Buf.data)
}

// LineCount returns 1 + (count of '\n' bytes).
func (e *Editor) LineCount() int { return e.Buf.LineCount() }

// lineStart returns the byte index of the start of the line containing
// pos (i.e., the byte after the previous '\n', or 0).
func (e *Editor) lineStart(pos int) int {
	if pos > len(e.Buf.data) {
		pos = len(e.Buf.data)
	}
	for i := pos - 1; i >= 0; i-- {
		if e.Buf.data[i] == '\n' {
			return i + 1
		}
	}
	return 0
}

// lineEnd returns the byte index of the next '\n' or len(Data).
func (e *Editor) lineEnd(pos int) int {
	for i := pos; i < len(e.Buf.data); i++ {
		if e.Buf.data[i] == '\n' {
			return i
		}
	}
	return len(e.Buf.data)
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
	if pos > len(e.Buf.data) {
		pos = len(e.Buf.data)
	}
	return bytes.Count(e.Buf.data[:pos], []byte{'\n'})
}

// clampToRuneStart returns pos moved back to the first byte of the
// UTF-8 rune it lands in, so a stale byte offset can never position
// the caret mid-rune.
func (e *Editor) clampToRuneStart(pos int) int { return e.Buf.clampToRuneStart(pos) }

// lineByIndex returns the byte range [start, end) of the line at idx.
func (e *Editor) lineByIndex(idx int) (int, int) {
	start := 0
	for cur := 0; cur < idx; cur++ {
		nl := bytes.IndexByte(e.Buf.data[start:], '\n')
		if nl < 0 {
			return len(e.Buf.data), len(e.Buf.data)
		}
		start += nl + 1
	}
	end := bytes.IndexByte(e.Buf.data[start:], '\n')
	if end < 0 {
		return start, len(e.Buf.data)
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
		cluster, _, width, newState := uniseg.FirstGraphemeCluster(e.Buf.data[i:pos], state)
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
		cluster, _, width, newState := uniseg.FirstGraphemeCluster(e.Buf.data[i:lineEnd], state)
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

// MoveCursor sets the cursor and clears selection (unless preserve is
// true). Landing on a line hidden inside a collapsed fold (Ctrl+End,
// Find, bookmark jumps, undo restores) unfolds that region — the
// caret is never parked out of sight.
func (e *Editor) MoveCursor(pos int, preserveSel bool) {
	if pos < 0 {
		pos = 0
	}
	if pos > len(e.Buf.data) {
		pos = len(e.Buf.data)
	}
	if !preserveSel {
		e.SelAnchor = -1
	} else if e.SelAnchor < 0 {
		e.SelAnchor = e.Cursor
	}
	e.Cursor = pos
	if len(e.folds) > 0 {
		if line := e.lineNumber(pos); !e.IsLineVisible(line) {
			e.unfoldAt(line)
		}
	}
	e.checkSnippetBounds()
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
	e.Top = e.lineWalkVisible(e.nextVisibleLine(e.Top), deltaLines)
	maxTop := e.prevVisibleLine(e.LineCount() - 1)
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

// adjustScroll ensures Cursor is visible. All vertical math runs in
// visual-line space so collapsed folds count for the single row they
// occupy. Top always rests on a visible line.
func (e *Editor) adjustScroll() {
	line := e.lineNumber(e.Cursor)
	if !e.IsLineVisible(line) {
		// Defensive: the caret invariants should prevent this, but a
		// hidden caret must not wedge the scroll math.
		line = e.prevVisibleLine(line)
	}
	e.Top = e.nextVisibleLine(e.Top)
	visibleRows := e.Size.Y
	if line < e.Top {
		e.Top = line
	} else if visibleRows > 0 {
		vline := e.visIndex(line)
		vtop := e.visIndex(e.Top)
		if vline >= vtop+visibleRows {
			e.Top = e.lineAtVisIndex(vline - visibleRows + 1)
		}
	}
	col := e.columnAt(e.Cursor)
	if col < e.LeftCol {
		e.LeftCol = col
	} else if col >= e.LeftCol+e.Size.X {
		e.LeftCol = col - e.Size.X + 1
	}
	e.refreshScroll()
}

// refreshScroll updates linked scroll bars (in visible-line space).
func (e *Editor) refreshScroll() {
	if e.VScroll != nil {
		e.VScroll.SetRange(0, e.VisibleLineCount())
		e.VScroll.SetValue(e.visIndex(e.Top))
	}
}

// Draw paints visible lines with selection highlighting and (if
// Colorer is set) per-line syntax coloring.
func (e *Editor) Draw() {
	pal := theme.Get()
	normal := pal.EditorText
	selColor := pal.InputArrow
	caretAttr := pal.EditorCaretExtra
	foldAttr := pal.EditorFoldSummary

	// Selection ranges of every caret, sorted; i advances monotonically
	// across the whole Draw (rows walk lines in byte order), so a single
	// cursor into the range list suffices — no per-cell search.
	selRanges := e.selectionRanges()
	extraPos := e.extraCaretPositions()
	selIdx := 0
	inSel := func(i int) bool {
		for selIdx < len(selRanges) && i >= selRanges[selIdx].End {
			selIdx++
		}
		return selIdx < len(selRanges) && i >= selRanges[selIdx].Start
	}
	decs := e.mergedDecorations()
	decIdx := 0
	inDeco := func(i int) (uint16, bool) {
		for decIdx < len(decs) && i >= decs[decIdx].End {
			decIdx++
		}
		if decIdx < len(decs) && i >= decs[decIdx].Start {
			return decs[decIdx].Attr, true
		}
		return 0, false
	}

	// LineCount is loop-invariant; computing it (and lineByIndex) per row
	// re-scanned the whole buffer every row, making Draw O(rows×fileSize)
	// when scrolled deep into a large file. Resolve the first visible
	// line's byte range once, then advance to the next line each row —
	// skipping fold-hidden lines in the same incremental walk.
	lineCount := e.LineCount()
	line := e.nextVisibleLine(e.Top)
	if line >= lineCount {
		line = lineCount
	}
	lstart, lend := e.lineByIndex(line)
	for r := 0; r < e.Size.Y; r++ {
		buf := screen.MakeDrawBuffer(e.Size.X)
		for x := 0; x < e.Size.X; x++ {
			screen.DrawCell(buf, x, " ", normal)
		}
		if line >= lineCount {
			e.WriteLine(0, r, e.Size.X, 1, buf)
			continue
		}
		var spans []ColorSpan
		if e.Colorer != nil {
			spans = e.Colorer.Tokenize(string(e.Buf.data[lstart:lend]))
		}
		col := 0
		i := lstart
		state := -1
		for i < lend && col-e.LeftCol < e.Size.X {
			cluster, _, width, newState := uniseg.FirstGraphemeCluster(e.Buf.data[i:lend], state)
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
				if da, ok := inDeco(i); ok {
					attr = da
				}
				if inSel(i) {
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
		// Collapsed fold header: append a dim summary of what's hidden.
		if n := e.foldedLinesAt(line); n > 0 {
			suffix := " ⋯ " + strconv.Itoa(n) + " lines"
			x := col - e.LeftCol
			for _, sr := range suffix {
				if x >= 0 && x < e.Size.X {
					buf[x] = types.DrawCell{Ch: string(sr), Attr: foldAttr}
				}
				x++
			}
		}
		// Secondary carets on this line render as a styled cell (the
		// hardware cursor marks only the primary).
		for _, p := range extraPos {
			if p < lstart || p > lend {
				continue
			}
			cx := e.columnAt(p) - e.LeftCol
			if cx >= 0 && cx < e.Size.X {
				ch := buf[cx].Ch
				if ch == "" {
					ch = " "
				}
				buf[cx] = types.DrawCell{Ch: ch, Attr: caretAttr}
			}
		}
		e.WriteLine(0, r, e.Size.X, 1, buf)
		// Advance to the next visible line for the following row
		// (incremental; no rescan from byte 0 — hidden lines are walked
		// once, cumulatively, across the whole Draw). lend points at
		// the '\n' (or EOF).
		target := e.nextVisibleLine(line + 1)
		for line < target {
			if lend >= len(e.Buf.data) {
				line = lineCount
				break
			}
			lstart = lend + 1
			if nl := bytes.IndexByte(e.Buf.data[lstart:], '\n'); nl < 0 {
				lend = len(e.Buf.data)
			} else {
				lend = lstart + nl
			}
			line++
		}
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
	cy := e.RowOfLine(line)
	cx := col - e.LeftCol
	if cy < 0 || cy >= e.Size.Y || cx < 0 || cx >= e.Size.X {
		// Caret is scrolled off (or inside a collapsed fold) —
		// negatives mark "no visible cursor", which Program.placeCursor
		// honors by hiding the terminal cursor.
		e.Base.Cursor = geom.Point{X: -1, Y: -1}
		return
	}
	e.Base.Cursor = geom.Point{X: cx, Y: cy}
}

// Insert puts s at the caret, replacing any active selection. The
// replacement is one splice (not delete-then-insert), so it is one
// undo entry and sibling panes see one consistent remap. With
// multiple carets, s is inserted at every caret as one undo group.
func (e *Editor) Insert(s string) {
	if e.ReadOnly || s == "" {
		return
	}
	if len(e.extraCarets) > 0 {
		e.insertAtCarets(s)
		return
	}
	start, end := e.Cursor, e.Cursor
	if e.HasSelection() {
		start, end = e.selRange()
	}
	e.SelAnchor = -1
	e.applyChange(start, end, []byte(s), start+len(s))
	e.adjustScroll()
}

// deleteSelection removes the selected range; returns true if anything
// was removed.
func (e *Editor) deleteSelection() bool {
	if !e.HasSelection() {
		return false
	}
	lo, hi := e.selRange()
	e.SelAnchor = -1
	e.applyChange(lo, hi, nil, lo)
	return true
}

// Backspace deletes the char or selection before the caret (every
// caret, in multi-cursor mode).
func (e *Editor) Backspace() {
	if e.ReadOnly {
		return
	}
	if len(e.extraCarets) > 0 {
		e.backspaceAtCarets()
		return
	}
	if e.deleteSelection() {
		e.adjustScroll()
		return
	}
	if e.Cursor == 0 {
		return
	}
	prev := e.moverLeft(e.Cursor)
	e.applyChange(prev, e.Cursor, nil, prev)
	e.adjustScroll()
}

// DeleteForward removes the char or selection after the caret (every
// caret, in multi-cursor mode).
func (e *Editor) DeleteForward() {
	if e.ReadOnly {
		return
	}
	if len(e.extraCarets) > 0 {
		e.deleteForwardAtCarets()
		return
	}
	if e.deleteSelection() {
		e.adjustScroll()
		return
	}
	if e.Cursor >= len(e.Buf.data) {
		return
	}
	e.applyChange(e.Cursor, e.moverRight(e.Cursor), nil, e.Cursor)
	e.adjustScroll()
}

// Copy puts the selection (or the current line if no selection) onto
// the clipboard. Multiple caret selections are joined with newlines
// in document order.
func (e *Editor) Copy() {
	if text, ok := e.multiSelectionText(); ok {
		clipboard.SetText(text)
		return
	}
	if e.HasSelection() {
		lo, hi := e.selRange()
		clipboard.SetText(string(e.Buf.data[lo:hi]))
		return
	}
	ls, le := e.lineByIndex(e.lineNumber(e.Cursor))
	clipboard.SetText(string(e.Buf.data[ls:le]))
}

// Cut copies + deletes the selection (all caret selections, as one
// undo group).
func (e *Editor) Cut() {
	if e.ReadOnly {
		return
	}
	if len(e.extraCarets) > 0 {
		if _, ok := e.multiSelectionText(); !ok {
			return
		}
		e.Copy()
		e.deleteSelections()
		return
	}
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
		e.mouseDown(ev)
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
				// Bookmarks follow the text across splices (see
				// bufferSpliced); the rune-boundary snap stays as a
				// defensive clamp for hand-poked offsets.
				if pos := e.bookmarks[idx]; pos >= 0 && pos <= len(e.Buf.data) {
					e.MoveCursor(e.clampToRuneStart(pos), false)
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
	case consts.KbCtrlUp, consts.KbCtrlDown:
		// Ctrl+Alt+Up/Down adds a caret above/below; bare Ctrl+Up/Down
		// stays unbound (don't consume).
		if ev.KeyShift&consts.KbAltShift == 0 {
			return
		}
		dir := -1
		if ev.KeyCode == consts.KbCtrlDown {
			dir = 1
		}
		e.addCaretVertically(dir)
		e.Draw()
		e.ClearEvent(ev)
		return
	}
	switch ev.KeyCode {
	case consts.KbLeft:
		e.moveCarets(shift, e.moverLeft)
	case consts.KbRight:
		e.moveCarets(shift, e.moverRight)
	case consts.KbUp:
		e.moveCarets(shift, e.moverVertical(-1))
	case consts.KbDown:
		e.moveCarets(shift, e.moverVertical(1))
	case consts.KbHome:
		e.moveCarets(shift, func(pos int) int { return e.lineStart(pos) })
	case consts.KbEnd:
		e.moveCarets(shift, func(pos int) int { return e.lineEnd(pos) })
	case consts.KbPgUp:
		// MoveCursor → adjustScroll already shifts Top to keep the new
		// caret on screen. The old manual `Top -= Size.Y` here scrolled
		// a second page and could push the caret off the top; PgDn never
		// had it. Let adjustScroll own the scroll for both. Pages count
		// visible lines, so collapsed folds cost one row each.
		e.CollapseCarets()
		e.MoveCursor(e.posAtVisible(e.lineWalkVisible(e.lineNumber(e.Cursor), -e.Size.Y), e.columnAt(e.Cursor)), shift)
	case consts.KbPgDn:
		e.CollapseCarets()
		e.MoveCursor(e.posAtVisible(e.lineWalkVisible(e.lineNumber(e.Cursor), e.Size.Y), e.columnAt(e.Cursor)), shift)
	case consts.KbCtrlHome:
		e.CollapseCarets()
		e.MoveCursor(0, shift)
	case consts.KbCtrlEnd:
		e.CollapseCarets()
		e.MoveCursor(len(e.Buf.data), shift)
	case consts.KbCtrlA:
		e.SelectAll()
	case consts.KbEsc:
		// Snippet-session end beats caret collapse; consume Esc only
		// when it actually did something — dialogs rely on Esc
		// reaching them.
		if e.snippet != nil {
			e.CancelSnippet()
		} else if len(e.extraCarets) > 0 {
			e.CollapseCarets()
		} else {
			return
		}
	case consts.KbBack:
		e.Backspace()
	case consts.KbDel:
		e.DeleteForward()
	case consts.KbEnter:
		e.Insert("\n")
	case consts.KbTab:
		if e.snippet != nil {
			e.snippetNext()
		} else {
			e.Insert("\t")
		}
	case consts.KbShiftTab:
		// Without a snippet session, Shift-Tab stays with the group's
		// focus navigation.
		if e.snippet == nil {
			return
		}
		e.snippetPrev()
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

// asciiLowerBytes returns a copy of b with ASCII A–Z folded to lower
// case and every other byte left untouched. It is length-preserving —
// unlike bytes.ToLower, which case-folds multi-byte runes (e.g. K
// U+212A → k) and can change the byte length, which would misalign a
// match offset taken in the folded buffer when applied to the original.
func asciiLowerBytes(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return out
}

// Find searches forward for needle starting at the cursor. Returns
// the new caret position and true on match. If caseSense is false,
// matches are case-insensitive (ASCII case folding).
func (e *Editor) Find(needle string, caseSense bool) bool {
	if needle == "" {
		return false
	}
	e.CollapseCarets()
	hay := e.Buf.data[e.Cursor:]
	var idx int
	if caseSense {
		idx = bytes.Index(hay, []byte(needle))
	} else {
		idx = bytes.Index(asciiLowerBytes(hay), asciiLowerBytes([]byte(needle)))
	}
	if idx < 0 {
		return false
	}
	pos := e.Cursor + idx
	// Reveal a match hidden inside a collapsed fold before selecting
	// it, then route the caret through MoveCursor so the auto-unfold,
	// snippet-bounds check, and scrolling all apply — a Find must
	// never park the selection out of sight.
	if len(e.folds) > 0 {
		if line := e.lineNumber(pos); !e.IsLineVisible(line) {
			e.unfoldAt(line)
		}
	}
	e.SelAnchor = pos
	e.MoveCursor(pos+len(needle), true)
	return true
}

// ReplaceAll replaces every needle with replacement and returns the
// number of replacements made. Each match is its own splice inside
// one undo group — Ctrl+Z still reverses all instances at once, but
// folds, decorations, bookmarks, and sibling-pane carets anchored in
// untouched text survive (a single whole-buffer splice would count
// as destroying every interior anchor). Gated by ReadOnly like the
// other user-input mutators.
func (e *Editor) ReplaceAll(needle, replacement string, caseSense bool) int {
	if needle == "" || e.ReadOnly {
		return 0
	}
	e.CollapseCarets()
	e.SelAnchor = -1
	nb := []byte(needle)
	hay := e.Buf.data
	// Case-insensitive matching folds once with the length-preserving
	// ASCII fold so match offsets map exactly onto the original bytes
	// (bytes.ToLower can change length on non-ASCII runes).
	search := hay
	searchNeedle := nb
	if !caseSense {
		search = asciiLowerBytes(hay)
		searchNeedle = asciiLowerBytes(nb)
	}
	var matches []int
	for pos := 0; ; {
		rel := bytes.Index(search[pos:], searchNeedle)
		if rel < 0 {
			break
		}
		matches = append(matches, pos+rel)
		pos += rel + len(nb)
	}
	if len(matches) == 0 {
		return 0
	}
	rb := []byte(replacement)
	e.Buf.BeginGroup()
	delta := 0
	for _, m := range matches {
		at := m + delta
		e.Buf.splice(nil, at, at+len(nb), rb, -1, -1)
		delta += len(rb) - len(nb)
	}
	e.Buf.EndGroup()
	e.adjustScroll()
	return len(matches)
}
