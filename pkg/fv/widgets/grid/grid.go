// Package grid provides StringGrid — a terminal-mode data grid with
// rows, named columns, in-place cell editing, mouse navigation, per-
// column alignment and validation, multi-cell selection, click-to-sort
// headers, an optional filter row, copy-to-clipboard, column resize
// and reorder via mouse drag, frozen leading columns, CSV import /
// export, and a virtual-data-source mode for huge datasets.
//
// Ported from Grid.pas (TStringGrid). The Pascal version's per-cell
// undo log isn't here yet (that's editor scope); everything else from
// the FVTest grid-test page is.
package grid

import (
	"sort"

	"github.com/oldwired/fv-go/pkg/fv/clipboard"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/validators"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Alignment picks how a cell's text aligns within its column.
type Alignment int

const (
	AlignLeft Alignment = iota
	AlignCenter
	AlignRight
)

// Column describes one column. Validator (optional) runs on commit of
// an edit; rejection bounces the user back into edit mode. Sortable
// can be cleared to lock a column out of header-click sorting.
type Column struct {
	Title     string
	Width     int
	Align     Alignment
	ReadOnly  bool
	Sortable  bool
	Validator validators.Validator
}

// Cell coords. (0, 0) is the top-left data cell — the header row /
// column don't have addresses in this scheme.
type Cell struct{ Col, Row int }

// SelectionMode controls how selections expand. Cell = a single cell
// (anchor == focus). Range = a rectangular block. Row = full row(s).
type SelectionMode int

const (
	SelectCell SelectionMode = iota
	SelectRange
	SelectRow
)

// SortDirection — Asc, Desc, or None (unsorted).
type SortDirection int

const (
	SortNone SortDirection = iota
	SortAsc
	SortDesc
)

// dragKind tracks what the user is doing with a held mouse button.
type dragKind int

const (
	dragNone dragKind = iota
	dragSelect
	dragResize
	dragReorder
)

// StringGrid is a 2D grid of text cells.
type StringGrid struct {
	views.Base

	Columns []Column
	rows    [][]string // rows[row][col] — bypassed in virtual mode

	// Selection. Focus is the "active" corner the cursor moves; Anchor
	// is the opposite end of the range. Cell vs Range vs Row is the
	// Mode toggle.
	Anchor, Focus Cell
	Mode          SelectionMode

	Top       int // first visible row (in visible/filtered index space)
	LeftCol   int // first visible column (in column-order space)
	HasHeader bool

	// FixedCols pins the first N columns so they don't scroll
	// horizontally. The pin is visual only — Anchor/Focus still
	// address pinned columns the same as any others.
	FixedCols int

	// ShowFilter toggles the per-column filter row below the header.
	// Filters[i] is the case-insensitive substring required in column i
	// (empty = no filter).
	ShowFilter bool
	Filters    []string

	// ShowRowMarker paints a "►" glyph over the leading padding cell of
	// the focused row, so the active row stays visible even when focus
	// moves elsewhere on the screen. Purely cosmetic — doesn't change
	// column geometry or hit-testing.
	ShowRowMarker bool

	// Sort state. SortCol < 0 means "unsorted" (use natural row order).
	SortCol int
	SortDir SortDirection

	// Virtual mode. When OnGetCell is non-nil, rows is ignored and the
	// callback supplies cell data on demand. VirtualRowCount is the
	// reported total row count in this mode.
	OnGetCell       func(row, col int) string
	OnSetCell       func(row, col int, value string)
	VirtualRowCount int

	// Derived render order. visibleRows is the slice of row indices
	// passing filters, in sorted order. Recomputed on rebuild().
	visibleRows []int
	dirty       bool // visibleRows + sort state need recompute

	// Edit mode.
	editing    bool
	editBuf    []rune
	editPos    int
	editStatus string // brief validation message
	editFocus  bool   // focus row of the filter input

	// Drag-in-flight state.
	drag      dragKind
	dragCol   int // column being resized or reordered
	dragFromX int // local X where the drag began

	HScroll *views.ScrollBar
	VScroll *views.ScrollBar
}

// New constructs an empty grid with the given columns.
func New(bounds geom.Rect, cols []Column, h, v *views.ScrollBar) *StringGrid {
	// Default every column to sortable; callers can override per-column.
	for i := range cols {
		if !cols[i].Sortable && cols[i].Title != "" {
			cols[i].Sortable = true
		}
	}
	g := &StringGrid{
		Base:      views.NewBase(bounds),
		Columns:   cols,
		HasHeader: true,
		Mode:      SelectCell,
		SortCol:   -1,
		Filters:   make([]string, len(cols)),
		HScroll:   h,
		VScroll:   v,
		dirty:     true,
	}
	g.SetSelf(g)
	g.Options |= consts.OfSelectable | consts.OfFirstClick
	g.State |= consts.SfCursorVis
	g.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	return g
}

// GetTypeID for serial registry.
func (g *StringGrid) GetTypeID() string { return "stringgrid" }

// SetRows replaces all cell data.
func (g *StringGrid) SetRows(rows [][]string) {
	g.rows = rows
	g.markDirty()
	g.clampSelection()
	g.refreshScroll()
}

// AddRow appends a row.
func (g *StringGrid) AddRow(values []string) {
	if len(values) > len(g.Columns) {
		values = values[:len(g.Columns)]
	}
	for len(values) < len(g.Columns) {
		values = append(values, "")
	}
	g.rows = append(g.rows, values)
	g.markDirty()
	g.refreshScroll()
}

// rawCell returns the raw stored / source value for (row, col). Used
// internally for sorting / filtering / rendering. Virtual-mode reads
// through OnGetCell; non-virtual reads from the underlying slice.
func (g *StringGrid) rawCell(row, col int) string {
	if g.OnGetCell != nil {
		if row < 0 || row >= g.VirtualRowCount {
			return ""
		}
		if col < 0 || col >= len(g.Columns) {
			return ""
		}
		return g.OnGetCell(row, col)
	}
	if row < 0 || row >= len(g.rows) {
		return ""
	}
	if col < 0 || col >= len(g.rows[row]) {
		return ""
	}
	return g.rows[row][col]
}

// Cell returns the displayed cell at (visible-row, col). Use this for
// reading what the user sees, not for editing internal state.
func (g *StringGrid) Cell(row, col int) string {
	g.ensureVisible()
	if row < 0 || row >= len(g.visibleRows) {
		return ""
	}
	return g.rawCell(g.visibleRows[row], col)
}

// SetCell updates the (row, col) value. In non-virtual mode it mutates
// the backing slice (and expands it if needed). In virtual mode it
// calls OnSetCell.
func (g *StringGrid) SetCell(row, col int, value string) {
	g.ensureVisible()
	if g.OnSetCell != nil {
		if row >= 0 && row < len(g.visibleRows) {
			g.OnSetCell(g.visibleRows[row], col, value)
		}
		g.markDirty()
		return
	}
	if row < 0 || row >= len(g.visibleRows) {
		// Out of visible range — extend in raw space.
		for row >= len(g.rows) {
			g.rows = append(g.rows, make([]string, len(g.Columns)))
		}
		g.rows[row][col] = value
		g.markDirty()
		return
	}
	rawRow := g.visibleRows[row]
	for rawRow >= len(g.rows) {
		g.rows = append(g.rows, make([]string, len(g.Columns)))
	}
	for col >= len(g.rows[rawRow]) {
		g.rows[rawRow] = append(g.rows[rawRow], "")
	}
	g.rows[rawRow][col] = value
	g.markDirty()
}

// RowCount returns the number of VISIBLE rows (after filtering /
// sorting). For the underlying raw count, use RawRowCount.
func (g *StringGrid) RowCount() int {
	g.ensureVisible()
	return len(g.visibleRows)
}

// RawRowCount returns the unfiltered row count (virtual or in-memory).
func (g *StringGrid) RawRowCount() int {
	if g.OnGetCell != nil {
		return g.VirtualRowCount
	}
	return len(g.rows)
}

// ColCount returns the column count.
func (g *StringGrid) ColCount() int { return len(g.Columns) }

// RawRowAt maps a visible row index to its underlying raw row.
// Returns -1 if visRow is out of range. Use this when you need to
// edit the underlying storage (e.g. RemoveRow) and only have a
// visible-row index from Focus / a mouse hit.
func (g *StringGrid) RawRowAt(visRow int) int {
	g.ensureVisible()
	if visRow < 0 || visRow >= len(g.visibleRows) {
		return -1
	}
	return g.visibleRows[visRow]
}

// RemoveRow drops the raw row at the given index from the underlying
// store. No-op in virtual mode (the caller's data source owns the
// rows there). Selection is clamped after the deletion.
func (g *StringGrid) RemoveRow(raw int) {
	if g.OnGetCell != nil {
		return
	}
	if raw < 0 || raw >= len(g.rows) {
		return
	}
	g.rows = append(g.rows[:raw], g.rows[raw+1:]...)
	g.markDirty()
	g.clampSelection()
	g.refreshScroll()
}

// HasActiveFilters returns true if any column has a non-empty filter.
func (g *StringGrid) HasActiveFilters() bool {
	for _, f := range g.Filters {
		if f != "" {
			return true
		}
	}
	return false
}

// markDirty schedules a rebuild of the visibleRows index. Cheap; the
// rebuild itself runs lazily via ensureVisible.
func (g *StringGrid) markDirty() { g.dirty = true }

// ensureVisible rebuilds visibleRows (filter + sort) if dirty.
func (g *StringGrid) ensureVisible() {
	if !g.dirty {
		return
	}
	total := g.RawRowCount()
	g.visibleRows = g.visibleRows[:0]
	for r := 0; r < total; r++ {
		if g.rowPassesFilters(r) {
			g.visibleRows = append(g.visibleRows, r)
		}
	}
	if g.SortCol >= 0 && g.SortCol < len(g.Columns) {
		col := g.SortCol
		dir := g.SortDir
		sort.SliceStable(g.visibleRows, func(i, j int) bool {
			a := g.rawCell(g.visibleRows[i], col)
			b := g.rawCell(g.visibleRows[j], col)
			if dir == SortDesc {
				return a > b
			}
			return a < b
		})
	}
	g.dirty = false
}

// rowPassesFilters checks every active filter against a raw row.
func (g *StringGrid) rowPassesFilters(rawRow int) bool {
	for c, f := range g.Filters {
		if f == "" {
			continue
		}
		v := g.rawCell(rawRow, c)
		if !containsFold(v, f) {
			return false
		}
	}
	return true
}

// Sort sets the active sort column and direction. Call with
// (col, SortNone) to clear sorting.
func (g *StringGrid) Sort(col int, dir SortDirection) {
	if col < 0 || col >= len(g.Columns) {
		dir = SortNone
	}
	if dir == SortNone {
		g.SortCol = -1
	} else {
		g.SortCol = col
		g.SortDir = dir
	}
	g.markDirty()
}

// CycleSort advances col through asc → desc → unsorted. Used by
// header click handlers. Only fires when the column is Sortable.
func (g *StringGrid) CycleSort(col int) {
	if col < 0 || col >= len(g.Columns) || !g.Columns[col].Sortable {
		return
	}
	if g.SortCol != col {
		g.SortCol = col
		g.SortDir = SortAsc
		g.markDirty()
		return
	}
	switch g.SortDir {
	case SortAsc:
		g.SortDir = SortDesc
	case SortDesc:
		g.SortCol = -1
	default:
		g.SortDir = SortAsc
	}
	g.markDirty()
}

// SetFilter sets the filter substring for column col. Empty value
// clears it.
func (g *StringGrid) SetFilter(col int, text string) {
	if col < 0 || col >= len(g.Columns) {
		return
	}
	for len(g.Filters) <= col {
		g.Filters = append(g.Filters, "")
	}
	g.Filters[col] = text
	g.markDirty()
}

// ClearFilters wipes all filter strings.
func (g *StringGrid) ClearFilters() {
	for i := range g.Filters {
		g.Filters[i] = ""
	}
	g.markDirty()
}

// containsFold is case-insensitive ASCII substring check. We don't
// pull in strings.EqualFold-style locale rules; for the column filter
// this is exactly what users expect.
func containsFold(s, sub string) bool {
	if sub == "" {
		return true
	}
	ls := lowerASCII(s)
	lt := lowerASCII(sub)
	return contains(ls, lt)
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func contains(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// columnX returns the screen X position of column c relative to the
// grid's left edge. Pinned columns are always at their natural X;
// scrollable columns are offset by LeftCol.
func (g *StringGrid) columnX(c int) int {
	if c < 0 {
		return 0
	}
	if c < g.FixedCols {
		// Pinned: simple prefix sum.
		x := 0
		for i := 0; i < c && i < len(g.Columns); i++ {
			x += g.Columns[i].Width
		}
		return x
	}
	// Scrollable: start at the pinned-cols end, then walk from LeftCol.
	x := 0
	for i := 0; i < g.FixedCols && i < len(g.Columns); i++ {
		x += g.Columns[i].Width
	}
	start := g.LeftCol
	if start < g.FixedCols {
		start = g.FixedCols
	}
	for i := start; i < c && i < len(g.Columns); i++ {
		x += g.Columns[i].Width
	}
	return x
}

// columnAt returns the column index whose cells cover screen-relative
// x, or -1 if none. Pinned columns get priority hits.
func (g *StringGrid) columnAt(x int) int {
	cur := 0
	for i := 0; i < g.FixedCols && i < len(g.Columns); i++ {
		w := g.Columns[i].Width
		if x >= cur && x < cur+w {
			return i
		}
		cur += w
	}
	start := g.LeftCol
	if start < g.FixedCols {
		start = g.FixedCols
	}
	for i := start; i < len(g.Columns); i++ {
		w := g.Columns[i].Width
		if x >= cur && x < cur+w {
			return i
		}
		cur += w
	}
	return -1
}

// headerHeight returns the number of header rows currently shown:
// title row (HasHeader) plus the underline row (also part of the
// header when HasHeader is set) plus filter row (ShowFilter).
func (g *StringGrid) headerHeight() int {
	h := 0
	if g.HasHeader {
		// Title row + decorative underline row.
		h += 2
	}
	if g.ShowFilter {
		h++
	}
	return h
}

// dataRows returns how many rows the body can show.
func (g *StringGrid) dataRows() int {
	r := g.Size.Y - g.headerHeight()
	if r < 0 {
		return 0
	}
	return r
}

// MoveTo sets the active cell + collapses selection to it.
func (g *StringGrid) MoveTo(col, row int) {
	g.moveTo(col, row, false)
}

// ExtendTo moves the focus end of the selection, keeping the anchor
// fixed. Shift+arrow / Shift+click are the entry points.
func (g *StringGrid) ExtendTo(col, row int) {
	g.moveTo(col, row, true)
}

func (g *StringGrid) moveTo(col, row int, extend bool) {
	if g.editing {
		g.commitEdit()
	}
	if row < 0 {
		row = 0
	}
	if vc := g.RowCount(); row >= vc {
		row = vc - 1
	}
	if row < 0 {
		row = 0
	}
	if col < 0 {
		col = 0
	}
	if col >= len(g.Columns) {
		col = len(g.Columns) - 1
	}
	if col < 0 {
		col = 0
	}
	g.Focus = Cell{Col: col, Row: row}
	if !extend {
		g.Anchor = g.Focus
	}
	if extend && g.Mode == SelectCell {
		g.Mode = SelectRange
	}
	g.adjustScroll()
}

// clampSelection brings Anchor/Focus into the visible row range.
func (g *StringGrid) clampSelection() {
	g.ensureVisible()
	n := len(g.visibleRows)
	if g.Focus.Row >= n {
		g.Focus.Row = n - 1
	}
	if g.Focus.Row < 0 {
		g.Focus.Row = 0
	}
	if g.Anchor.Row >= n {
		g.Anchor.Row = n - 1
	}
	if g.Anchor.Row < 0 {
		g.Anchor.Row = 0
	}
}

// adjustScroll keeps the focused cell visible.
func (g *StringGrid) adjustScroll() {
	rows := g.dataRows()
	if g.Focus.Row < g.Top {
		g.Top = g.Focus.Row
	} else if g.Focus.Row >= g.Top+rows {
		g.Top = g.Focus.Row - rows + 1
	}
	if g.Focus.Col < g.LeftCol && g.Focus.Col >= g.FixedCols {
		g.LeftCol = g.Focus.Col
	}
	// Right-edge horizontal scroll.
	for g.LeftCol < g.Focus.Col && g.columnX(g.Focus.Col)+columnWidth(g.Columns, g.Focus.Col) > g.Size.X {
		g.LeftCol++
	}
	g.refreshScroll()
}

func columnWidth(cols []Column, i int) int {
	if i < 0 || i >= len(cols) {
		return 0
	}
	return cols[i].Width
}

// refreshScroll updates linked scroll bars.
func (g *StringGrid) refreshScroll() {
	if g.VScroll != nil {
		g.VScroll.SetRange(0, g.RowCount())
		g.VScroll.SetValue(g.Top)
	}
	if g.HScroll != nil {
		g.HScroll.SetRange(0, len(g.Columns))
		g.HScroll.SetValue(g.LeftCol)
	}
}

// selectionRect returns the normalized (top-left, bottom-right) of the
// current selection. Always valid even for a single-cell selection.
func (g *StringGrid) selectionRect() (Cell, Cell) {
	a, b := g.Anchor, g.Focus
	if g.Mode == SelectCell {
		return b, b
	}
	if a.Row > b.Row {
		a.Row, b.Row = b.Row, a.Row
	}
	if g.Mode == SelectRow {
		a.Col = 0
		b.Col = len(g.Columns) - 1
	} else {
		if a.Col > b.Col {
			a.Col, b.Col = b.Col, a.Col
		}
	}
	return a, b
}

// inSelection reports whether (row, col) is selected.
func (g *StringGrid) inSelection(row, col int) bool {
	a, b := g.selectionRect()
	if row < a.Row || row > b.Row {
		return false
	}
	if col < a.Col || col > b.Col {
		return false
	}
	return true
}

// CopySelection copies the selected cells to the clipboard as TSV.
// Rows are separated by '\n', cells by '\t'. Multi-line cell contents
// have their tabs/newlines stripped so the output stays one cell per
// field — TV cells are single-line anyway.
func (g *StringGrid) CopySelection() {
	a, b := g.selectionRect()
	if g.RowCount() == 0 {
		return
	}
	var out []byte
	for r := a.Row; r <= b.Row; r++ {
		for c := a.Col; c <= b.Col; c++ {
			if c > a.Col {
				out = append(out, '\t')
			}
			v := g.Cell(r, c)
			for _, ch := range v {
				if ch == '\t' || ch == '\n' || ch == '\r' {
					out = append(out, ' ')
				} else {
					out = append(out, string(ch)...)
				}
			}
		}
		out = append(out, '\n')
	}
	clipboard.SetText(string(out))
}

// Draw paints header (if any), optional filter row, then visible rows.
func (g *StringGrid) Draw() {
	g.ensureVisible()
	// Palette. Header is white on dark blue (calmer than the previous
	// red bar); rows alternate fg brightness for subtle zebra striping;
	// column separators are dark-gray glyphs that visually anchor the
	// grid without competing with the data.
	headerColor := types.MakeAttr(0x0F, 0x01)
	headerSepColor := types.MakeAttr(0x08, 0x01)
	rowEven := types.MakeAttr(0x07, 0x01)
	rowOdd := types.MakeAttr(0x0F, 0x01)
	selColor := types.MakeAttr(0x00, 0x07)
	focusColor := types.MakeAttr(0x0E, 0x06)
	editColor := types.MakeAttr(0x0F, 0x02)
	filterColor := types.MakeAttr(0x00, 0x07)
	dividerAttrFor := func(bg byte) uint16 {
		// Dark-gray glyph on whatever the row's bg is.
		return types.MakeAttr(0x08, bg)
	}

	rowOff := 0
	if g.HasHeader {
		buf := screen.MakeDrawBuffer(g.Size.X)
		for x := 0; x < g.Size.X; x++ {
			screen.DrawCell(buf, x, " ", headerColor)
		}
		g.drawCellsRow(buf, headerColor, dividerAttrFor(0x01), func(c int) string {
			suffix := ""
			if g.SortCol == c {
				if g.SortDir == SortAsc {
					suffix = " ▲"
				} else if g.SortDir == SortDesc {
					suffix = " ▼"
				}
			}
			budget := g.Columns[c].Width - 2 - utf8.StringDisplayWidth(suffix)
			if budget < 0 {
				budget = 0
			}
			label := truncate(g.Columns[c].Title, budget) + suffix
			return alignText(label, g.Columns[c].Width-1, g.Columns[c].Align)
		})
		g.WriteLine(0, 0, g.Size.X, 1, buf)
		rowOff++
		// Subtle underline row separating header from body.
		sep := screen.MakeDrawBuffer(g.Size.X)
		for x := 0; x < g.Size.X; x++ {
			screen.DrawCell(sep, x, "─", headerSepColor)
		}
		g.drawCellSeparators(sep, headerSepColor, "┬")
		g.WriteLine(0, rowOff, g.Size.X, 1, sep)
		rowOff++
	}
	filterRowY := -1
	if g.ShowFilter {
		buf := screen.MakeDrawBuffer(g.Size.X)
		for x := 0; x < g.Size.X; x++ {
			screen.DrawCell(buf, x, " ", filterColor)
		}
		g.drawCellsRow(buf, filterColor, dividerAttrFor(0x07), func(c int) string {
			text := ""
			if c < len(g.Filters) {
				text = g.Filters[c]
			}
			// Show the live edit buffer while the user is typing into
			// this column's filter — otherwise typed chars would stay
			// invisible until they hit Enter and we'd look broken.
			if g.editing && g.editFocus && c == g.Focus.Col {
				text = string(g.editBuf)
			}
			return alignText(truncate(text, g.Columns[c].Width-2), g.Columns[c].Width-1, AlignLeft)
		})
		g.WriteLine(0, rowOff, g.Size.X, 1, buf)
		filterRowY = rowOff
		rowOff++
	}
	for r := 0; r < g.dataRows(); r++ {
		visRow := g.Top + r
		rowAttr := rowEven
		if visRow%2 == 1 {
			rowAttr = rowOdd
		}
		buf := screen.MakeDrawBuffer(g.Size.X)
		for x := 0; x < g.Size.X; x++ {
			screen.DrawCell(buf, x, " ", rowAttr)
		}
		if visRow >= g.RowCount() {
			g.WriteLine(0, rowOff+r, g.Size.X, 1, buf)
			continue
		}
		g.drawCellsRowAt(buf, visRow, rowAttr, selColor, focusColor, editColor, dividerAttrFor(0x01))
		// Row marker overlay: bright glyph at x=0 on the focused row.
		// Paint over the leading padding cell using the row's own
		// background so the indicator blends with selection / focus
		// highlights instead of clashing with them.
		if g.ShowRowMarker && visRow == g.Focus.Row {
			bg := types.BG(buf[0].Attr)
			buf[0] = types.DrawCell{Ch: "►", Attr: types.MakeAttr(0x0E, bg)}
		}
		g.WriteLine(0, rowOff+r, g.Size.X, 1, buf)
	}
	// Empty-state placeholder when no data and no filter is hiding rows.
	if g.RowCount() == 0 && g.RawRowCount() == 0 && g.dataRows() > 0 {
		buf := screen.MakeDrawBuffer(g.Size.X)
		for x := 0; x < g.Size.X; x++ {
			screen.DrawCell(buf, x, " ", types.MakeAttr(0x08, 0x01))
		}
		msg := "(no rows)"
		mw := utf8.StringDisplayWidth(msg)
		screen.DrawStr(buf, (g.Size.X-mw)/2, msg, types.MakeAttr(0x08, 0x01))
		g.WriteLine(0, rowOff+g.dataRows()/2, g.Size.X, 1, buf)
	}
	// Caret in edit mode. The filter-edit cursor lives in the filter
	// row regardless of which data row Focus.Row points at.
	if g.editing {
		x := g.columnX(g.Focus.Col) + g.editPos + 1 // +1 for leading divider pad
		var y int
		if g.editFocus && filterRowY >= 0 {
			y = filterRowY
		} else {
			y = g.Focus.Row - g.Top + rowOff
		}
		g.Cursor = geom.Point{X: x, Y: y}
	}
}

// drawCellSeparators overlays the "┬" / "┼" / "┴" glyphs onto a
// horizontal-rule row so column boundaries line up visually with the
// data rows' "│" dividers. Walks the same column geometry as the
// data renderer.
func (g *StringGrid) drawCellSeparators(buf screen.DrawBuffer, attr uint16, glyph string) {
	x := 0
	for c := 0; c < g.FixedCols && c < len(g.Columns); c++ {
		w := g.Columns[c].Width
		if x+w-1 < g.Size.X {
			screen.DrawCell(buf, x+w-1, glyph, attr)
		}
		x += w
	}
	start := g.LeftCol
	if start < g.FixedCols {
		start = g.FixedCols
	}
	for c := start; c+1 < len(g.Columns); c++ {
		w := g.Columns[c].Width
		if x+w-1 < g.Size.X {
			screen.DrawCell(buf, x+w-1, glyph, attr)
		}
		x += w
		if x >= g.Size.X {
			return
		}
	}
}

// drawCellsRow paints one non-data row (header or filter), letting the
// caller supply each cell's text. Same column geometry as data rows
// so columns line up. divAttr is the attribute used to draw the "│"
// dividers between columns.
func (g *StringGrid) drawCellsRow(buf screen.DrawBuffer, attr uint16, divAttr uint16, textOf func(c int) string) {
	paint := func(c int, x int) int {
		col := g.Columns[c]
		s := textOf(c)
		// Leave column-edge cell for divider.
		text := truncate(s, col.Width-1)
		for i, ch := range text {
			if x+i < g.Size.X {
				buf[x+i] = types.DrawCell{Ch: string(ch), Attr: attr}
			}
		}
		// Divider in the rightmost cell of the column (except for the
		// final visible column, which doesn't need one).
		if x+col.Width-1 < g.Size.X {
			buf[x+col.Width-1] = types.DrawCell{Ch: "│", Attr: divAttr}
		}
		return x + col.Width
	}
	x := 0
	for c := 0; c < g.FixedCols && c < len(g.Columns); c++ {
		x = paint(c, x)
		if x >= g.Size.X {
			return
		}
	}
	start := g.LeftCol
	if start < g.FixedCols {
		start = g.FixedCols
	}
	for c := start; c < len(g.Columns); c++ {
		x = paint(c, x)
		if x >= g.Size.X {
			return
		}
	}
}

// drawCellsRowAt paints a single data row, picking per-cell attr
// based on selection / focus / edit state. divAttr is the divider
// color when the cell isn't highlighted; selected/focused cells skip
// the divider so the highlight reads as one continuous block.
func (g *StringGrid) drawCellsRowAt(buf screen.DrawBuffer, visRow int, cellColor, selColor, focusColor, editColor, divAttr uint16) {
	paintCol := func(c int, x int) {
		col := g.Columns[c]
		text := g.Cell(visRow, c)
		attr := cellColor
		highlighted := false
		if g.inSelection(visRow, c) {
			attr = selColor
			highlighted = true
		}
		if visRow == g.Focus.Row && c == g.Focus.Col {
			attr = focusColor
			highlighted = true
			if g.editing {
				attr = editColor
				text = string(g.editBuf)
			}
		}
		// Fill column with the chosen attr.
		for i := 0; i < col.Width && x+i < g.Size.X; i++ {
			buf[x+i] = types.DrawCell{Ch: " ", Attr: attr}
		}
		// Reserve 1 cell on each side for padding and the divider.
		innerW := col.Width - 2
		if innerW < 0 {
			innerW = 0
		}
		padded := alignText(truncateEllipsis(text, innerW), innerW, col.Align)
		for i, ch := range padded {
			if x+1+i < g.Size.X {
				buf[x+1+i] = types.DrawCell{Ch: string(ch), Attr: attr}
			}
		}
		if !highlighted && x+col.Width-1 < g.Size.X {
			buf[x+col.Width-1] = types.DrawCell{Ch: "│", Attr: divAttr}
		}
	}
	x := 0
	for c := 0; c < g.FixedCols && c < len(g.Columns); c++ {
		paintCol(c, x)
		x += g.Columns[c].Width
		if x >= g.Size.X {
			return
		}
	}
	start := g.LeftCol
	if start < g.FixedCols {
		start = g.FixedCols
	}
	for c := start; c < len(g.Columns); c++ {
		paintCol(c, x)
		x += g.Columns[c].Width
		if x >= g.Size.X {
			return
		}
	}
}

// truncateEllipsis is like truncate but ends with "…" when it had to
// drop characters — gives the user a visual signal that the cell has
// more content than fits.
func truncateEllipsis(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if utf8.StringDisplayWidth(s) <= maxLen {
		return s
	}
	if maxLen < 2 {
		return utf8.CopyDisplayCells(s, 0, maxLen)
	}
	return utf8.CopyDisplayCells(s, 0, maxLen-1) + "…"
}

// truncate cuts s to at most maxLen display cells.
func truncate(s string, maxLen int) string {
	if maxLen < 0 {
		return ""
	}
	return utf8.CopyDisplayCells(s, 0, maxLen)
}

// alignText pads/clips s to width with the given alignment.
func alignText(s string, width int, align Alignment) string {
	w := utf8.StringDisplayWidth(s)
	if w >= width {
		return s
	}
	pad := width - w
	switch align {
	case AlignRight:
		return spaces(pad) + s
	case AlignCenter:
		l := pad / 2
		return spaces(l) + s + spaces(pad-l)
	}
	return s + spaces(pad)
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

// HandleEvent dispatches keyboard / mouse events.
func (g *StringGrid) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		g.handleMouseDown(ev)
		return
	}
	if ev.What == consts.EvMouseMove && g.drag != dragNone {
		g.handleMouseMove(ev)
		return
	}
	if ev.What == consts.EvMouseUp && g.drag != dragNone {
		g.handleMouseUp(ev)
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	if g.editing {
		g.handleEditKey(ev)
		return
	}
	shift := ev.KeyShift&consts.KbLeftShift != 0
	ctrl := ev.KeyShift&consts.KbCtrlShift != 0
	switch ev.KeyCode {
	case consts.KbLeft:
		g.moveTo(g.Focus.Col-1, g.Focus.Row, shift)
	case consts.KbRight:
		g.moveTo(g.Focus.Col+1, g.Focus.Row, shift)
	case consts.KbUp:
		g.moveTo(g.Focus.Col, g.Focus.Row-1, shift)
	case consts.KbDown:
		g.moveTo(g.Focus.Col, g.Focus.Row+1, shift)
	case consts.KbHome:
		g.moveTo(0, g.Focus.Row, shift)
	case consts.KbEnd:
		g.moveTo(len(g.Columns)-1, g.Focus.Row, shift)
	case consts.KbCtrlHome:
		g.moveTo(0, 0, shift)
	case consts.KbCtrlEnd:
		g.moveTo(len(g.Columns)-1, g.RowCount()-1, shift)
	case consts.KbPgUp:
		g.moveTo(g.Focus.Col, g.Focus.Row-g.dataRows(), shift)
	case consts.KbPgDn:
		g.moveTo(g.Focus.Col, g.Focus.Row+g.dataRows(), shift)
	case consts.KbCtrlA:
		// Select everything.
		g.Mode = SelectRange
		g.Anchor = Cell{Col: 0, Row: 0}
		g.Focus = Cell{Col: len(g.Columns) - 1, Row: g.RowCount() - 1}
	case consts.KbCtrlIns, consts.KbCtrlC:
		g.CopySelection()
	case consts.KbEnter, consts.KbF2:
		g.beginEdit()
	default:
		if ctrl {
			return
		}
		// Any printable starts edit + replaces cell with that char.
		if ev.UnicodeChar >= ' ' {
			g.beginEdit()
			g.editBuf = []rune{ev.UnicodeChar}
			g.editPos = 1
		} else {
			return
		}
	}
	g.ClearEvent(ev)
}

// handleMouseDown handles a fresh click — figures out whether it's
// header sort, header drag (reorder), separator drag (resize), filter
// edit, or cell select.
func (g *StringGrid) handleMouseDown(ev *drivers.Event) {
	local := g.MakeLocal(ev.Where)
	// Wheel scrolls vertically without selecting.
	if ev.Buttons&consts.MbScrollWheelUp != 0 {
		g.moveTo(g.Focus.Col, g.Focus.Row-3, false)
		g.ClearEvent(ev)
		return
	}
	if ev.Buttons&consts.MbScrollWheelDown != 0 {
		g.moveTo(g.Focus.Col, g.Focus.Row+3, false)
		g.ClearEvent(ev)
		return
	}
	if g.Owner != nil {
		g.Owner.Focus(g.Self())
	}
	rowOff := 0
	// Click on header row (title): sort, or start a column reorder drag.
	if g.HasHeader && local.Y == 0 {
		// Check if the click is on a column separator (last cell of the column).
		if sepCol := g.separatorAt(local.X); sepCol >= 0 {
			g.drag = dragResize
			g.dragCol = sepCol
			g.dragFromX = local.X
			g.ClearEvent(ev)
			return
		}
		if col := g.columnAt(local.X); col >= 0 {
			if ev.DoubleClk {
				// Double-click toggles sort even faster.
				g.CycleSort(col)
			} else {
				g.CycleSort(col)
				// Set up a tentative reorder drag if user keeps holding.
				g.drag = dragReorder
				g.dragCol = col
				g.dragFromX = local.X
			}
		}
		g.ClearEvent(ev)
		return
	}
	// Header underline row: swallow clicks so a stray hit on the
	// separator strip doesn't accidentally select the first data row.
	if g.HasHeader && local.Y == 1 {
		g.ClearEvent(ev)
		return
	}
	rowOff = 0
	if g.HasHeader {
		// Title + underline.
		rowOff += 2
	}
	if g.ShowFilter && local.Y == rowOff {
		// Click on filter row — focus that filter input.
		if col := g.columnAt(local.X); col >= 0 {
			g.editing = true
			g.editFocus = true
			g.editBuf = []rune(g.Filters[col])
			g.editPos = len(g.editBuf)
			g.Focus.Col = col
		}
		g.ClearEvent(ev)
		return
	}
	if g.ShowFilter {
		rowOff++
	}
	if local.Y < rowOff {
		g.ClearEvent(ev)
		return
	}
	row := g.Top + local.Y - rowOff
	col := g.columnAt(local.X)
	if col < 0 {
		g.ClearEvent(ev)
		return
	}
	shift := ev.KeyShift&consts.KbLeftShift != 0
	if row < 0 {
		row = 0
	}
	if row >= g.RowCount() {
		row = g.RowCount() - 1
	}
	if shift {
		g.moveTo(col, row, true)
	} else {
		g.moveTo(col, row, false)
		g.drag = dragSelect
		g.dragFromX = local.X
	}
	g.ClearEvent(ev)
}

// separatorAt returns the column whose RIGHT edge is at local X, or
// -1 if X isn't on a separator. Used for column-resize hit detection.
func (g *StringGrid) separatorAt(x int) int {
	cur := 0
	for c := 0; c < len(g.Columns); c++ {
		cur += g.Columns[c].Width
		if x == cur-1 {
			return c
		}
		// Only walk visible columns.
		if c < g.FixedCols {
			continue
		}
	}
	return -1
}

// handleMouseMove extends an in-flight drag.
func (g *StringGrid) handleMouseMove(ev *drivers.Event) {
	local := g.MakeLocal(ev.Where)
	switch g.drag {
	case dragSelect:
		rowOff := g.headerHeight()
		row := g.Top + local.Y - rowOff
		col := g.columnAt(local.X)
		if col < 0 || row < 0 || row >= g.RowCount() {
			return
		}
		g.moveTo(col, row, true)
		g.ClearEvent(ev)
	case dragResize:
		delta := local.X - g.dragFromX
		if g.dragCol >= 0 && g.dragCol < len(g.Columns) {
			w := g.Columns[g.dragCol].Width + delta
			if w >= 3 {
				g.Columns[g.dragCol].Width = w
				g.dragFromX = local.X
			}
		}
		g.ClearEvent(ev)
	case dragReorder:
		// Convert reorder threshold: move at least 1 cell to commit.
		if abs(local.X-g.dragFromX) > 1 {
			target := g.columnAt(local.X)
			if target >= 0 && target != g.dragCol {
				g.swapColumns(g.dragCol, target)
				g.dragCol = target
				g.dragFromX = local.X
			}
		}
		g.ClearEvent(ev)
	}
}

func (g *StringGrid) handleMouseUp(ev *drivers.Event) {
	g.drag = dragNone
	g.ClearEvent(ev)
}

// swapColumns moves the column at from to position to. Pascal-style
// column reorder via header drag.
func (g *StringGrid) swapColumns(from, to int) {
	if from < 0 || from >= len(g.Columns) || to < 0 || to >= len(g.Columns) {
		return
	}
	col := g.Columns[from]
	g.Columns = append(g.Columns[:from], g.Columns[from+1:]...)
	g.Columns = append(g.Columns[:to], append([]Column{col}, g.Columns[to:]...)...)
	// Move filter strings to stay aligned with columns.
	if from < len(g.Filters) && to < len(g.Filters) {
		f := g.Filters[from]
		g.Filters = append(g.Filters[:from], g.Filters[from+1:]...)
		g.Filters = append(g.Filters[:to], append([]string{f}, g.Filters[to:]...)...)
	}
	// In non-virtual mode, also move the data column.
	if g.OnGetCell == nil {
		for r := range g.rows {
			if from >= len(g.rows[r]) || to >= len(g.rows[r]) {
				continue
			}
			v := g.rows[r][from]
			g.rows[r] = append(g.rows[r][:from], g.rows[r][from+1:]...)
			g.rows[r] = append(g.rows[r][:to], append([]string{v}, g.rows[r][to:]...)...)
		}
	}
	// Sort tracking column shifts too.
	if g.SortCol == from {
		g.SortCol = to
	}
	g.markDirty()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// beginEdit puts the focused cell into edit mode.
func (g *StringGrid) beginEdit() {
	if g.Focus.Col < 0 || g.Focus.Col >= len(g.Columns) {
		return
	}
	if g.Columns[g.Focus.Col].ReadOnly {
		return
	}
	g.editing = true
	g.editFocus = false
	g.editBuf = []rune(g.Cell(g.Focus.Row, g.Focus.Col))
	g.editPos = len(g.editBuf)
	g.editStatus = ""
}

// commitEdit writes the edit buffer back. Cell edits run through the
// column's Validator if present; failed validation keeps the user in
// edit mode and surfaces the error in editStatus.
func (g *StringGrid) commitEdit() {
	if !g.editing {
		return
	}
	value := string(g.editBuf)
	if g.editFocus {
		// Filter edit — applies directly, no validator.
		g.SetFilter(g.Focus.Col, value)
		g.editing = false
		g.editFocus = false
		g.editBuf = nil
		return
	}
	if g.Focus.Col >= 0 && g.Focus.Col < len(g.Columns) {
		if v := g.Columns[g.Focus.Col].Validator; v != nil {
			if !v.IsValid(value) {
				g.editStatus = "Invalid"
				return
			}
		}
	}
	g.SetCell(g.Focus.Row, g.Focus.Col, value)
	g.editing = false
	g.editBuf = nil
}

// handleEditKey processes keystrokes while a cell is in edit mode.
// Filter-row edits (editFocus) apply live on every keystroke so the
// grid feels responsive — no need to hit Enter to see results. Tab /
// Shift-Tab move between filter columns without leaving edit mode.
// Esc on an empty filter buffer clears the filter for that column.
func (g *StringGrid) handleEditKey(ev *drivers.Event) {
	mutated := false
	switch ev.KeyCode {
	case consts.KbEsc:
		if g.editFocus {
			// On Esc, clear the current filter and leave edit mode —
			// matches how every other "search box" widget behaves.
			g.SetFilter(g.Focus.Col, "")
		}
		g.editing = false
		g.editFocus = false
		g.editBuf = nil
	case consts.KbEnter:
		g.commitEdit()
	case consts.KbTab:
		if g.editFocus {
			g.advanceFilterColumn(1)
		}
	case consts.KbShiftTab:
		if g.editFocus {
			g.advanceFilterColumn(-1)
		}
	case consts.KbLeft:
		if g.editPos > 0 {
			g.editPos--
		}
	case consts.KbRight:
		if g.editPos < len(g.editBuf) {
			g.editPos++
		}
	case consts.KbHome:
		g.editPos = 0
	case consts.KbEnd:
		g.editPos = len(g.editBuf)
	case consts.KbBack:
		if g.editPos > 0 {
			g.editBuf = append(g.editBuf[:g.editPos-1], g.editBuf[g.editPos:]...)
			g.editPos--
			mutated = true
		}
	case consts.KbDel:
		if g.editPos < len(g.editBuf) {
			g.editBuf = append(g.editBuf[:g.editPos], g.editBuf[g.editPos+1:]...)
			mutated = true
		}
	default:
		if ev.UnicodeChar >= ' ' {
			g.editBuf = append(g.editBuf[:g.editPos], append([]rune{ev.UnicodeChar}, g.editBuf[g.editPos:]...)...)
			g.editPos++
			mutated = true
		} else {
			return
		}
	}
	if mutated && g.editFocus {
		g.SetFilter(g.Focus.Col, string(g.editBuf))
	}
	g.ClearEvent(ev)
}

// advanceFilterColumn commits any pending edit in the current filter
// column and re-enters edit mode in the next (or previous) column,
// wrapping around the end of the row. Used by Tab / Shift-Tab inside
// the filter row.
func (g *StringGrid) advanceFilterColumn(dir int) {
	if len(g.Columns) == 0 {
		return
	}
	g.SetFilter(g.Focus.Col, string(g.editBuf))
	next := (g.Focus.Col + dir + len(g.Columns)) % len(g.Columns)
	g.Focus.Col = next
	g.editBuf = []rune(g.Filters[next])
	g.editPos = len(g.editBuf)
}
