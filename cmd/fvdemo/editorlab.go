package main

import (
	"fmt"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/editor"
	"github.com/oldwired/fv-go/pkg/fv/widgets/editorgutter"
	"github.com/oldwired/fv-go/pkg/fv/widgets/hoverpopup"
	"github.com/oldwired/fv-go/pkg/fv/widgets/popupmenu"
	"github.com/oldwired/fv-go/pkg/fv/widgets/syntax"
	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"
)

// showEditorLab opens the IDE-feature playground: a file tree with a
// right-click context menu, two split panes sharing one buffer (live
// edits + shared undo), a fold gutter, and hotkeys exercising
// snippets, decorations, and the hover popup.
func showEditorLab(a *app.Application) {
	win := views.NewWindow(geom.NewRect(0, 0, 79, 23),
		"Editor Lab — Alt-S snippet · Alt-D highlights · Alt-H hover", 0)
	w, h := win.Size.X, win.Size.Y

	const treeW = 18
	tree := treeview.New(geom.NewRect(1, 1, treeW, h-1), labTreeNodes())
	// Fixed-width sidebar: stretch vertically only, the split area owns
	// the horizontal growth.
	tree.GrowMode = consts.GfGrowHiY
	tree.OnContextMenu = func(n *treeview.Node, where geom.Point) {
		// where is in screen coords; the popup is hosted by (and
		// positioned relative to) the Desktop.
		dx, dy := a.Desktop.ScreenOrigin()
		at := geom.Point{X: where.X - dx, Y: where.Y - dy}
		actions := []string{"Open", "Rename…", "Delete"}
		pop := popupmenu.New(at, actions, 24)
		if pick := pop.Run(&a.Desktop.Group); pick >= 0 {
			msgbox.Showf(&a.Desktop.Group, msgbox.Info,
				"%s → %q", []any{actions[pick], n.Label}, msgbox.OKOnly)
		}
	}
	win.Insert(tree)

	// One shared buffer, two panes: edits in either reflect live in the
	// other; undo is shared.
	const gutterW = 8
	splitArea := geom.NewRect(treeW, 1, w-2, h-1)
	topH := (splitArea.B.Y - splitArea.A.Y) / 2
	paneW := splitArea.B.X - splitArea.A.X

	ed1 := editor.New(geom.NewRect(gutterW, 0, paneW, topH), nil, nil)
	ed1.Colorer = syntax.GoSyntax().ToEditorColorer()
	ed1.SetText(labSample())
	ed1.SetFoldRegions(labFoldRegions())

	gut := editorgutter.New(geom.NewRect(0, 0, gutterW, topH), ed1,
		editorgutter.NewFolds(ed1), editorgutter.NewLineNumbers(4))
	gut.OnClick = func(line int) {
		if ed1.FoldMarkerAt(line) != 0 {
			ed1.ToggleFold(line)
		}
	}
	top := views.NewGroup(geom.NewRect(0, 0, paneW, topH))
	top.Insert(gut)
	top.Insert(ed1)

	ed2 := editor.NewShared(geom.NewRect(0, 0, paneW, 5), nil, nil, ed1.Buf)
	ed2.Colorer = syntax.GoSyntax().ToEditorColorer()

	split := views.NewSplitGroup(splitArea, views.SplitHorizontal, topH)
	split.SetPanels(top, ed2)
	win.Insert(split)

	keys := &labKeys{
		Base: views.NewBase(geom.Rect{}),
		ed1:  ed1, ed2: ed2,
		win: win,
		pop: hoverpopup.New(),
	}
	keys.SetSelf(keys)
	keys.Options |= consts.OfPreProcess
	win.Insert(keys)

	a.Desktop.InsertWindow(win)
}

// labKeys handles the lab's demo hotkeys via the keyboard pre-process
// pass: Alt-S inserts an LSP snippet (Tab/Shift-Tab navigate, mirrors
// edit together), Alt-D toggles document-highlight decorations on
// every "Greet" in both panes, Alt-H shows a hover popup at the caret.
type labKeys struct {
	views.Base
	ed1, ed2 *editor.Editor
	win      *views.Window
	pop      *hoverpopup.HoverPopup
	decsOn   bool
}

func (l *labKeys) GetTypeID() string { return "labkeys" }

func (l *labKeys) Draw() {}

func (l *labKeys) HandleEvent(ev *drivers.Event) {
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbAltS:
		_ = l.ed1.InsertSnippet("for ${1:i} := 0; $1 < ${2:n}; $1++ {\n\t$0\n}")
		// Focus must start at the editor's direct owner — Group.Focus
		// only matches direct children; the upward propagation carries
		// it through split → window from there.
		if l.ed1.Owner != nil {
			l.ed1.Owner.Focus(l.ed1.Self())
		}
	case consts.KbAltD:
		l.decsOn = !l.decsOn
		for _, ed := range []*editor.Editor{l.ed1, l.ed2} {
			if !l.decsOn {
				ed.ClearDecorations("greet")
				continue
			}
			var decs []editor.Decoration
			text := ed.Text()
			attr := theme.Get().EditorSelected
			for at := 0; ; {
				i := strings.Index(text[at:], "Greet")
				if i < 0 {
					break
				}
				decs = append(decs, editor.Decoration{
					Start: at + i, End: at + i + len("Greet"), Attr: attr})
				at += i + len("Greet")
			}
			ed.SetDecorations("greet", decs)
		}
		views.MarkDirty()
	case consts.KbAltH:
		if cell, ok := l.ed1.CellOf(l.ed1.Cursor); ok {
			line, col := l.ed1.PositionFor(l.ed1.Cursor)
			// Editor-local cell → screen → window-local (the popup's
			// host is the window group).
			ex, ey := l.ed1.ScreenOrigin()
			wx, wy := l.win.ScreenOrigin()
			at := geom.Point{X: cell.X + ex - wx, Y: cell.Y + ey - wy}
			l.pop.Show(&l.win.Group, at, fmt.Sprintf(
				"func Greet(name string) string\n\nGreet says hello to the world.\nCaret: line %d, col %d",
				line+1, col+1))
		}
	default:
		return
	}
	l.ClearEvent(ev)
}

func labTreeNodes() []*treeview.Node {
	return []*treeview.Node{{
		Label:    "turbogo (r-click!)",
		Expanded: true,
		Children: []*treeview.Node{
			{Label: "cmd", Expanded: true, Children: []*treeview.Node{
				{Label: "main.go"},
			}},
			{Label: "internal", Children: []*treeview.Node{
				{Label: "lsp.go"},
				{Label: "ui.go"},
			}},
			{Label: "go.mod"},
		},
	}}
}

func labSample() string {
	return `// Editor Lab: both panes share this buffer.
package main

import (
	"fmt"
	"strings"
)

func Greet(name string) string {
	if name == "" {
		return "Hello, world!"
	}
	return fmt.Sprintf("Hello, %s!", name)
}

func Shout(name string) string {
	return strings.ToUpper(Greet(name))
}

func main() {
	for i := 0; i < 3; i++ {
		fmt.Println(Shout("fv-go"))
	}
}
`
}

func labFoldRegions() []editor.FoldRegion {
	return []editor.FoldRegion{
		{StartLine: 3, EndLine: 6},   // import block
		{StartLine: 8, EndLine: 13},  // Greet
		{StartLine: 9, EndLine: 11},  // if (nested)
		{StartLine: 15, EndLine: 17}, // Shout
		{StartLine: 19, EndLine: 23}, // main
		{StartLine: 20, EndLine: 22}, // for (nested)
	}
}
