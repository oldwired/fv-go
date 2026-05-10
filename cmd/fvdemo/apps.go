package main

import (
	"github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/editor"
	"github.com/oldwired/fv-go/pkg/fv/widgets/grid"
	"github.com/oldwired/fv-go/pkg/fv/widgets/hexedit"
	"github.com/oldwired/fv-go/pkg/fv/widgets/stddlg"
)

// showHexEdit opens a hex editor window seeded with a preview buffer.
// Use Tab to switch between hex and ASCII columns; arrows navigate;
// hex digits / printable ASCII edit in place.
func showHexEdit(a *app.Application) {
	preview := []byte(
		"Welcome to fv-go's hex editor.\n" +
			"Tab toggles hex / ASCII column.\n" +
			"Arrows / PgUp / PgDn navigate.\n" +
			"Type to overwrite the byte at the caret.\n" +
			"Modified bytes are highlighted.\n",
	)
	src := hexedit.NewMemorySource(preview)
	win := views.NewWindow(geom.NewRect(2, 2, 80, 22), "Hex Editor", 0)
	w, h := win.Size.X, win.Size.Y
	body := hexedit.New(geom.NewRect(1, 1, w-1, h-1), src)
	win.Insert(body)
	a.Desktop.InsertWindow(win)
}

// showEditor opens a text editor window. Loads a stub buffer to show
// off the controls; users can also save to a real file via the menu.
//
// The Window's interior, after the frame, runs cols 1..Width-2 and
// rows 1..Height-2. Scrollbar lives at col Width-2 (one inside the
// frame's right border at col Width-1), and the editor body ends one
// column before the scrollbar.
func showEditor(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 70, 20), "Text Editor", 0)
	w, h := win.Size.X, win.Size.Y
	scroll := views.NewScrollBar(geom.NewRect(w-2, 1, w-1, h-1))
	win.Insert(scroll)
	ed := editor.New(geom.NewRect(1, 1, w-2, h-1), nil, scroll)
	ed.SetText(
		"# fv-go editor demo\n\n" +
			"Type freely. Up/Down/Left/Right move; Home/End jump; PgUp/PgDn page.\n" +
			"Hold Shift while moving to extend a selection.\n" +
			"Ctrl+C / Ctrl+X / Ctrl+V copy / cut / paste through the clipboard.\n" +
			"Ctrl+A selects everything; Ctrl+Home / Ctrl+End jump to top/bottom.\n\n" +
			"This editor handles UTF-8 natively. Try é, 日本語, ✨ — all fine.\n",
	)
	win.Insert(ed)
	a.Desktop.InsertWindow(win)
}

// showGrid opens a grid window with sample tabular data.
func showGrid(a *app.Application) {
	win := views.NewWindow(geom.NewRect(1, 1, 70, 20), "Data Grid", 0)
	w, h := win.Size.X, win.Size.Y
	scroll := views.NewScrollBar(geom.NewRect(w-2, 1, w-1, h-1))
	win.Insert(scroll)
	g := grid.New(geom.NewRect(1, 1, w-2, h-1), []grid.Column{
		{Title: "Name", Width: 20, Align: grid.AlignLeft},
		{Title: "Qty", Width: 6, Align: grid.AlignRight},
		{Title: "Price", Width: 10, Align: grid.AlignRight},
		{Title: "Notes", Width: 30, Align: grid.AlignLeft},
	}, nil, scroll)
	g.SetRows([][]string{
		{"Apples", "12", "$0.55", "Local farm"},
		{"Bananas", "8", "$0.30", "Imported"},
		{"Cherries", "100", "$3.20", "Seasonal"},
		{"Dates", "20", "$5.00", "Medjool"},
		{"Elderberry", "4", "$8.75", "For tinctures"},
		{"Figs", "15", "$2.10", ""},
		{"Grapes", "50", "$1.40", "Red seedless"},
		{"Honeydew", "3", "$4.00", "Ripe"},
	})
	win.Insert(g)
	a.Desktop.InsertWindow(win)
}

// showEditorWithFile opens the file open dialog, then loads the chosen
// file into the editor. Demonstrates StdDlg + editor working together.
func showEditorWithFile(a *app.Application) {
	path, ok := stddlg.Show(&a.Desktop.Group, stddlg.ModeOpen, "Open File", "", "*")
	if !ok {
		return
	}
	win := views.NewWindow(geom.NewRect(1, 1, 80, 24), "Editor — "+path, 0)
	w, h := win.Size.X, win.Size.Y
	scroll := views.NewScrollBar(geom.NewRect(w-2, 1, w-1, h-1))
	win.Insert(scroll)
	ed := editor.New(geom.NewRect(1, 1, w-2, h-1), nil, scroll)
	if err := ed.LoadFile(path); err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"Couldn't open %s:\n%s", []any{path, err.Error()}, msgbox.OKOnly)
		return
	}
	win.Insert(ed)
	a.Desktop.InsertWindow(win)
}

// dispatchApp wires the Test → Apps submenu commands.
func dispatchApp(a *app.Application, cmd uint16) bool {
	switch cmd {
	case cmAppHexEdit:
		showHexEdit(a)
	case cmAppEditor:
		showEditor(a)
	case cmAppEditorOpen:
		showEditorWithFile(a)
	case cmAppGrid:
		showGrid(a)
	default:
		return false
	}
	return true
}

const (
	cmAppHexEdit    uint16 = 240
	cmAppEditor     uint16 = 241
	cmAppEditorOpen uint16 = 242
	cmAppGrid       uint16 = 243
)

// Suppress unused-import warning if no path needs dialogs directly.
var _ = dialogs.Dialog{}
var _ = consts.CmOK
