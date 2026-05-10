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
	"strings"
	stdutf8 "unicode/utf8"

	"github.com/oldwired/fv-go/pkg/fv/clipboard"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
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
}

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

// SetText replaces the buffer.
func (e *Editor) SetText(s string) {
	e.Data = []byte(s)
	e.Cursor = 0
	e.SelAnchor = -1
	e.Top = 0
	e.LeftCol = 0
	e.Modified = false
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
	normal := types.MakeAttr(0x07, 0x01)
	selColor := types.MakeAttr(0x0F, 0x06)

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
	e.placeCursor()
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
	e.deleteSelection()
	pos := e.Cursor
	e.Data = append(e.Data[:pos], append([]byte(s), e.Data[pos:]...)...)
	e.Cursor = pos + len(s)
	e.SelAnchor = -1
	e.Modified = true
	e.adjustScroll()
}

// deleteSelection removes the selected range; returns true if anything
// was removed.
func (e *Editor) deleteSelection() bool {
	if !e.HasSelection() {
		return false
	}
	lo, hi := e.selRange()
	e.Data = append(e.Data[:lo], e.Data[hi:]...)
	e.Cursor = lo
	e.SelAnchor = -1
	e.Modified = true
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
	r, sz := stdutf8.DecodeLastRune(e.Data[:e.Cursor])
	_ = r
	e.Data = append(e.Data[:e.Cursor-sz], e.Data[e.Cursor:]...)
	e.Cursor -= sz
	e.Modified = true
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
	e.Data = append(e.Data[:e.Cursor], e.Data[e.Cursor+sz:]...)
	e.Modified = true
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
	shift := ev.KeyShift&(consts.KbLeftShift|consts.KbRightShift) != 0
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
// number of replacements made.
func (e *Editor) ReplaceAll(needle, replacement string, caseSense bool) int {
	if needle == "" {
		return 0
	}
	var count int
	hay := e.Data
	if caseSense {
		count = bytes.Count(hay, []byte(needle))
		e.Data = bytes.ReplaceAll(hay, []byte(needle), []byte(replacement))
	} else {
		// Case-insensitive: do it the hard way to avoid corrupting
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
		e.Data = out.Bytes()
	}
	if count > 0 {
		e.Modified = true
	}
	if e.Cursor > len(e.Data) {
		e.Cursor = len(e.Data)
	}
	e.SelAnchor = -1
	e.adjustScroll()
	return count
}
