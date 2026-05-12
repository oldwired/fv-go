package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"

	"github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/validators"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/grid"
	"github.com/oldwired/fv-go/pkg/fv/widgets/stddlg"
)

// Command IDs local to the grid demo. Picked from a free range so they
// don't collide with the cmAppXxx allocations.
const (
	cmGridDemoLoad uint16 = 700 + iota
	cmGridDemoSave
	cmGridDemoClearFilters
	cmGridDemoAddRow
	cmGridDemoDelRow
	cmGridDemoToggleFilter
	cmGridDemoResetSort
	cmGridDemoCopy
	cmGridDemoStats
)

// gridDemoWindow wraps a Window with our grid + toolbar buttons. The
// embedded Window inherits its frame / move / close behavior; we only
// override HandleEvent so toolbar commands can drive the inner grid.
type gridDemoWindow struct {
	views.Window
	app     *app.Application
	grid    *grid.StringGrid
	stat    *dialogs.StaticText
	hint    *dialogs.StaticText
	buttons []*dialogs.Button
}

func newGridDemoWindow(a *app.Application, bounds geom.Rect) *gridDemoWindow {
	w := &gridDemoWindow{app: a}
	views.InitWindow(&w.Window, bounds, "Data Grid — full demo", 0)
	w.SetSelf(w)
	w.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY

	W, H := w.Size.X, w.Size.Y

	// Hint row at the top of the interior. GfGrowHiX so the right
	// edge follows the window when resized.
	hint := "Sort: click header  ·  Filter: type in filter row  ·  Edit: Enter/F2  ·  Copy: Ctrl+C  ·  Range: Shift+arrows"
	w.hint = dialogs.NewStaticText(geom.NewRect(1, 1, W-1, 2), hint)
	w.hint.GrowMode = consts.GfGrowHiX
	w.Insert(w.hint)

	// Status row at the bottom, just above the toolbar. Anchor both
	// edges to the bottom so the row stays glued there.
	w.stat = dialogs.NewStaticText(geom.NewRect(1, H-4, W-1, H-3), "")
	w.stat.GrowMode = consts.GfGrowLoY | consts.GfGrowHiY | consts.GfGrowHiX
	w.Insert(w.stat)

	// Grid in the middle. Default GrowHiX|GrowHiY from grid.New —
	// its bottom edge will follow the status row anchor.
	scroll := views.NewScrollBar(geom.NewRect(W-2, 2, W-1, H-4))
	w.Insert(scroll)
	g := grid.New(geom.NewRect(1, 2, W-2, H-4), gridDemoColumns(), nil, scroll)
	g.ShowFilter = true
	g.ShowRowMarker = true
	g.FixedCols = 1
	g.Mode = grid.SelectRange
	g.SetRows(generateGridDemoRows(500))
	w.Insert(g)
	w.grid = g

	// Toolbar buttons across the bottom row. Each button is a fixed
	// width; lay them out left-to-right and stop when we run out of
	// horizontal room. Buttons anchor to the bottom edge.
	w.layoutToolbar(H - 3)

	w.refreshStatus()
	return w
}

// layoutToolbar drops buttons onto row y of the window interior.
// Keeping this in a method lets us re-call it when the window resizes
// (left as a TODO — the simple approach is good enough for the demo).
func (w *gridDemoWindow) layoutToolbar(y int) {
	x := 1
	add := func(label string, cmd uint16) {
		btnW := len(stripAmp(label)) + 4
		btn := dialogs.NewButton(geom.NewRect(x, y, x+btnW, y+2),
			label, cmd, dialogs.BfNormal)
		// Anchor each button to the bottom of the window so it stays
		// glued to the toolbar row when the user resizes the window.
		// Horizontal growth is left at zero (X stays put), so on a
		// horizontal shrink some buttons will end up past the right
		// frame — our ChangeBounds override hides those.
		btn.GrowMode = consts.GfGrowLoY | consts.GfGrowHiY
		w.Insert(btn)
		w.buttons = append(w.buttons, btn)
		if x+btnW > w.Size.X-1 {
			btn.Hide()
		}
		x += btnW + 1
	}
	add("~L~oad", cmGridDemoLoad)
	add("~S~ave", cmGridDemoSave)
	add("~A~dd", cmGridDemoAddRow)
	add("~D~el", cmGridDemoDelRow)
	add("Cop~y~", cmGridDemoCopy)
	add("Clear ~F~ilt", cmGridDemoClearFilters)
	add("~T~oggle Filt", cmGridDemoToggleFilter)
	add("~R~eset Sort", cmGridDemoResetSort)
	add("S~t~ats", cmGridDemoStats)
}

// stripAmp removes hotkey markers (`~X~`) from a label so we can
// measure its rendered width.
func stripAmp(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '~' {
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

// Draw refits the toolbar buttons before delegating to the embedded
// Window's Draw. The resize loop calls ChangeBounds non-virtually on
// the embedded Window (skipping any override on the wrapper), so the
// reliable hook is Draw — which the Group walks via the View interface
// and therefore dispatches to our override.
func (w *gridDemoWindow) Draw() {
	w.refitButtons()
	w.Window.Draw()
}

// refitButtons toggles button visibility so each button is visible iff
// its full extent fits inside the window interior. The position math
// stays driven by GrowMode — we only flip Show/Hide.
func (w *gridDemoWindow) refitButtons() {
	for _, b := range w.buttons {
		bv := b.BaseView()
		right := bv.Origin.X + bv.Size.X
		bottom := bv.Origin.Y + bv.Size.Y
		fits := right <= w.Size.X-1 && bottom <= w.Size.Y-1 && bv.Origin.Y >= 1
		visible := bv.GetState(consts.SfVisible)
		switch {
		case fits && !visible:
			b.Show()
		case !fits && visible:
			b.Hide()
		}
	}
}

// HandleEvent routes toolbar commands to grid operations. Anything we
// don't handle bubbles up to the embedded Window's default handler,
// preserving move / close / focus behavior.
func (w *gridDemoWindow) HandleEvent(ev *drivers.Event) {
	w.Window.HandleEvent(ev)
	if ev.What != consts.EvCommand {
		// Status changes on most input — cheap to refresh unconditionally.
		w.refreshStatus()
		return
	}
	switch ev.Command {
	case cmGridDemoLoad:
		w.doLoad()
	case cmGridDemoSave:
		w.doSave()
	case cmGridDemoClearFilters:
		w.grid.ClearFilters()
	case cmGridDemoAddRow:
		w.doAddRow()
	case cmGridDemoDelRow:
		w.doDelete()
	case cmGridDemoCopy:
		w.grid.CopySelection()
	case cmGridDemoToggleFilter:
		w.grid.ShowFilter = !w.grid.ShowFilter
	case cmGridDemoResetSort:
		w.grid.Sort(-1, grid.SortNone)
	case cmGridDemoStats:
		w.showStats()
	default:
		w.refreshStatus()
		return
	}
	w.ClearEvent(ev)
	w.refreshStatus()
	views.MarkDirty()
}

// refreshStatus updates the status text with row counts + selection /
// sort state. Cheap enough to call after every event.
func (w *gridDemoWindow) refreshStatus() {
	if w.stat == nil || w.grid == nil {
		return
	}
	visible := w.grid.RowCount()
	total := w.grid.RawRowCount()
	sortHint := "unsorted"
	if w.grid.SortCol >= 0 && w.grid.SortCol < len(w.grid.Columns) {
		dir := "asc"
		if w.grid.SortDir == grid.SortDesc {
			dir = "desc"
		}
		sortHint = fmt.Sprintf("sorted by %s %s",
			w.grid.Columns[w.grid.SortCol].Title, dir)
	}
	w.stat.Text = fmt.Sprintf(
		"Rows: %d shown / %d total  ·  Focus: (%d, %d)  ·  %s",
		visible, total, w.grid.Focus.Row, w.grid.Focus.Col, sortHint)
	views.MarkDirty()
}

func (w *gridDemoWindow) doSave() {
	path, ok := stddlg.ShowModern(&w.app.Desktop.Group, stddlg.ModeSave,
		"Save Grid as CSV", "", "*.csv")
	if !ok {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		msgbox.Showf(&w.app.Desktop.Group, msgbox.Error,
			"Couldn't create %s:\n%s", []any{path, err.Error()}, msgbox.OKOnly)
		return
	}
	defer f.Close()
	if err := w.grid.SaveCSV(f, grid.CSVOptions{IncludeHeader: true}); err != nil {
		msgbox.Showf(&w.app.Desktop.Group, msgbox.Error,
			"Save failed: %s", []any{err.Error()}, msgbox.OKOnly)
	}
}

func (w *gridDemoWindow) doLoad() {
	path, ok := stddlg.ShowModern(&w.app.Desktop.Group, stddlg.ModeOpen,
		"Load Grid from CSV", "", "*.csv")
	if !ok {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		msgbox.Showf(&w.app.Desktop.Group, msgbox.Error,
			"Couldn't open %s:\n%s", []any{path, err.Error()}, msgbox.OKOnly)
		return
	}
	defer f.Close()
	if err := w.grid.LoadCSV(f, grid.CSVOptions{
		AutoDetectDelimiter: true,
		IncludeHeader:       true,
	}); err != nil {
		msgbox.Showf(&w.app.Desktop.Group, msgbox.Error,
			"Load failed: %s", []any{err.Error()}, msgbox.OKOnly)
	}
}

// doAddRow appends a blank row and moves the focus to it. If a sort
// is active the new row may not land at the visible bottom on its own,
// and an active filter could even hide it entirely — handle both so
// the user actually SEES the row they just asked for.
func (w *gridDemoWindow) doAddRow() {
	w.grid.AddRow([]string{"(new)", "0", "$0.00", "", "Misc", "No"})
	if w.grid.HasActiveFilters() {
		w.grid.ClearFilters()
	}
	if w.grid.SortCol >= 0 {
		w.grid.Sort(-1, grid.SortNone)
	}
	last := w.grid.RowCount() - 1
	if last >= 0 {
		w.grid.MoveTo(0, last)
	}
}

// doDelete removes the focused raw row. Grid's Focus.Row is a visible-
// row index (post-filter); the rawRowAt helper resolves it.
func (w *gridDemoWindow) doDelete() {
	if w.grid.RowCount() == 0 {
		return
	}
	if msgbox.Show(&w.app.Desktop.Group, msgbox.Question,
		"Delete the focused row?", msgbox.YesNo) != consts.CmYes {
		return
	}
	raw := w.grid.RawRowAt(w.grid.Focus.Row)
	if raw < 0 {
		return
	}
	w.grid.RemoveRow(raw)
}

func (w *gridDemoWindow) showStats() {
	msgbox.Showf(&w.app.Desktop.Group, msgbox.Info,
		"Columns: %d\nVisible rows: %d\nTotal rows: %d\nSort column: %d\nFiltered: %v",
		[]any{
			w.grid.ColCount(),
			w.grid.RowCount(),
			w.grid.RawRowCount(),
			w.grid.SortCol,
			w.grid.HasActiveFilters(),
		}, msgbox.OKOnly)
}

// gridDemoColumns returns the column layout used by the demo grid.
// Validators are wired so cells reject obviously-invalid input on
// commit (e.g. typing letters into Qty). Frozen first column is the
// caller's responsibility — set FixedCols on the grid.
func gridDemoColumns() []grid.Column {
	digits := validators.NewFilterValidator("0123456789")
	priceChars := validators.NewFilterValidator("0123456789.$ ")
	return []grid.Column{
		{Title: "Name", Width: 20, Align: grid.AlignLeft, Sortable: true},
		{Title: "Qty", Width: 7, Align: grid.AlignRight, Sortable: true, Validator: digits},
		{Title: "Price", Width: 10, Align: grid.AlignRight, Sortable: true, Validator: priceChars},
		{Title: "Notes", Width: 24, Align: grid.AlignLeft, Sortable: true},
		{Title: "Category", Width: 12, Align: grid.AlignLeft, Sortable: true},
		{Title: "In Stock", Width: 10, Align: grid.AlignCenter, Sortable: true},
	}
}

// generateGridDemoRows produces n synthetic inventory rows. Seeded
// deterministically so the demo looks the same every launch — handy
// when comparing screenshots / behavior across builds.
func generateGridDemoRows(n int) [][]string {
	r := rand.New(rand.NewSource(42))
	names := []string{
		"Apples", "Bananas", "Cherries", "Dates", "Elderberry", "Figs",
		"Grapes", "Honeydew", "Iceberg", "Jalapeño", "Kiwi", "Lemons",
		"Mango", "Nectarines", "Oranges", "Peaches", "Quince", "Raspberry",
		"Strawberry", "Tangerine", "Ugli", "Vanilla", "Watermelon",
		"Ximenia", "Yam", "Zucchini",
	}
	notes := []string{
		"Local farm", "Imported", "Seasonal", "Organic", "On sale",
		"Limited stock", "Pre-order", "Premium grade", "", "",
	}
	categories := []string{"Fruit", "Vegetable", "Spice", "Herb", "Dairy", "Bakery"}
	rows := make([][]string, n)
	for i := 0; i < n; i++ {
		name := names[r.Intn(len(names))] + " #" + strconv.Itoa(i+1)
		qty := strconv.Itoa(r.Intn(200) + 1)
		price := fmt.Sprintf("$%d.%02d", r.Intn(20), r.Intn(100))
		note := notes[r.Intn(len(notes))]
		cat := categories[r.Intn(len(categories))]
		stock := "Yes"
		if r.Intn(5) == 0 {
			stock = "No"
		}
		rows[i] = []string{name, qty, price, note, cat, stock}
	}
	return rows
}
