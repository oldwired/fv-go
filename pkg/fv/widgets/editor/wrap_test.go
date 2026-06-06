package editor

import (
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

func TestReformatBasic(t *testing.T) {
	e := New(geom.NewRect(0, 0, 80, 10), nil, nil)
	e.RightMargin = 20
	e.SetText("The quick brown fox jumps over the lazy dog. " +
		"The quick brown fox jumps over the lazy dog.")
	e.Cursor = 5
	e.Reformat()
	for _, line := range strings.Split(e.Text(), "\n") {
		if len(line) > 20 {
			t.Errorf("line over margin: %q", line)
		}
	}
	// All the words must survive.
	got := strings.Join(strings.Fields(e.Text()), " ")
	want := "The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog."
	if got != want {
		t.Errorf("words don't round-trip:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestReformatStopsAtBlankLine(t *testing.T) {
	e := New(geom.NewRect(0, 0, 80, 10), nil, nil)
	e.RightMargin = 10
	e.SetText("para one with many words to wrap\n\nuntouched second para")
	e.Cursor = 5
	e.Reformat()
	if !strings.Contains(e.Text(), "untouched second para") {
		t.Errorf("second paragraph mangled:\n%s", e.Text())
	}
}

func TestReformatIsOneUndoEntry(t *testing.T) {
	e := New(geom.NewRect(0, 0, 80, 10), nil, nil)
	e.RightMargin = 10
	e.SetText("the quick brown fox")
	e.Cursor = 0
	e.Reformat()
	before := e.Text()
	e.Undo()
	if got := e.Text(); got != "the quick brown fox" {
		t.Errorf("single undo should restore original: got %q", got)
	}
	e.Redo()
	if got := e.Text(); got != before {
		t.Errorf("redo: got %q, want %q", got, before)
	}
}

func TestTrimTrailingWS(t *testing.T) {
	e := New(geom.NewRect(0, 0, 80, 10), nil, nil)
	e.SetText("hello   \nworld \t  \nfoo")
	e.TrimTrailingWS()
	if got, want := e.Text(), "hello\nworld\nfoo"; got != want {
		t.Errorf("TrimTrailingWS: got %q, want %q", got, want)
	}
}
