package editor

import (
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

func TestDecorationsSetClearReplace(t *testing.T) {
	e := newTestEditor()
	e.SetText("0123456789")
	e.SetDecorations("hl", []Decoration{{Start: 2, End: 5, Attr: 0x42}})
	if got := e.Decorations("hl"); len(got) != 1 || got[0].Start != 2 {
		t.Fatalf("Decorations = %+v", got)
	}
	e.SetDecorations("hl", []Decoration{{Start: 6, End: 8, Attr: 0x42}})
	if got := e.Decorations("hl"); len(got) != 1 || got[0].Start != 6 {
		t.Fatalf("replace = %+v", got)
	}
	e.ClearDecorations("hl")
	if got := e.Decorations("hl"); len(got) != 0 {
		t.Fatalf("after clear = %+v", got)
	}
}

func TestDecorationsClampGarbage(t *testing.T) {
	e := newTestEditor()
	e.SetText("aé") // é at [1,3)
	e.SetDecorations("x", []Decoration{
		{Start: -5, End: 2, Attr: 1},  // negative start; end mid-rune
		{Start: 90, End: 99, Attr: 2}, // past EOF → empty → dropped
		{Start: 3, End: 3, Attr: 3},   // empty → dropped
	})
	got := e.Decorations("x")
	if len(got) != 1 || got[0].Start != 0 || got[0].End != 1 {
		t.Fatalf("clamped = %+v, want [{0 1 1}]", got)
	}
}

func TestDecorationsNamespaceOverlayDeterministic(t *testing.T) {
	e := newTestEditor()
	e.SetText("0123456789")
	// "b" sorts after "a" → "b" wins where they overlap, regardless of
	// the order the host set them.
	e.SetDecorations("b", []Decoration{{Start: 4, End: 8, Attr: 0xB}})
	e.SetDecorations("a", []Decoration{{Start: 2, End: 6, Attr: 0xA}})
	merged := e.mergedDecorations()
	want := []Decoration{{2, 4, 0xA}, {4, 8, 0xB}}
	if len(merged) != len(want) {
		t.Fatalf("merged = %+v, want %+v", merged, want)
	}
	for i := range want {
		if merged[i] != want[i] {
			t.Errorf("merged[%d] = %+v, want %+v", i, merged[i], want[i])
		}
	}
}

func TestDecorationsRemapAcrossEdits(t *testing.T) {
	e := newTestEditor()
	e.SetText("hello world")
	e.SetDecorations("hl", []Decoration{{Start: 6, End: 11, Attr: 7}})
	e.MoveCursor(0, false)
	e.Insert("say ")
	got := e.Decorations("hl")
	if len(got) != 1 || got[0].Start != 10 || got[0].End != 15 {
		t.Fatalf("after insert above = %+v, want [{10 15 7}]", got)
	}
	// Deleting the decorated text drops the decoration.
	e.ReplaceRange(10, 15, "")
	if got := e.Decorations("hl"); len(got) != 0 {
		t.Fatalf("after delete = %+v, want empty", got)
	}
}

func TestDecorationDrawPriority(t *testing.T) {
	h := term.NewHeadless(40, 5)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	e := newTestEditor()
	e.Size.Y = 5
	e.State |= consts.SfVisible | consts.SfExposed
	e.SetText("abcdefghij")
	const decAttr = 0x99
	e.SetDecorations("hl", []Decoration{{Start: 2, End: 8, Attr: decAttr}})
	// Selection [5,7) overlaps the decoration's tail.
	e.SelAnchor = 5
	e.Cursor = 7
	e.Draw()
	_ = h.Flush()

	pal := theme.Get()
	if got := h.GetCell(1, 0).Attr; got != pal.EditorText {
		t.Errorf("cell 1 attr = %#x, want normal %#x", got, pal.EditorText)
	}
	if got := h.GetCell(3, 0).Attr; got != decAttr {
		t.Errorf("cell 3 attr = %#x, want decoration %#x", got, decAttr)
	}
	if got := h.GetCell(5, 0).Attr; got != pal.InputArrow {
		t.Errorf("cell 5 attr = %#x, want selection %#x (selection beats decoration)", got, pal.InputArrow)
	}
	if got := h.GetCell(7, 0).Attr; got != decAttr {
		t.Errorf("cell 7 attr = %#x, want decoration %#x (past selection)", got, decAttr)
	}
}

func TestDecorationBeatsSyntax(t *testing.T) {
	h := term.NewHeadless(40, 5)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	e := newTestEditor()
	e.Size.Y = 5
	e.State |= consts.SfVisible | consts.SfExposed
	e.SetText("keyword rest")
	e.Colorer = staticColorer{ColorSpan{Start: 0, End: 7, Attr: 0x11}}
	const decAttr = 0x77
	e.SetDecorations("hl", []Decoration{{Start: 0, End: 3, Attr: decAttr}})
	e.Draw()
	_ = h.Flush()

	if got := h.GetCell(1, 0).Attr; got != decAttr {
		t.Errorf("cell 1 = %#x, want decoration %#x over syntax", got, decAttr)
	}
	if got := h.GetCell(5, 0).Attr; got != 0x11 {
		t.Errorf("cell 5 = %#x, want syntax %#x", got, 0x11)
	}
}

type staticColorer []ColorSpan

func (s staticColorer) Tokenize(string) []ColorSpan { return s }

func TestDecorationAcrossFoldBoundaryDoesntPanic(t *testing.T) {
	h := term.NewHeadless(40, 12)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	e := newTestEditor()
	e.State |= consts.SfVisible | consts.SfExposed
	e.SetText(strings.Repeat("text line\n", 10))
	e.SetFoldRegions([]FoldRegion{{StartLine: 1, EndLine: 4}})
	e.Fold(1)
	// Decoration spanning from the header into hidden lines and out.
	e.SetDecorations("hl", []Decoration{{Start: 12, End: 55, Attr: 0x55}})
	e.Draw()
	_ = h.Flush()
	// Visible header tail cell decorated; row 2 shows line 5 inside the
	// decorated range.
	if got := h.GetCell(3, 1).Attr; got != 0x55 {
		t.Errorf("header cell = %#x, want decorated", got)
	}
}
