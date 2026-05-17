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
	ReadOnly  bool
	Modified  bool
	encoding  utf8.FileEncoding // remembered for save round-trip
	Filename  string

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

	// lastReportedLine / lastReportedCol track what we last sent to
	// OnCursorMove so we don't fire it on every Draw if the caret
	// hasn't actually moved.
	lastReportedLine int
	lastReportedCol  int
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
// pre-swap state isn't reachable anymore.
func (e *Editor) SetText(s string) {
	e.Data = []byte(s)
	e.Cursor = 0
	e.SelAnchor = -1
	e.Top = 0
	e.LeftCol = 0
	e.Modified = false
	e.ResetUndo()
	e.refreshScroll()
}

// Text returns the buffer as a string.
func (e *Editor) Text() string { return string(e.Data) }

// LoadFile reads path, detects its encoding (UTF-8 / UTF-16 / BOM /
// ANSI), and converts to UTF-8 in memory. Filename and encoding are
// recorded so SaveFile can round-trip.
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

// SaveFile writes the buffer to path. UTF-8-with-BOM round-trips its
// BOM; UTF-16 is downgraded to UTF-8 (no re-encoding). To save a
// different encoding, set e.encoding before calling.
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
func (e *Editor) columnAt(pos int) int {
	ls := e.lineStart(pos)
	col := 0
	i := ls
	for i < pos {
		r, sz := stdutf8.DecodeRune(e.Data[i:])
		if r == '\t' {
			col += e.TabWidth - col%e.TabWidth
		} else {
			col += utf8.RuneCellWidth(r)
		}
		i += sz
	}
	return col
}

// posAtCol returns the byte index in [lineStart, lineEnd) closest to
// the given display column.
func (e *Editor) posAtCol(lineStart, lineEnd, col int) int {
	cur := 0
	i := lineStart
	for i < lineEnd {
		r, sz := stdutf8.DecodeRune(e.Data[i:])
		w := utf8.RuneCellWidth(r)
		if r == '\t' {
			w = e.TabWidth - cur%e.TabWidth
		}
		if cur+w > col {
			return i
		}
		cur += w
		i += sz
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
		for i < lend && col-e.LeftCol < e.Size.X {
			r2, sz := stdutf8.DecodeRune(e.Data[i:])
			w := utf8.RuneCellWidth(r2)
			ch := string(r2)
			if r2 == '\t' {
				w = e.TabWidth - col%e.TabWidth
				ch = strings.Repeat(" ", w)
			}
			if w == 0 {
				w = 1
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
				if r2 == '\t' {
					for j := 0; j < w && x+j < e.Size.X; j++ {
						buf[x+j] = types.DrawCell{Ch: " ", Attr: attr}
					}
				} else {
					buf[x] = types.DrawCell{Ch: ch, Attr: attr}
				}
			}
			col += w
			i += sz
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
