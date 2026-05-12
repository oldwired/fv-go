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
	for _, line := range strings.Split(string(e.Data), "\n") {
		if len(line) > 20 {
			t.Errorf("line over margin: %q", line)
		}
	}
	// All the words must survive.
	got := strings.Join(strings.Fields(string(e.Data)), " ")
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
	if !strings.Contains(string(e.Data), "untouched second para") {
		t.Errorf("second paragraph mangled:\n%s", string(e.Data))
	}
}

func TestReformatIsOneUndoEntry(t *testing.T) {
	e := New(geom.NewRect(0, 0, 80, 10), nil, nil)
	e.RightMargin = 10
	e.SetText("the quick brown fox")
	e.Cursor = 0
	e.Reformat()
	before := string(e.Data)
	e.Undo()
	if got := string(e.Data); got != "the quick brown fox" {
		t.Errorf("single undo should restore original: got %q", got)
	}
	e.Redo()
	if got := string(e.Data); got != before {
		t.Errorf("redo: got %q, want %q", got, before)
	}
}

func TestTrimTrailingWS(t *testing.T) {
	e := New(geom.NewRect(0, 0, 80, 10), nil, nil)
	e.SetText("hello   \nworld \t  \nfoo")
	e.TrimTrailingWS()
	if got, want := string(e.Data), "hello\nworld\nfoo"; got != want {
		t.Errorf("TrimTrailingWS: got %q, want %q", got, want)
	}
}

func TestWrapPoints(t *testing.T) {
	cases := []struct {
		in     string
		margin int
		want   []int
	}{
		{"hello world", 5, []int{6}}, // wrap after "hello "
		{"abc def ghi", 7, []int{8}}, // wrap after "abc def "
		{"unbreakablelongword", 5, []int{5, 10, 15}},
		{"short", 80, nil},
	}
	for _, c := range cases {
		got := wrapPoints([]byte(c.in), c.margin)
		if len(got) != len(c.want) {
			t.Errorf("%q@%d: got %v, want %v", c.in, c.margin, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q@%d: got %v, want %v", c.in, c.margin, got, c.want)
				break
			}
		}
	}
}
