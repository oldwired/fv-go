package editor

import (
	"bytes"
	"strings"

	"github.com/rivo/uniseg"
)

// Paragraph reformat support. The editor stores raw `[]byte`; Reformat
// rewrites the paragraph at the cursor to fit effectiveMargin. There is
// no visual soft-wrap — long lines clip at the view's right edge.
//
// For wrap-column calculation we use grapheme-cluster widths via uniseg
// (so CJK "你好" advances col by 4, not 2; emoji clusters advance by 2
// not by the rune count).

// effectiveMargin returns the column Reformat wraps to, given the view's
// current size and an optional explicit RightMargin. 0 means "fit the
// view width."
func (e *Editor) effectiveMargin() int {
	if e.RightMargin > 0 {
		return e.RightMargin
	}
	if e.Size.X > 0 {
		return e.Size.X
	}
	return 80
}

// Reformat reflows the paragraph containing the cursor to fit within
// effectiveMargin. A "paragraph" is a run of non-blank lines bounded
// by blank lines (or the buffer edges). Existing single-line breaks
// are merged; words are re-broken at the margin; trailing whitespace
// is trimmed. All changes are one undo entry.
//
// Bound to Ctrl+B by HandleEvent.
func (e *Editor) Reformat() {
	if e.ReadOnly {
		return
	}
	// Secondary carets don't survive a paragraph rewrite — the splice
	// comes from this editor, so bufferSpliced won't remap them, and
	// stale offsets would corrupt (or panic on) the next edit.
	e.CollapseCarets()
	margin := e.effectiveMargin()
	if margin <= 0 {
		return
	}
	// Find paragraph bounds.
	startLine := e.lineNumber(e.Cursor)
	endLine := startLine
	notBlank := func(line int) bool {
		ls, le := e.lineByIndex(line)
		return !lineIsBlank(e.Buf.data, ls, le)
	}
	for startLine > 0 && notBlank(startLine-1) {
		startLine--
	}
	for endLine+1 < e.LineCount() && notBlank(endLine+1) {
		endLine++
	}
	startByte, _ := e.lineByIndex(startLine)
	_, endByte := e.lineByIndex(endLine)
	if startByte >= endByte {
		return
	}
	paragraph := e.Buf.data[startByte:endByte]
	// Collapse: replace internal newlines + leading/trailing whitespace
	// of each line with single spaces, then re-wrap.
	words := strings.Fields(string(paragraph))
	if len(words) == 0 {
		return
	}
	var out bytes.Buffer
	col := 0
	for i, w := range words {
		wlen := uniseg.StringWidth(w)
		if i == 0 {
			out.WriteString(w)
			col = wlen
			continue
		}
		if col+1+wlen > margin {
			out.WriteByte('\n')
			out.WriteString(w)
			col = wlen
		} else {
			out.WriteByte(' ')
			out.WriteString(w)
			col += 1 + wlen
		}
	}
	cursor := startByte + out.Len()
	e.SelAnchor = -1
	e.applyChange(startByte, endByte, out.Bytes(), cursor)
	e.adjustScroll()
}

// lineIsBlank reports whether the byte range [start, end) is empty or
// pure whitespace.
func lineIsBlank(data []byte, start, end int) bool {
	for i := start; i < end; i++ {
		c := data[i]
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			return false
		}
	}
	return true
}

// TrimTrailingWS removes trailing spaces / tabs from every line in
// the buffer as one undo group. Each line's trim is its own splice so
// folds, decorations, bookmarks, and sibling-pane carets anchored in
// untouched text survive — a single whole-buffer splice would count
// as destroying every interior anchor. Convenient cleanup for source
// files; nothing in the framework auto-invokes it, but the editor
// menu can expose it.
func (e *Editor) TrimTrailingWS() {
	if e.ReadOnly {
		return
	}
	e.CollapseCarets()
	src := e.Buf.data
	// Collect [from, to) trailing-whitespace runs first; splicing
	// back-to-front keeps every earlier offset valid without delta
	// bookkeeping.
	var runs [][2]int
	lineStart := 0
	for i := 0; i <= len(src); i++ {
		if i == len(src) || src[i] == '\n' {
			end := i
			for end > lineStart && (src[end-1] == ' ' || src[end-1] == '\t' || src[end-1] == '\r') {
				end--
			}
			if end < i {
				runs = append(runs, [2]int{end, i})
			}
			lineStart = i + 1
		}
	}
	if len(runs) == 0 {
		return
	}
	e.Buf.BeginGroup()
	for i := len(runs) - 1; i >= 0; i-- {
		e.Buf.splice(nil, runs[i][0], runs[i][1], nil, -1, -1)
	}
	e.Buf.EndGroup()
	e.adjustScroll()
}
