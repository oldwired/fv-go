package editor

import (
	"bytes"
	"time"
	stdutf8 "unicode/utf8"

	"github.com/oldwired/fv-go/pkg/fv/utf8"
)

// Buffer is the text model an Editor view presents: the bytes, the
// undo/redo stack, and the file identity. Several Editors can share
// one Buffer (split panes over the same document); each keeps its own
// cursor, selection, and scroll state and is notified of every splice
// so it can remap them.
//
// All mutations funnel through splice, so undo coverage and listener
// notification are automatic.
type Buffer struct {
	data      []byte
	undoStack []editChange
	undoAt    int
	// savedAt is the undoAt value at the last SaveFile (0 for a fresh
	// or SetText-reset buffer, -1 when the saved state was dropped
	// with a redo tail and is no longer reachable). Modified is
	// derived from undoAt != savedAt on undo/redo so undoing past a
	// save — or redoing back onto it — reports dirtiness correctly.
	savedAt int

	Modified bool
	Filename string
	encoding utf8.FileEncoding

	// changeVersion is the monotonic counter emitted to listeners'
	// bufferChanged (and from there to Editor.OnChange). It is not
	// reset by SetText so consumers can tell "a brand-new buffer was
	// loaded" apart from "no edits since construction".
	changeVersion int

	listeners []bufferListener

	groupDepth int
	groupSeq   uint64
	groupDirty bool
}

// Splice describes one completed buffer mutation: the bytes at
// [Start, Start+OldLen) were replaced by NewLen bytes. Listeners use
// it to remap byte positions (cursors, bookmarks, decoration spans)
// and line-based state (scroll tops, fold regions).
type Splice struct {
	Start  int
	OldLen int
	NewLen int
	// StartLine is the 0-based line containing Start, computed
	// against the pre-splice content.
	StartLine int
	// LinesDelta is newline-count(new) − newline-count(old).
	LinesDelta int
	// Origin identifies who performed the edit (the acting *Editor,
	// or nil for direct Buffer calls). The acting editor manages its
	// own carets and skips the caret remap for its own splices.
	Origin any
}

type bufferListener interface {
	bufferSpliced(sp Splice)
	bufferReset()
	bufferChanged(version int)
	// bufferSaved fires after SaveFile clears Modified — a repaint
	// signal for dirty-tab indicators, deliberately NOT a version
	// bump (saving must not look like an LSP didChange).
	bufferSaved()
}

// NewBuffer returns an empty Buffer.
func NewBuffer() *Buffer {
	return &Buffer{encoding: utf8.EncUTF8}
}

// Text returns the buffer content as a string.
func (b *Buffer) Text() string { return string(b.data) }

// Bytes returns the live backing slice. Callers must treat it as
// read-only — mutating it bypasses undo and listener notification.
func (b *Buffer) Bytes() []byte { return b.data }

// Len returns the content length in bytes.
func (b *Buffer) Len() int { return len(b.data) }

// Version returns the current change counter.
func (b *Buffer) Version() int { return b.changeVersion }

// LineCount returns 1 + (count of '\n' bytes).
func (b *Buffer) LineCount() int {
	if len(b.data) == 0 {
		return 1
	}
	return 1 + bytes.Count(b.data, nlByte)
}

var nlByte = []byte{'\n'}

// SetText replaces the whole content. Wipes the undo history because
// the pre-swap state isn't reachable anymore; attached editors reset
// their view state (cursor to 0, scroll to top).
func (b *Buffer) SetText(s string) {
	b.data = []byte(s)
	b.Modified = false
	b.savedAt = 0
	b.ResetUndo()
	for _, l := range b.snapshotListeners() {
		l.bufferReset()
	}
	b.bumpVersion()
}

// ReplaceRange replaces the byte range [start, end) with text as one
// undo entry. Bounds are clamped to the buffer, swapped if reversed,
// and snapped back to UTF-8 rune starts. This is the intended entry
// point for host-driven edits (LSP TextEdits, refactoring tools):
// unlike SetText it preserves undo history and attached editors keep
// their cursors via remapping.
func (b *Buffer) ReplaceRange(start, end int, text string) {
	if start > end {
		start, end = end, start
	}
	start = b.clampToRuneStart(start)
	end = b.clampToRuneStart(end)
	b.splice(nil, start, end, []byte(text), -1, -1)
}

// OffsetAt returns the byte offset of (line, col). line is 0-based;
// col is a byte offset within the line. Both are clamped — a line
// past EOF maps to Len(), a col past the line end maps to the line
// end. The result is snapped to a rune start.
func (b *Buffer) OffsetAt(line, col int) int {
	start := 0
	for l := 0; l < line; l++ {
		nl := bytes.IndexByte(b.data[start:], '\n')
		if nl < 0 {
			return len(b.data)
		}
		start += nl + 1
	}
	lineLen := bytes.IndexByte(b.data[start:], '\n')
	if lineLen < 0 {
		lineLen = len(b.data) - start
	}
	if col < 0 {
		col = 0
	}
	if col > lineLen {
		col = lineLen
	}
	return b.clampToRuneStart(start + col)
}

// PositionFor returns the 0-based line and byte column of offset,
// clamped into the buffer and snapped to a rune start.
func (b *Buffer) PositionFor(offset int) (line, col int) {
	offset = b.clampToRuneStart(offset)
	line = bytes.Count(b.data[:offset], nlByte)
	ls := offset
	for ls > 0 && b.data[ls-1] != '\n' {
		ls--
	}
	return line, offset - ls
}

// clampToRuneStart returns pos moved back to the first byte of the
// UTF-8 rune it lands in (clamped into [0, Len]), so a stale byte
// offset can never sit mid-rune.
func (b *Buffer) clampToRuneStart(pos int) int {
	if pos < 0 {
		return 0
	}
	if pos > len(b.data) {
		return len(b.data)
	}
	for pos > 0 && pos < len(b.data) && !stdutf8.RuneStart(b.data[pos]) {
		pos--
	}
	return pos
}

// BeginGroup opens an undo group: every splice until the matching
// EndGroup shares one group id, and Undo/Redo replay the whole group
// atomically. Re-entrant — nested Begin/End pairs extend the same
// group.
func (b *Buffer) BeginGroup() {
	if b.groupDepth == 0 {
		b.groupSeq++
		b.groupDirty = false
	}
	b.groupDepth++
}

// EndGroup closes the innermost BeginGroup.
func (b *Buffer) EndGroup() {
	if b.groupDepth == 0 {
		return
	}
	b.groupDepth--
	if b.groupDepth == 0 && b.groupDirty {
		b.bumpVersion()
	}
}

// splice is the single mutation entry point: replace data[start:oldEnd]
// with newBytes, record the undo entry, notify listeners. origin and
// the cursor pair flow into the undo entry so an Editor-driven edit
// restores the caret on undo; pass nil/-1/-1 for host-driven edits.
func (b *Buffer) splice(origin any, start, oldEnd int, newBytes []byte, cursorBefore, cursorAfter int) {
	if start < 0 {
		start = 0
	}
	if start > len(b.data) {
		start = len(b.data)
	}
	if oldEnd > len(b.data) {
		oldEnd = len(b.data)
	}
	if oldEnd < start {
		oldEnd = start
	}
	old := append([]byte(nil), b.data[start:oldEnd]...)
	newCopy := append([]byte(nil), newBytes...)

	var group uint64
	if b.groupDepth > 0 {
		group = b.groupSeq
	}
	ch := editChange{
		start:        start,
		oldBytes:     old,
		newBytes:     newCopy,
		cursorBefore: cursorBefore,
		cursorAfter:  cursorAfter,
		timestamp:    time.Now(),
		group:        group,
		origin:       origin,
	}

	// Drop any redo tail before adding a new entry. If the tail held
	// the save point, the on-disk state is no longer reachable.
	if b.undoAt < len(b.undoStack) {
		b.undoStack = b.undoStack[:b.undoAt]
		if b.savedAt > b.undoAt {
			b.savedAt = -1
		}
	}

	// Try to coalesce with the previous change. Entries from
	// different groups (or grouped vs. ungrouped) never merge — each
	// multi-caret keystroke stays one discrete undo step. Entries
	// from different origins never merge either (typing in pane A
	// must not fuse with pane B's, nor with a host ReplaceRange), and
	// host edits (origin nil) are discrete operations that never
	// coalesce. The saved entry is also a barrier — merging into it
	// would make the on-disk state unreconstructable.
	merged := false
	if b.undoAt > 0 && b.undoAt != b.savedAt && ch.origin != nil {
		prev := &b.undoStack[b.undoAt-1]
		if prev.group == ch.group && prev.origin == ch.origin {
			switch coalesceKind(prev, &ch) {
			case mergeAppend:
				// Pure-insert typing: append both buffers. Adjacent
				// forward-deletes (Del key) also fit here — the second
				// delete is at the same start as the first, so the
				// chars-deleted-from-original line up in order.
				prev.newBytes = append(prev.newBytes, ch.newBytes...)
				prev.oldBytes = append(prev.oldBytes, ch.oldBytes...)
				prev.cursorAfter = ch.cursorAfter
				prev.timestamp = ch.timestamp
				merged = true
			case mergePrepend:
				// Backspace: characters were deleted right-to-left.
				// The new change is to the LEFT of prev; rebuild as one
				// span that re-inserts both in original (left-to-right)
				// order on undo.
				prev.start = ch.start
				prev.oldBytes = append(append([]byte(nil), ch.oldBytes...), prev.oldBytes...)
				prev.cursorAfter = ch.cursorAfter
				prev.timestamp = ch.timestamp
				merged = true
			case mergeNone:
				// Adjacent edits aren't coalescable — fall through to
				// the no-merge path below where we push a fresh entry.
			}
		}
	}
	if !merged {
		b.undoStack = append(b.undoStack, ch)
		b.undoAt = len(b.undoStack)
	}

	sp := Splice{
		Start:      start,
		OldLen:     oldEnd - start,
		NewLen:     len(newCopy),
		StartLine:  bytes.Count(b.data[:start], nlByte),
		LinesDelta: bytes.Count(newCopy, nlByte) - bytes.Count(old, nlByte),
		Origin:     origin,
	}

	tail := append([]byte(nil), b.data[oldEnd:]...)
	b.data = append(b.data[:start], newCopy...)
	b.data = append(b.data, tail...)
	b.Modified = true
	b.notifySpliced(sp)
	if b.groupDepth > 0 {
		b.groupDirty = true
	} else {
		b.bumpVersion()
	}
}

// Undo reverses the most-recent change (or whole group). Returns
// false at the bottom of the stack.
func (b *Buffer) Undo() bool {
	_, ok := b.undoCore(nil)
	if ok {
		b.bumpVersion()
	}
	return ok
}

// Redo re-applies the next change (or whole group). Returns false at
// the top.
func (b *Buffer) Redo() bool {
	_, ok := b.redoCore(nil)
	if ok {
		b.bumpVersion()
	}
	return ok
}

// undoCore reverses the top entry — or, for a grouped entry, the
// whole group in LIFO order. The returned cursor is the
// chronologically first entry's pre-edit caret (or its splice start
// when the edit was host-driven and recorded no caret), so the
// invoking editor can jump to the undone edit's site. Deliberately
// does NOT bump the change version: the caller settles its caret
// state first so OnChange handlers never observe a stale cursor.
func (b *Buffer) undoCore(origin any) (cursor int, ok bool) {
	if b.undoAt == 0 {
		return -1, false
	}
	group := b.undoStack[b.undoAt-1].group
	for {
		ch := &b.undoStack[b.undoAt-1]
		b.undoAt--
		end := ch.start + len(ch.newBytes)
		sp := Splice{
			Start:      ch.start,
			OldLen:     len(ch.newBytes),
			NewLen:     len(ch.oldBytes),
			StartLine:  bytes.Count(b.data[:ch.start], nlByte),
			LinesDelta: bytes.Count(ch.oldBytes, nlByte) - bytes.Count(ch.newBytes, nlByte),
			Origin:     origin,
		}
		tail := append([]byte(nil), b.data[end:]...)
		b.data = append(b.data[:ch.start], ch.oldBytes...)
		b.data = append(b.data, tail...)
		b.notifySpliced(sp)
		cursor = ch.cursorBefore
		if cursor < 0 {
			cursor = ch.start
		}
		if group == 0 || b.undoAt == 0 || b.undoStack[b.undoAt-1].group != group {
			break
		}
	}
	b.Modified = b.undoAt != b.savedAt
	return cursor, true
}

func (b *Buffer) redoCore(origin any) (cursor int, ok bool) {
	if b.undoAt >= len(b.undoStack) {
		return -1, false
	}
	group := b.undoStack[b.undoAt].group
	for {
		ch := &b.undoStack[b.undoAt]
		b.undoAt++
		end := ch.start + len(ch.oldBytes)
		sp := Splice{
			Start:      ch.start,
			OldLen:     len(ch.oldBytes),
			NewLen:     len(ch.newBytes),
			StartLine:  bytes.Count(b.data[:ch.start], nlByte),
			LinesDelta: bytes.Count(ch.newBytes, nlByte) - bytes.Count(ch.oldBytes, nlByte),
			Origin:     origin,
		}
		tail := append([]byte(nil), b.data[end:]...)
		b.data = append(b.data[:ch.start], ch.newBytes...)
		b.data = append(b.data, tail...)
		b.notifySpliced(sp)
		cursor = ch.cursorAfter
		if cursor < 0 {
			cursor = ch.start + len(ch.newBytes)
		}
		if group == 0 || b.undoAt >= len(b.undoStack) || b.undoStack[b.undoAt].group != group {
			break
		}
	}
	b.Modified = b.undoAt != b.savedAt
	return cursor, true
}

// CanUndo / CanRedo report whether the corresponding action will do
// anything. Useful for menu enabling.
func (b *Buffer) CanUndo() bool { return b.undoAt > 0 }
func (b *Buffer) CanRedo() bool { return b.undoAt < len(b.undoStack) }

// ResetUndo wipes the undo history. Called by SetText on full-buffer
// replacement so old state from before the swap doesn't leak through.
func (b *Buffer) ResetUndo() {
	b.undoStack = nil
	b.undoAt = 0
	b.savedAt = 0
}

// LoadFile reads path, detects its encoding (UTF-8 / UTF-16 / BOM /
// ANSI), and converts to UTF-8 in memory. Filename and encoding are
// recorded; the encoding is used by SaveFile to preserve the UTF-8
// BOM only — UTF-16 and ANSI files are saved as plain UTF-8.
func (b *Buffer) LoadFile(path string) error {
	data, err := readFile(path)
	if err != nil {
		return err
	}
	enc := utf8.DetectEncoding(data)
	body := utf8.ConvertToUTF8(data, enc)
	b.encoding = enc
	b.Filename = path
	b.SetText(string(body))
	return nil
}

// SaveFile writes the content to path as UTF-8 bytes. If the recorded
// encoding is EncUTF8BOM the file is prefixed with the UTF-8 BOM;
// otherwise no transformation is applied — UTF-16 LE/BE and ANSI
// (CP1252) files loaded via LoadFile are silently downgraded to
// UTF-8 on save. Hosts that need byte-for-byte round-trip of those
// encodings must re-encode before writing.
func (b *Buffer) SaveFile(path string) error {
	out := append([]byte(nil), b.data...)
	if b.encoding == utf8.EncUTF8BOM {
		out = append([]byte{0xEF, 0xBB, 0xBF}, out...)
	}
	if err := writeFile(path, out); err != nil {
		return err
	}
	b.Filename = path
	b.Modified = false
	b.savedAt = b.undoAt
	for _, l := range b.snapshotListeners() {
		l.bufferSaved()
	}
	return nil
}

// Encoding returns the encoding LoadFile detected (or utf8.EncUTF8
// for buffers populated via SetText / Append).
func (b *Buffer) Encoding() utf8.FileEncoding { return b.encoding }

// SetEncoding overrides the encoding hint. Currently only affects
// whether SaveFile prepends a UTF-8 BOM (when set to EncUTF8BOM).
func (b *Buffer) SetEncoding(enc utf8.FileEncoding) { b.encoding = enc }

func (b *Buffer) addListener(l bufferListener) {
	b.listeners = append(b.listeners, l)
}

func (b *Buffer) removeListener(l bufferListener) {
	for i, x := range b.listeners {
		if x == l {
			b.listeners = append(b.listeners[:i], b.listeners[i+1:]...)
			return
		}
	}
}

// snapshotListeners copies the listener list so a listener that
// detaches (or attaches another) mid-notification doesn't skew the
// iteration.
func (b *Buffer) snapshotListeners() []bufferListener {
	return append([]bufferListener(nil), b.listeners...)
}

func (b *Buffer) notifySpliced(sp Splice) {
	for _, l := range b.snapshotListeners() {
		l.bufferSpliced(sp)
	}
}

func (b *Buffer) bumpVersion() {
	b.changeVersion++
	for _, l := range b.snapshotListeners() {
		l.bufferChanged(b.changeVersion)
	}
}
