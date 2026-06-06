package editor

import (
	"time"
)

// editChange is one undoable mutation: at byte position start, the
// range data[start:start+len(oldBytes)] used to contain oldBytes; it
// was replaced with newBytes. Either side may be empty (pure
// insertion has oldBytes == nil; pure deletion has newBytes == nil).
//
// Cursor positions before and after are snapshotted (when the edit
// came from an Editor) so undo/redo can put the caret back where the
// user expects; -1 marks a host-driven edit with no caret to restore.
// group links the entries of one BeginGroup/EndGroup transaction; 0
// means ungrouped. origin is the acting editor (nil for host edits)
// — coalescing requires matching origins so keystrokes from two
// panes, or typing plus an adjacent host edit, stay separate undo
// steps.
type editChange struct {
	start        int
	oldBytes     []byte
	newBytes     []byte
	cursorBefore int
	cursorAfter  int
	timestamp    time.Time
	group        uint64
	origin       any
}

// coalesceWindow is the time gap inside which a fresh insert/delete
// can merge into the previous one. Picked to feel "single edit" for
// fast typing without merging across thinking pauses.
const coalesceWindow = 500 * time.Millisecond

// applyChange routes an Editor-driven mutation into the shared
// Buffer, recording this editor's caret for undo. All user-input
// mutating methods (Insert, Backspace, DeleteForward, etc.) come
// through here so undo coverage is automatic. The caret is placed
// BEFORE the splice runs: OnChange fires from inside the splice and
// its handlers must observe the post-edit caret, never a stale one
// pointing past the new EOF.
func (e *Editor) applyChange(start, oldEnd int, newBytes []byte, cursorAfter int) {
	before := e.Cursor
	e.Cursor = cursorAfter
	e.Buf.splice(e, start, oldEnd, newBytes, before, cursorAfter)
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

// Undo reverses the most-recent change (or whole group) on the shared
// buffer and jumps this editor's caret to the undone edit's site —
// even when the edit was made through another pane. No-op at the
// bottom of the stack — and under ReadOnly: a read-only pane over a
// shared buffer must not revert the writable pane's edits. The
// version bump (OnChange) fires only after this editor's caret state
// is settled.
func (e *Editor) Undo() {
	if e.ReadOnly {
		return
	}
	cursor, ok := e.Buf.undoCore(e)
	if !ok {
		return
	}
	e.Cursor = e.Buf.clampToRuneStart(cursor)
	e.SelAnchor = -1
	e.CollapseCarets()
	e.adjustScroll()
	e.Buf.bumpVersion()
}

// Redo re-applies the next change (or whole group). No-op at the top
// and under ReadOnly.
func (e *Editor) Redo() {
	if e.ReadOnly {
		return
	}
	cursor, ok := e.Buf.redoCore(e)
	if !ok {
		return
	}
	e.Cursor = e.Buf.clampToRuneStart(cursor)
	e.SelAnchor = -1
	e.CollapseCarets()
	e.adjustScroll()
	e.Buf.bumpVersion()
}

// CanUndo / CanRedo report whether the corresponding action will do
// anything. Useful for menu enabling.
func (e *Editor) CanUndo() bool { return e.Buf.CanUndo() }
func (e *Editor) CanRedo() bool { return e.Buf.CanRedo() }

// ResetUndo wipes the undo history.
func (e *Editor) ResetUndo() { e.Buf.ResetUndo() }
