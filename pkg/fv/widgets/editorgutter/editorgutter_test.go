package editorgutter

import (
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/editor"
)

func numberedText(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("text\n")
	}
	return sb.String()
}

func newFoldedEditor() *editor.Editor {
	ed := editor.New(geom.NewRect(8, 0, 40, 10), nil, nil)
	ed.SetText(numberedText(20))
	ed.SetFoldRegions([]editor.FoldRegion{{StartLine: 1, EndLine: 4}})
	ed.Fold(1)
	return ed
}

func TestGutterRendersThroughFold(t *testing.T) {
	h := term.NewHeadless(48, 12)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	ed := newFoldedEditor()
	g := New(geom.NewRect(0, 0, 8, 10), ed, NewFolds(ed), NewLineNumbers(4))
	g.State |= consts.SfVisible | consts.SfExposed
	g.Draw()
	_ = h.Flush()

	rows := strings.Split(h.Snapshot(), "\n")
	// Row 0 = line 1, row 1 = line 2 (collapsed header, marker ▸ +
	// number 2), row 2 = line 6 (fold skipped lines 2-5).
	if !strings.Contains(rows[0], "1") {
		t.Errorf("row 0 = %q, want line number 1", rows[0])
	}
	if !strings.Contains(rows[1], "▸") || !strings.Contains(rows[1], "2") {
		t.Errorf("row 1 = %q, want collapsed marker + line number 2", rows[1])
	}
	if !strings.Contains(rows[2], "6") {
		t.Errorf("row 2 = %q, want line number 6 (post-fold)", rows[2])
	}
}

func TestGutterClickMapsThroughFold(t *testing.T) {
	ed := newFoldedEditor()
	g := New(geom.NewRect(0, 0, 8, 10), ed, NewFolds(ed))
	var clicked int
	g.OnClick = func(line int) { clicked = line }

	ev := drivers.Event{What: consts.EvMouseDown, Buttons: consts.MbLeftButton,
		Where: geom.Point{X: 1, Y: 2}}
	g.HandleEvent(&ev)
	if clicked != 5 {
		t.Errorf("click row 2 reported line %d, want 5", clicked)
	}
	if ev.What != consts.EvNothing {
		t.Error("gutter click must be consumed")
	}
}

func TestFoldsProviderGlyphs(t *testing.T) {
	ed := newFoldedEditor()
	p := NewFolds(ed)
	if text, _ := p.CellAt(1); !strings.HasPrefix(text, "▸") {
		t.Errorf("collapsed header cell = %q, want ▸", text)
	}
	ed.Unfold(1)
	if text, _ := p.CellAt(1); !strings.HasPrefix(text, "▾") {
		t.Errorf("expanded header cell = %q, want ▾", text)
	}
	if text, _ := p.CellAt(0); text != "  " {
		t.Errorf("plain line cell = %q, want spaces", text)
	}
}

func TestGutterClickToToggleFoldWiring(t *testing.T) {
	ed := newFoldedEditor()
	g := New(geom.NewRect(0, 0, 8, 10), ed, NewFolds(ed))
	// The documented host wiring: toggle when the clicked line heads a
	// region.
	g.OnClick = func(line int) {
		if ed.FoldMarkerAt(line) != 0 {
			ed.ToggleFold(line)
		}
	}
	ev := drivers.Event{What: consts.EvMouseDown, Buttons: consts.MbLeftButton,
		Where: geom.Point{X: 1, Y: 1}} // row 1 = collapsed header line 1
	g.HandleEvent(&ev)
	if ed.IsFolded(1) {
		t.Error("click on the header must unfold")
	}
}
