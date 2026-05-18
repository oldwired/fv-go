package editor

import (
	"bytes"
	"strings"

	"github.com/rivo/uniseg"
)

// Word-wrap + reformat support. The editor stores raw `[]byte` and
// wrap rendering operates on top — there's no insertion of soft-wrap
// markers into the buffer. RightMargin (= 0 means "use view width")
// is the wrap point; the wrap logic respects existing line breaks
// (so an unwrapped buffer renders identically to the no-wrap path).
//
// Tabs expand to TabWidth in the editor render but for wrap-column
// calculation we use grapheme-cluster widths via uniseg (so CJK
// "你好" advances col by 4, not 2; emoji clusters advance by 2 not by
// the rune count).

// effectiveMargin returns the column at which to wrap, given the
// view's current size and an optional explicit RightMargin. 0 means
// "fit the view width."
func (e *Editor) effectiveMargin() int {
	if e.RightMargin > 0 {
		return e.RightMargin
	}
	if e.Size.X > 0 {
		return e.Size.X
	}
	return 80
}

// wrapPoints returns the byte offsets at which a buffer line, given
// as a slice of bytes, should soft-wrap. The first segment is implied
// (starts at 0); each returned offset is the start of a continuation
// segment. Offsets are relative to the start of `line`.
//
// Algorithm: walk runes, track current column. When the column would
// exceed margin, look backward for the last whitespace and wrap
// there. If there's no whitespace in the segment, hard-wrap at the
// margin column. Trailing whitespace at a wrap point is consumed (so
// the next segment doesn't start with a space).
func wrapPoints(line []byte, margin int) []int {
	if margin <= 0 || len(line) == 0 {
		return nil
	}
	var out []int
	segStart := 0
	col := 0
	lastSpace := -1 // byte offset of the last space in the current segment
	state := -1
	for i := 0; i < len(line); {
		cluster, _, width, newState := uniseg.FirstGraphemeCluster(line[i:], state)
		if len(cluster) == 0 {
			break
		}
		state = newState
		w := width
		if w <= 0 {
			w = 1
		}
		isSpace := len(cluster) == 1 && (cluster[0] == ' ' || cluster[0] == '\t')
		if isSpace {
			lastSpace = i
		}
		col += w
		next := i + len(cluster)
		if col > margin {
			breakAt := lastSpace
			if breakAt < 0 || breakAt <= segStart {
				breakAt = i // hard-wrap; current cluster begins new segment
			} else {
				breakAt++ // skip the space itself
			}
			out = append(out, breakAt)
			segStart = breakAt
			col = 0
			lastSpace = -1
			state = -1 // reset cluster state after a wrap break
			// Re-walk: the next cluster starts at breakAt; if that
			// equals i, we're hard-wrapping and need to count
			// the current cluster's width in the new segment.
			if breakAt == i {
				col = w
				i = next
				continue
			}
			i = breakAt
			continue
		}
		i = next
	}
	return out
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
	margin := e.effectiveMargin()
	if margin <= 0 {
		return
	}
	// Find paragraph bounds.
	startLine := e.lineNumber(e.Cursor)
	endLine := startLine
	notBlank := func(line int) bool {
		ls, le := e.lineByIndex(line)
		return !lineIsBlank(e.Data, ls, le)
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
	paragraph := e.Data[startByte:endByte]
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
	e.applyChange(startByte, endByte, out.Bytes(), cursor)
	e.SelAnchor = -1
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
// the buffer in a single undo entry. Convenient cleanup for source
// files; nothing in the framework auto-invokes it, but the editor
// menu can expose it.
func (e *Editor) TrimTrailingWS() {
	if e.ReadOnly {
		return
	}
	src := e.Data
	out := make([]byte, 0, len(src))
	lineStart := 0
	for i := 0; i <= len(src); i++ {
		if i == len(src) || src[i] == '\n' {
			// Trim back from i.
			end := i
			for end > lineStart && (src[end-1] == ' ' || src[end-1] == '\t' || src[end-1] == '\r') {
				end--
			}
			out = append(out, src[lineStart:end]...)
			if i < len(src) {
				out = append(out, '\n')
			}
			lineStart = i + 1
		}
	}
	if bytes.Equal(out, src) {
		return
	}
	cursor := e.Cursor
	if cursor > len(out) {
		cursor = len(out)
	}
	e.applyChange(0, len(src), out, cursor)
	e.adjustScroll()
}
