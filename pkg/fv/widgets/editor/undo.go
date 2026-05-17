package editor

import (
	"time"
)

// editChange is one undoable mutation: at byte position start, the
// range Data[start:start+len(oldBytes)] used to contain oldBytes; it
// was replaced with newBytes. Either side may be empty (pure
// insertion has oldBytes == nil; pure deletion has newBytes == nil).
//
// We snapshot cursor positions before and after so undo/redo can put
// the caret back where the user expects.
type editChange struct {
	start        int
	oldBytes     []byte
	newBytes     []byte
	cursorBefore int
	cursorAfter  int
	timestamp    time.Time
}

// coalesceWindow is the time gap inside which a fresh insert/delete
// can merge into the previous one. Picked to feel "single edit" for
// fast typing without merging across thinking pauses.
const coalesceWindow = 500 * time.Millisecond

// applyChange is the single mutation entry point. It captures the old
// bytes, performs the replacement, drops the redo tail, and either
// appends a new undo entry or coalesces into the previous one.
//
// All public mutating methods (Insert, Backspace, DeleteForward, etc.)
// route through here so undo coverage is automatic.
func (e *Editor) applyChange(start, oldEnd int, newBytes []byte, cursorAfter int) {
	if start < 0 {
		start = 0
	}
	if oldEnd > len(e.Data) {
		oldEnd = len(e.Data)
	}
	if oldEnd < start {
		oldEnd = start
	}
	old := append([]byte(nil), e.Data[start:oldEnd]...)
	newCopy := append([]byte(nil), newBytes...)

	ch := editChange{
		start:        start,
		oldBytes:     old,
		newBytes:     newCopy,
		cursorBefore: e.Cursor,
		cursorAfter:  cursorAfter,
		timestamp:    time.Now(),
	}

	// Drop any redo tail before adding a new entry.
	if e.undoAt < len(e.undoStack) {
		e.undoStack = e.undoStack[:e.undoAt]
	}

	// Try to coalesce with the previous change.
	merged := false
	if e.undoAt > 0 {
		prev := &e.undoStack[e.undoAt-1]
		kind := coalesceKind(prev, &ch)
		switch kind {
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
	if !merged {
		e.undoStack = append(e.undoStack, ch)
		e.undoAt = len(e.undoStack)
	}

	// Perform the replacement on Data.
	tail := append([]byte(nil), e.Data[oldEnd:]...)
	e.Data = append(e.Data[:start], newCopy...)
	e.Data = append(e.Data, tail...)
	e.Cursor = cursorAfter
	e.Modified = true
	e.notifyChange()
}

// notifyChange increments the change counter and fires OnChange.
// Centralized so applyChange / Undo / Redo all emit a single, ordered
// version sequence.
func (e *Editor) notifyChange() {
	e.changeVersion++
	if e.OnChange != nil {
		e.OnChange(e.changeVersion)
	}
}

// mergeMode describes how two adjacent changes coalesce.
type mergeMode int

const (
	mergeNone    mergeMode = iota
	mergeAppend            // typing right or forward-deleting: append next's bytes to prev's
	mergePrepend           // backspacing: next is to the LEFT of prev; prepend
)

// coalesceKind decides whether two adjacent changes should merge into
// one undo entry, and how. Merge when:
//   - Same edit "kind" (both pure inserts OR both pure deletions).
//   - Adjacent in byte space (so character-by-character typing or
//     repeated backspacing coalesces).
//   - Less than coalesceWindow has passed.
//   - Neither change crosses a newline (so paragraph breaks always
//     start a new undo group).
func coalesceKind(prev, next *editChange) mergeMode {
	if time.Since(prev.timestamp) > coalesceWindow {
		return mergeNone
	}
	if next.timestamp.Sub(prev.timestamp) > coalesceWindow {
		return mergeNone
	}
	prevInsert := len(prev.oldBytes) == 0 && len(prev.newBytes) > 0
	prevDelete := len(prev.newBytes) == 0 && len(prev.oldBytes) > 0
	nextInsert := len(next.oldBytes) == 0 && len(next.newBytes) > 0
	nextDelete := len(next.newBytes) == 0 && len(next.oldBytes) > 0
	if prevInsert && nextInsert {
		if next.start != prev.start+len(prev.newBytes) {
			return mergeNone
		}
		if containsByte(prev.newBytes, '\n') || containsByte(next.newBytes, '\n') {
			return mergeNone
		}
		return mergeAppend
	}
	if prevDelete && nextDelete {
		if containsByte(prev.oldBytes, '\n') || containsByte(next.oldBytes, '\n') {
			return mergeNone
		}
		// Forward delete (Del key): every press deletes at the same start.
		if next.start == prev.start {
			return mergeAppend
		}
		// Backspace: next.start is len(next.oldBytes) less than prev.start.
		if next.start+len(next.oldBytes) == prev.start {
			return mergePrepend
		}
	}
	return mergeNone
}

func containsByte(b []byte, c byte) bool {
	for _, x := range b {
		if x == c {
			return true
		}
	}
	return false
}

// Undo reverses the most-recent change. No-op when at the bottom of
// the stack.
func (e *Editor) Undo() {
	if e.undoAt == 0 {
		return
	}
	e.undoAt--
	ch := &e.undoStack[e.undoAt]
	// Reverse: Data[start:start+len(newBytes)] → oldBytes.
	end := ch.start + len(ch.newBytes)
	tail := append([]byte(nil), e.Data[end:]...)
	e.Data = append(e.Data[:ch.start], ch.oldBytes...)
	e.Data = append(e.Data, tail...)
	e.Cursor = ch.cursorBefore
	e.SelAnchor = -1
	e.Modified = e.undoAt > 0
	e.adjustScroll()
	e.notifyChange()
}

// Redo re-applies the next change in the stack. No-op at the top.
func (e *Editor) Redo() {
	if e.undoAt >= len(e.undoStack) {
		return
	}
	ch := &e.undoStack[e.undoAt]
	end := ch.start + len(ch.oldBytes)
	tail := append([]byte(nil), e.Data[end:]...)
	e.Data = append(e.Data[:ch.start], ch.newBytes...)
	e.Data = append(e.Data, tail...)
	e.Cursor = ch.cursorAfter
	e.SelAnchor = -1
	e.undoAt++
	e.Modified = true
	e.adjustScroll()
	e.notifyChange()
}

// CanUndo / CanRedo report whether the corresponding action will do
// anything. Useful for menu enabling.
func (e *Editor) CanUndo() bool { return e.undoAt > 0 }
func (e *Editor) CanRedo() bool { return e.undoAt < len(e.undoStack) }

// ResetUndo wipes the undo history. Called by SetText on full-buffer
// replacement so old state from before the swap doesn't leak through.
func (e *Editor) ResetUndo() {
	e.undoStack = nil
	e.undoAt = 0
}
