package editor

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

func newTestEditor() *Editor {
	return New(geom.NewRect(0, 0, 40, 10), nil, nil)
}

func TestUndoSimpleInsert(t *testing.T) {
	e := newTestEditor()
	e.Insert("hello")
	if got := string(e.Data); got != "hello" {
		t.Fatalf("after insert: %q", got)
	}
	e.Undo()
	if got := string(e.Data); got != "" {
		t.Fatalf("after undo: %q (want empty)", got)
	}
	e.Redo()
	if got := string(e.Data); got != "hello" {
		t.Fatalf("after redo: %q (want %q)", got, "hello")
	}
}

func TestUndoCoalesceTyping(t *testing.T) {
	e := newTestEditor()
	// Five single-char inserts within the coalesce window — should
	// merge into a single undo entry.
	for _, r := range "hello" {
		e.Insert(string(r))
	}
	if got := string(e.Data); got != "hello" {
		t.Fatalf("data: %q", got)
	}
	e.Undo()
	if got := string(e.Data); got != "" {
		t.Fatalf("after single undo, expected empty (coalesced), got %q", got)
	}
}

func TestUndoNoCoalesceAcrossNewline(t *testing.T) {
	e := newTestEditor()
	e.Insert("ab")
	e.Insert("\n")
	e.Insert("cd")
	// First undo removes "cd".
	e.Undo()
	if got := string(e.Data); got != "ab\n" {
		t.Fatalf("undo 1: %q", got)
	}
	// Second undo removes "\n".
	e.Undo()
	if got := string(e.Data); got != "ab" {
		t.Fatalf("undo 2: %q", got)
	}
	// Third undo removes "ab".
	e.Undo()
	if got := string(e.Data); got != "" {
		t.Fatalf("undo 3: %q", got)
	}
}

func TestUndoBackspaceCoalesce(t *testing.T) {
	e := newTestEditor()
	e.SetText("hello")
	e.Cursor = 5
	e.Backspace()
	e.Backspace()
	e.Backspace()
	if got := string(e.Data); got != "he" {
		t.Fatalf("after 3 backspaces: %q", got)
	}
	e.Undo()
	if got := string(e.Data); got != "hello" {
		t.Fatalf("undo should restore all 3 backspaces: %q", got)
	}
}

func TestUndoDropsRedoOnNewEdit(t *testing.T) {
	e := newTestEditor()
	e.Insert("a")
	e.Insert("b")
	e.Undo()
	// "b" gone, but typing "c" should drop the "b" redo opportunity.
	e.Insert("c")
	if e.CanRedo() {
		t.Errorf("redo should not be available after new edit")
	}
}

func TestUndoReplaceAllIsOneEntry(t *testing.T) {
	e := newTestEditor()
	e.SetText("the quick brown the fox the dog")
	n := e.ReplaceAll("the", "THE", true)
	if n != 3 {
		t.Fatalf("ReplaceAll: %d (want 3)", n)
	}
	e.Undo()
	if got := string(e.Data); got != "the quick brown the fox the dog" {
		t.Fatalf("after undo: %q", got)
	}
}

func TestUndoCanFlags(t *testing.T) {
	e := newTestEditor()
	if e.CanUndo() {
		t.Error("fresh editor should not CanUndo")
	}
	e.Insert("a")
	if !e.CanUndo() {
		t.Error("after insert, CanUndo should be true")
	}
	e.Undo()
	if !e.CanRedo() {
		t.Error("after undo, CanRedo should be true")
	}
	if e.CanUndo() {
		t.Error("after undo of single change, CanUndo should be false")
	}
}
