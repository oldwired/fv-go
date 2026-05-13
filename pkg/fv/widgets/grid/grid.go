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
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/validators"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/popupmenu"
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
// can be cleared to lock a column out of header-click sorting. The
// optional Compare hook is used for typed sorts (e.g. numeric); when
// nil, the grid falls back to lexicographic string compare.
type Column struct {
	Title        string
	Width        int
	MinWidth     int // 0 = no lower clamp on resize / auto-fit
	MaxWidth     int // 0 = no upper clamp
	Align        Alignment
	Color        uint16 // per-column override; 0 = use the grid default
	ReadOnly     bool
	Sortable     bool
	Visible      bool   // hidden columns stay in data + filters but don't render
	DefaultValue string // used by AddRow() / SetRowCount() to fill new cells
	Validator    validators.Validator
	Compare      func(a, b string) int // typed comparison; nil = string compare
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

// SortKey is one entry in a multi-column sort. Multiple keys form a
// lexicographic comparison: when the primary key ties, the secondary
// breaks the tie, and so on.
type SortKey struct {
	Col int
	Dir SortDirection
}

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

	// FixedRows pins the first N rows (in visible / post-sort order) so
	// they stay on screen even when the body scrolls.
	FixedRows int

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

	// Visual toggles. All default to true via New(); callers can flip
	// any of them off to get a stripped-down look.
	ShowGridLines       bool // "│" column dividers between cells
	ShowZebra           bool // alternating row backgrounds
	ShowHeaderUnderline bool // the "─" row under the header titles

	// Behavior toggles. All default to true.
	ReadOnly         bool // disables every mutation path from input
	AllowResize      bool // mouse-drag on column separator
	AllowReorder     bool // mouse-drag on column header
	AllowDragSelect  bool // mouse-drag on cells extends selection
	AllowWheelScroll bool // wheel scrolls the body

	// Sort state. SortKeys[0] is the primary sort column; subsequent
	// entries break ties. Empty = unsorted. SortCol / SortDir mirror
	// the primary key for back-compat with existing display code.
	SortKeys []SortKey
	SortCol  int
	SortDir  SortDirection

	// FindText is the active incremental-search substring (set by
	// FindFirst / FindNext); used purely for cell highlighting.
	FindText string

	// Per-cell color hook. Called for every data cell during Draw with
	// the base attr the grid would use; returning a non-zero override
	// is painted instead. Useful for conditional formatting / heatmaps.
	OnCellAttr func(row, col int, base uint16) uint16

	// Edit callbacks. OnBeforeEdit can deny an edit by returning false.
	// OnAfterEdit fires after a successful commit. OnCellFocused fires
	// when MoveTo / arrow keys / mouse change Focus.
	OnBeforeEdit  func(row, col int, current string) bool
	OnAfterEdit   func(row, col int, oldVal, newVal string)
	OnCellFocused func(row, col int)

	// Modified is set true after any data-mutating edit and cleared by
	// SetRows / LoadCSV / SaveCSV / ClearModified.
	Modified bool

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

// New constructs an empty grid with the given columns. All visual /
// behavior toggles default to "on" — turn the ones you don't want off
// individually rather than passing a flags blob.
func New(bounds geom.Rect, cols []Column, h, v *views.ScrollBar) *StringGrid {
	// Per-column defaults: sortable + visible. Callers may have set
	// Visible=true explicitly, but the zero value of a freshly-built
	// Column is false, so default it on here.
	for i := range cols {
		if !cols[i].Sortable && cols[i].Title != "" {
			cols[i].Sortable = true
		}
		cols[i].Visible = true
	}
	g := &StringGrid{
		Base:                views.NewBase(bounds),
		Columns:             cols,
		HasHeader:           true,
		Mode:                SelectCell,
		SortCol:             -1,
		Filters:             make([]string, len(cols)),
		HScroll:             h,
		VScroll:             v,
		dirty:               true,
		ShowGridLines:       true,
		ShowZebra:           true,
		ShowHeaderUnderline: true,
		AllowResize:         true,
		AllowReorder:        true,
		AllowDragSelect:     true,
		AllowWheelScroll:    true,
	}
	g.SetSelf(g)
	g.Options |= consts.OfSelectable | consts.OfFirstClick
	g.State |= consts.SfCursorVis
	g.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	return g
}

// GetTypeID for serial registry.
func (g *StringGrid) GetTypeID() string { return "stringgrid" }

// SetRows replaces all cell data. Clears the Modified flag because
// the new data is the baseline by definition.
func (g *StringGrid) SetRows(rows [][]string) {
	g.rows = rows
	g.markDirty()
	g.clampSelection()
	g.refreshScroll()
	g.Modified = false
}

// AddRow appends a row. Missing cells are filled from each column's
// DefaultValue (or "" when unset). Sets the Modified flag.
func (g *StringGrid) AddRow(values []string) {
	if len(values) > len(g.Columns) {
		values = values[:len(g.Columns)]
	}
	for len(values) < len(g.Columns) {
		i := len(values)
		values = append(values, g.Columns[i].DefaultValue)
	}
	g.rows = append(g.rows, values)
	g.markDirty()
	g.refreshScroll()
	g.Modified = true
}

// ClearModified resets the dirty-data flag. Use it after persisting
// the grid through a non-CSV path that the grid can't detect itself.
func (g *StringGrid) ClearModified() { g.Modified = false }

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
// calls OnSetCell. Sets Modified and fires OnAfterEdit when the value
// actually changes.
func (g *StringGrid) SetCell(row, col int, value string) {
	g.ensureVisible()
	if g.OnSetCell != nil {
		if row >= 0 && row < len(g.visibleRows) {
			rawRow := g.visibleRows[row]
			old := g.rawCell(rawRow, col)
			g.OnSetCell(rawRow, col, value)
			if old != value {
				g.Modified = true
				if g.OnAfterEdit != nil {
					g.OnAfterEdit(rawRow, col, old, value)
				}
			}
		}
		g.markDirty()
		return
	}
	if row < 0 || row >= len(g.visibleRows) {
		// Out of visible range — extend in raw space.
		for row >= len(g.rows) {
			g.rows = append(g.rows, make([]string, len(g.Columns)))
		}
		old := g.rows[row][col]
		g.rows[row][col] = value
		g.markDirty()
		if old != value {
			g.Modified = true
			if g.OnAfterEdit != nil {
				g.OnAfterEdit(row, col, old, value)
			}
		}
		return
	}
	rawRow := g.visibleRows[row]
	for rawRow >= len(g.rows) {
		g.rows = append(g.rows, make([]string, len(g.Columns)))
	}
	for col >= len(g.rows[rawRow]) {
		g.rows[rawRow] = append(g.rows[rawRow], "")
	}
	old := g.rows[rawRow][col]
	g.rows[rawRow][col] = value
	g.markDirty()
	if old != value {
		g.Modified = true
		if g.OnAfterEdit != nil {
			g.OnAfterEdit(rawRow, col, old, value)
		}
	}
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
	g.Modified = true
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
// Multi-column sort runs the SortKeys in order; ties on the primary
// key are broken by subsequent keys. Per-column Compare hooks let
// callers swap in numeric / date-aware ordering.
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
	if len(g.SortKeys) > 0 {
		sort.SliceStable(g.visibleRows, func(i, j int) bool {
			ri, rj := g.visibleRows[i], g.visibleRows[j]
			for _, k := range g.SortKeys {
				if k.Col < 0 || k.Col >= len(g.Columns) {
					continue
				}
				a := g.rawCell(ri, k.Col)
				b := g.rawCell(rj, k.Col)
				var cmp int
				if fn := g.Columns[k.Col].Compare; fn != nil {
					cmp = fn(a, b)
				} else {
					switch {
					case a < b:
						cmp = -1
					case a > b:
						cmp = 1
					}
				}
				if k.Dir == SortDesc {
					cmp = -cmp
				}
				if cmp != 0 {
					return cmp < 0
				}
			}
			return false
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
// (col, SortNone) to clear sorting. Replaces any existing multi-key
// sort; use AddSortKey to extend instead.
func (g *StringGrid) Sort(col int, dir SortDirection) {
	if col < 0 || col >= len(g.Columns) {
		dir = SortNone
	}
	if dir == SortNone {
		g.SortKeys = nil
	} else {
		g.SortKeys = []SortKey{{Col: col, Dir: dir}}
	}
	g.syncPrimarySort()
	g.markDirty()
}

// AddSortKey appends col as a secondary sort key, or — if col is
// already in the chain — cycles its direction (Asc → Desc → remove).
// Used by Shift-click on header to build multi-column sorts.
func (g *StringGrid) AddSortKey(col int) {
	if col < 0 || col >= len(g.Columns) || !g.Columns[col].Sortable {
		return
	}
	for i, k := range g.SortKeys {
		if k.Col != col {
			continue
		}
		switch k.Dir {
		case SortAsc:
			g.SortKeys[i].Dir = SortDesc
		case SortDesc:
			g.SortKeys = append(g.SortKeys[:i], g.SortKeys[i+1:]...)
		default:
			g.SortKeys[i].Dir = SortAsc
		}
		g.syncPrimarySort()
		g.markDirty()
		return
	}
	g.SortKeys = append(g.SortKeys, SortKey{Col: col, Dir: SortAsc})
	g.syncPrimarySort()
	g.markDirty()
}

// CycleSort advances col through asc → desc → unsorted. Used by a
// plain header click. Only fires when the column is Sortable.
func (g *StringGrid) CycleSort(col int) {
	if col < 0 || col >= len(g.Columns) || !g.Columns[col].Sortable {
		return
	}
	// Replace the entire sort chain with this column so that an
	// unmodified click feels like a fresh sort rather than appending.
	if len(g.SortKeys) != 1 || g.SortKeys[0].Col != col {
		g.SortKeys = []SortKey{{Col: col, Dir: SortAsc}}
		g.syncPrimarySort()
		g.markDirty()
		return
	}
	switch g.SortKeys[0].Dir {
	case SortAsc:
		g.SortKeys[0].Dir = SortDesc
	case SortDesc:
		g.SortKeys = nil
	default:
		g.SortKeys[0].Dir = SortAsc
	}
	g.syncPrimarySort()
	g.markDirty()
}

// syncPrimarySort keeps SortCol / SortDir mirroring SortKeys[0] so
// callers and the header-glyph renderer can keep reading the old
// single-column fields without knowing about the multi-key chain.
func (g *StringGrid) syncPrimarySort() {
	if len(g.SortKeys) == 0 {
		g.SortCol = -1
		g.SortDir = SortNone
		return
	}
	g.SortCol = g.SortKeys[0].Col
	g.SortDir = g.SortKeys[0].Dir
}

// ClearSort drops every sort key.
func (g *StringGrid) ClearSort() {
	g.SortKeys = nil
	g.syncPrimarySort()
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

// colVisible reports whether the column at idx renders / hit-tests.
func (g *StringGrid) colVisible(c int) bool {
	return c >= 0 && c < len(g.Columns) && g.Columns[c].Visible
}

// columnX returns the screen X position of column c relative to the
// grid's left edge. Pinned columns are always at their natural X;
// scrollable columns are offset by LeftCol. Invisible columns
// contribute 0 to the layout.
func (g *StringGrid) columnX(c int) int {
	if c < 0 {
		return 0
	}
	x := 0
	for i := 0; i < g.FixedCols && i < len(g.Columns) && i < c; i++ {
		if g.Columns[i].Visible {
			x += g.Columns[i].Width
		}
	}
	if c < g.FixedCols {
		return x
	}
	start := g.LeftCol
	if start < g.FixedCols {
		start = g.FixedCols
	}
	for i := start; i < c && i < len(g.Columns); i++ {
		if g.Columns[i].Visible {
			x += g.Columns[i].Width
		}
	}
	return x
}

// columnAt returns the column index whose cells cover screen-relative
// x, or -1 if none. Pinned columns get priority hits. Invisible
// columns are skipped entirely.
func (g *StringGrid) columnAt(x int) int {
	cur := 0
	for i := 0; i < g.FixedCols && i < len(g.Columns); i++ {
		if !g.Columns[i].Visible {
			continue
		}
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
		if !g.Columns[i].Visible {
			continue
		}
		w := g.Columns[i].Width
		if x >= cur && x < cur+w {
			return i
		}
		cur += w
	}
	return -1
}

// nextVisibleCol returns the next column index >= start that's
// visible, or -1 if none.
func (g *StringGrid) nextVisibleCol(start, dir int) int {
	if len(g.Columns) == 0 {
		return -1
	}
	i := start
	for steps := 0; steps < len(g.Columns); steps++ {
		i = (i + dir + len(g.Columns)) % len(g.Columns)
		if g.Columns[i].Visible {
			return i
		}
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
	// Skip past invisible columns. If everything's invisible (edge
	// case), fall through and keep col as-is.
	if c := len(g.Columns); c > 0 && !g.Columns[col].Visible {
		if next := g.nextVisibleCol(col-1, 1); next >= 0 {
			col = next
		}
	}
	old := g.Focus
	g.Focus = Cell{Col: col, Row: row}
	if !extend {
		g.Anchor = g.Focus
	}
	if extend && g.Mode == SelectCell {
		g.Mode = SelectRange
	}
	g.adjustScroll()
	if g.OnCellFocused != nil && (old.Col != g.Focus.Col || old.Row != g.Focus.Row) {
		g.OnCellFocused(g.Focus.Row, g.Focus.Col)
	}
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

// adjustScroll keeps the focused cell visible. The first FixedRows
// visible rows are always painted; only Focus.Row >= FixedRows can
// affect Top.
func (g *StringGrid) adjustScroll() {
	rows := g.dataRows()
	if g.Top < g.FixedRows {
		g.Top = g.FixedRows
	}
	scrollable := rows - g.FixedRows
	if scrollable < 1 {
		scrollable = 1
	}
	if g.Focus.Row >= g.FixedRows {
		if g.Focus.Row < g.Top {
			g.Top = g.Focus.Row
		} else if g.Focus.Row >= g.Top+scrollable {
			g.Top = g.Focus.Row - scrollable + 1
		}
		if g.Top < g.FixedRows {
			g.Top = g.FixedRows
		}
	}
	if g.Focus.Col < g.LeftCol && g.Focus.Col >= g.FixedCols {
		g.LeftCol = g.Focus.Col
	}
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
	// Clamp Top so the scrollable section never tries to start before
	// the pinned rows end.
	if g.Top < g.FixedRows {
		g.Top = g.FixedRows
	}
	// Palette. Header is white on dark blue (calmer than the previous
	// red bar); rows alternate fg brightness for subtle zebra striping;
	// column separators are dark-gray glyphs that visually anchor the
	// grid without competing with the data.
	pal := theme.Get()
	headerColor := pal.GridHeader
	headerSepColor := pal.GridHeaderSep
	rowEven := pal.GridCellAlt
	rowOdd := pal.GridCell
	pinnedColor := pal.GridPinned
	selColor := pal.PopupMenuNormal
	focusColor := pal.ComboButton
	editColor := pal.GridCellCursor
	filterColor := pal.GridFrame
	dividerAttrFor := func(bg byte) uint16 {
		return types.MakeAttr(0x08, bg)
	}

	rowOff := 0
	if g.HasHeader {
		buf := screen.MakeDrawBuffer(g.Size.X)
		for x := 0; x < g.Size.X; x++ {
			screen.DrawCell(buf, x, " ", headerColor)
		}
		g.drawCellsRow(buf, headerColor, dividerAttrFor(0x01), func(c int) string {
			suffix := g.sortGlyph(c)
			budget := g.Columns[c].Width - 2 - utf8.StringDisplayWidth(suffix)
			if budget < 0 {
				budget = 0
			}
			label := truncate(g.Columns[c].Title, budget) + suffix
			return alignText(label, g.Columns[c].Width-1, g.Columns[c].Align)
		})
		g.WriteLine(0, 0, g.Size.X, 1, buf)
		rowOff++
		if g.ShowHeaderUnderline {
			sep := screen.MakeDrawBuffer(g.Size.X)
			for x := 0; x < g.Size.X; x++ {
				screen.DrawCell(sep, x, "─", headerSepColor)
			}
			if g.ShowGridLines {
				g.drawCellSeparators(sep, headerSepColor, "┬")
			}
			g.WriteLine(0, rowOff, g.Size.X, 1, sep)
			rowOff++
		}
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
		// Resolve which raw-visible row this body line shows. The
		// first FixedRows lines always show rows 0..FixedRows-1; the
		// rest are scroll-based.
		var visRow int
		isPinned := false
		if r < g.FixedRows {
			visRow = r
			isPinned = true
		} else {
			visRow = g.Top + (r - g.FixedRows)
		}
		rowAttr := rowEven
		if g.ShowZebra && visRow%2 == 1 {
			rowAttr = rowOdd
		}
		if isPinned {
			rowAttr = pinnedColor
		}
		buf := screen.MakeDrawBuffer(g.Size.X)
		for x := 0; x < g.Size.X; x++ {
			screen.DrawCell(buf, x, " ", rowAttr)
		}
		if visRow >= g.RowCount() {
			g.WriteLine(0, rowOff+r, g.Size.X, 1, buf)
			continue
		}
		g.drawCellsRowAt(buf, visRow, rowAttr, selColor, focusColor, editColor, dividerAttrFor(types.BG(rowAttr)))
		if g.ShowRowMarker && visRow == g.Focus.Row {
			bg := types.BG(buf[0].Attr)
			buf[0] = types.DrawCell{Ch: "►", Attr: types.MakeAttr(0x0E, bg)} // intentionally synthesized: marker FG over per-row bg
		}
		g.WriteLine(0, rowOff+r, g.Size.X, 1, buf)
	}
	// Empty-state placeholder when no data and no filter is hiding rows.
	if g.RowCount() == 0 && g.RawRowCount() == 0 && g.dataRows() > 0 {
		buf := screen.MakeDrawBuffer(g.Size.X)
		for x := 0; x < g.Size.X; x++ {
			screen.DrawCell(buf, x, " ", pal.EditorComment)
		}
		msg := "(no rows)"
		mw := utf8.StringDisplayWidth(msg)
		screen.DrawStr(buf, (g.Size.X-mw)/2, msg, pal.EditorComment)
		g.WriteLine(0, rowOff+g.dataRows()/2, g.Size.X, 1, buf)
	}
	// Caret in edit mode. The filter-edit cursor lives in the filter
	// row regardless of which data row Focus.Row points at. The
	// data-edit cursor needs FixedRows-aware Y math.
	if g.editing {
		x := g.columnX(g.Focus.Col) + g.editPos + 1 // +1 for leading divider pad
		var y int
		switch {
		case g.editFocus && filterRowY >= 0:
			y = filterRowY
		case g.Focus.Row < g.FixedRows:
			y = rowOff + g.Focus.Row
		default:
			y = rowOff + g.FixedRows + (g.Focus.Row - g.Top)
		}
		g.Cursor = geom.Point{X: x, Y: y}
	}
}

// sortGlyph returns the suffix to append after a column's title when
// it's part of the active sort. Includes a numeric index when the
// column is the secondary / tertiary key in a multi-column sort.
func (g *StringGrid) sortGlyph(col int) string {
	for i, k := range g.SortKeys {
		if k.Col != col {
			continue
		}
		arrow := " ▲"
		if k.Dir == SortDesc {
			arrow = " ▼"
		}
		if len(g.SortKeys) > 1 {
			// Index is 1-based for readability ("primary, then 2, then 3…").
			return arrow + subscript(i+1)
		}
		return arrow
	}
	return ""
}

// subscript returns the Unicode subscript-digit representation of n
// (single-digit only — multi-column sorts with >9 keys are uncommon).
func subscript(n int) string {
	if n < 0 || n > 9 {
		return ""
	}
	return string([]rune("₀₁₂₃₄₅₆₇₈₉")[n])
}

// drawCellSeparators overlays the "┬" / "┼" / "┴" glyphs onto a
// horizontal-rule row so column boundaries line up visually with the
// data rows' "│" dividers. Walks the same column geometry as the
// data renderer; invisible columns are skipped.
func (g *StringGrid) drawCellSeparators(buf screen.DrawBuffer, attr uint16, glyph string) {
	x := 0
	for c := 0; c < g.FixedCols && c < len(g.Columns); c++ {
		if !g.Columns[c].Visible {
			continue
		}
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
		if !g.Columns[c].Visible {
			continue
		}
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
// dividers between columns; pass 0 to suppress them.
func (g *StringGrid) drawCellsRow(buf screen.DrawBuffer, attr uint16, divAttr uint16, textOf func(c int) string) {
	paint := func(c int, x int) int {
		col := g.Columns[c]
		s := textOf(c)
		text := truncate(s, col.Width-1)
		for i, ch := range text {
			if x+i < g.Size.X {
				buf[x+i] = types.DrawCell{Ch: string(ch), Attr: attr}
			}
		}
		if g.ShowGridLines && x+col.Width-1 < g.Size.X {
			buf[x+col.Width-1] = types.DrawCell{Ch: "│", Attr: divAttr}
		}
		return x + col.Width
	}
	x := 0
	for c := 0; c < g.FixedCols && c < len(g.Columns); c++ {
		if !g.Columns[c].Visible {
			continue
		}
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
		if !g.Columns[c].Visible {
			continue
		}
		x = paint(c, x)
		if x >= g.Size.X {
			return
		}
	}
}

// drawCellsRowAt paints a single data row, picking per-cell attr
// based on selection / focus / edit / find / per-column / per-cell
// overrides. divAttr is the divider color when the cell isn't
// highlighted; selected / focused cells skip the divider so the
// highlight reads as one continuous block.
func (g *StringGrid) drawCellsRowAt(buf screen.DrawBuffer, visRow int, cellColor, selColor, focusColor, editColor, divAttr uint16) {
	paintCol := func(c int, x int) {
		col := g.Columns[c]
		text := g.Cell(visRow, c)
		// Pick the base attr: per-column color > grid default.
		attr := cellColor
		if col.Color != 0 {
			attr = col.Color
		}
		// User OnCellAttr hook gets the last word (after base + column
		// color, before selection / focus / edit override).
		if g.OnCellAttr != nil {
			if a := g.OnCellAttr(visRow, c, attr); a != 0 {
				attr = a
			}
		}
		highlighted := false
		if g.inSelection(visRow, c) {
			attr = selColor
			highlighted = true
		}
		if visRow == g.Focus.Row && c == g.Focus.Col {
			attr = focusColor
			highlighted = true
			if g.editing && !g.editFocus {
				attr = editColor
				text = string(g.editBuf)
			}
		}
		// Fill column with the chosen attr.
		for i := 0; i < col.Width && x+i < g.Size.X; i++ {
			buf[x+i] = types.DrawCell{Ch: " ", Attr: attr}
		}
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
		// Find-match highlight: invert the cells that contain the
		// active search substring. Cheap to compute per-cell since
		// FindText is short and most rows won't match.
		if !highlighted && g.FindText != "" && cellMatchesFind(text, g.FindText) {
			findAttr := theme.Get().TooltipNormal
			for i := 0; i < innerW && x+1+i < g.Size.X; i++ {
				if i < len(padded) {
					buf[x+1+i].Attr = findAttr
				}
			}
		}
		if g.ShowGridLines && !highlighted && x+col.Width-1 < g.Size.X {
			buf[x+col.Width-1] = types.DrawCell{Ch: "│", Attr: divAttr}
		}
	}
	x := 0
	for c := 0; c < g.FixedCols && c < len(g.Columns); c++ {
		if !g.Columns[c].Visible {
			continue
		}
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
		if !g.Columns[c].Visible {
			continue
		}
		paintCol(c, x)
		x += g.Columns[c].Width
		if x >= g.Size.X {
			return
		}
	}
}

// cellMatchesFind is a case-insensitive ASCII substring match. Same
// semantics as the filter row's containsFold.
func cellMatchesFind(cell, needle string) bool { return containsFold(cell, needle) }

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
		if next := g.nextVisibleCol(g.Focus.Col, -1); next >= 0 && next < g.Focus.Col {
			g.moveTo(next, g.Focus.Row, shift)
		}
	case consts.KbRight:
		if next := g.nextVisibleCol(g.Focus.Col, 1); next >= 0 && next > g.Focus.Col {
			g.moveTo(next, g.Focus.Row, shift)
		}
	case consts.KbUp:
		g.moveTo(g.Focus.Col, g.Focus.Row-1, shift)
	case consts.KbDown:
		g.moveTo(g.Focus.Col, g.Focus.Row+1, shift)
	case consts.KbHome:
		first := g.nextVisibleCol(len(g.Columns)-1, 1)
		if first >= 0 {
			g.moveTo(first, g.Focus.Row, shift)
		}
	case consts.KbEnd:
		last := g.nextVisibleCol(0, -1)
		if last >= 0 {
			g.moveTo(last, g.Focus.Row, shift)
		}
	case consts.KbCtrlHome:
		first := g.nextVisibleCol(len(g.Columns)-1, 1)
		if first >= 0 {
			g.moveTo(first, 0, shift)
		}
	case consts.KbCtrlEnd:
		last := g.nextVisibleCol(0, -1)
		if last >= 0 {
			g.moveTo(last, g.RowCount()-1, shift)
		}
	case consts.KbPgUp:
		g.moveTo(g.Focus.Col, g.Focus.Row-g.dataRows(), shift)
	case consts.KbPgDn:
		g.moveTo(g.Focus.Col, g.Focus.Row+g.dataRows(), shift)
	case consts.KbCtrlA:
		g.Mode = SelectRange
		g.Anchor = Cell{Col: 0, Row: 0}
		g.Focus = Cell{Col: len(g.Columns) - 1, Row: g.RowCount() - 1}
	case consts.KbCtrlIns, consts.KbCtrlC:
		g.CopySelection()
	case consts.KbShiftIns, consts.KbCtrlV:
		g.PasteClipboard()
	case consts.KbCtrlF:
		g.promptFind()
	case consts.KbF3:
		g.FindNext(1)
	case consts.KbEnter, consts.KbF2:
		g.beginEdit()
	default:
		if ctrl {
			return
		}
		if ev.UnicodeChar >= ' ' {
			if !g.canEditFocused() {
				return
			}
			g.beginEdit()
			g.editBuf = []rune{ev.UnicodeChar}
			g.editPos = 1
		} else {
			return
		}
	}
	g.ClearEvent(ev)
}

// canEditFocused returns true if the current cell can be edited
// (grid not ReadOnly, column not ReadOnly, OnBeforeEdit doesn't deny).
func (g *StringGrid) canEditFocused() bool {
	if g.ReadOnly {
		return false
	}
	if g.Focus.Col < 0 || g.Focus.Col >= len(g.Columns) {
		return false
	}
	if g.Columns[g.Focus.Col].ReadOnly {
		return false
	}
	if g.OnBeforeEdit != nil {
		cur := g.Cell(g.Focus.Row, g.Focus.Col)
		if !g.OnBeforeEdit(g.Focus.Row, g.Focus.Col, cur) {
			return false
		}
	}
	return true
}

// handleMouseDown handles a fresh click — figures out whether it's
// header sort, header drag (reorder), separator drag (resize), filter
// edit, or cell select. Respects the Allow* permission toggles.
func (g *StringGrid) handleMouseDown(ev *drivers.Event) {
	local := g.MakeLocal(ev.Where)
	if ev.Buttons&consts.MbScrollWheelUp != 0 {
		if g.AllowWheelScroll {
			g.moveTo(g.Focus.Col, g.Focus.Row-3, false)
		}
		g.ClearEvent(ev)
		return
	}
	if ev.Buttons&consts.MbScrollWheelDown != 0 {
		if g.AllowWheelScroll {
			g.moveTo(g.Focus.Col, g.Focus.Row+3, false)
		}
		g.ClearEvent(ev)
		return
	}
	if g.Owner != nil {
		g.Owner.Focus(g.Self())
	}
	shift := ev.KeyShift&consts.KbLeftShift != 0
	rightClick := ev.Buttons&consts.MbRightButton != 0
	rowOff := 0
	// Header row (title): sort / context menu / reorder / resize.
	if g.HasHeader && local.Y == 0 {
		if rightClick {
			if col := g.columnAt(local.X); col >= 0 {
				g.showHeaderMenu(col, ev.Where)
			}
			g.ClearEvent(ev)
			return
		}
		if sepCol := g.separatorAt(local.X); sepCol >= 0 {
			if ev.DoubleClk {
				g.AutoFitColumn(sepCol)
				g.ClearEvent(ev)
				return
			}
			if g.AllowResize {
				g.drag = dragResize
				g.dragCol = sepCol
				g.dragFromX = local.X
			}
			g.ClearEvent(ev)
			return
		}
		if col := g.columnAt(local.X); col >= 0 {
			switch {
			case shift:
				g.AddSortKey(col)
			default:
				g.CycleSort(col)
				if g.AllowReorder && !ev.DoubleClk {
					g.drag = dragReorder
					g.dragCol = col
					g.dragFromX = local.X
				}
			}
		}
		g.ClearEvent(ev)
		return
	}
	if g.HasHeader && g.ShowHeaderUnderline && local.Y == 1 {
		g.ClearEvent(ev)
		return
	}
	rowOff = 0
	if g.HasHeader {
		rowOff++
		if g.ShowHeaderUnderline {
			rowOff++
		}
	}
	if g.ShowFilter && local.Y == rowOff {
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
	dataY := local.Y - rowOff
	var row int
	if dataY < g.FixedRows {
		row = dataY
	} else {
		row = g.Top + (dataY - g.FixedRows)
	}
	col := g.columnAt(local.X)
	if col < 0 {
		g.ClearEvent(ev)
		return
	}
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
		if g.AllowDragSelect {
			g.drag = dragSelect
			g.dragFromX = local.X
		}
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
		dataY := local.Y - rowOff
		var row int
		if dataY < g.FixedRows {
			row = dataY
		} else {
			row = g.Top + (dataY - g.FixedRows)
		}
		col := g.columnAt(local.X)
		if col < 0 || row < 0 || row >= g.RowCount() {
			return
		}
		g.moveTo(col, row, true)
		g.ClearEvent(ev)
	case dragResize:
		delta := local.X - g.dragFromX
		if g.dragCol >= 0 && g.dragCol < len(g.Columns) {
			col := &g.Columns[g.dragCol]
			w := clampWidth(col, col.Width+delta)
			if w != col.Width {
				col.Width = w
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

// beginEdit puts the focused cell into edit mode. Honors the grid-
// level ReadOnly toggle, the column's ReadOnly flag, and OnBeforeEdit.
func (g *StringGrid) beginEdit() {
	if !g.canEditFocused() {
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
// column and re-enters edit mode in the next (or previous) visible
// column, wrapping around the end of the row. Used by Tab / Shift-Tab
// inside the filter row.
func (g *StringGrid) advanceFilterColumn(dir int) {
	if len(g.Columns) == 0 {
		return
	}
	g.SetFilter(g.Focus.Col, string(g.editBuf))
	next := g.nextVisibleCol(g.Focus.Col, dir)
	if next < 0 {
		return
	}
	g.Focus.Col = next
	g.editBuf = []rune(g.Filters[next])
	g.editPos = len(g.editBuf)
}

// clampWidth applies Column.MinWidth / MaxWidth bounds plus a global
// minimum of 3 cells (anything narrower is unreadable).
func clampWidth(col *Column, w int) int {
	if w < 3 {
		w = 3
	}
	if col.MinWidth > 0 && w < col.MinWidth {
		w = col.MinWidth
	}
	if col.MaxWidth > 0 && w > col.MaxWidth {
		w = col.MaxWidth
	}
	return w
}

// AutoFitColumn scans every currently-visible row, plus the column
// title, and resizes the column so the widest entry fits without
// truncation. Clamped by MinWidth / MaxWidth and the 3-cell global
// minimum. Triggered by double-clicking the column separator.
func (g *StringGrid) AutoFitColumn(col int) {
	if col < 0 || col >= len(g.Columns) {
		return
	}
	max := utf8.StringDisplayWidth(g.Columns[col].Title)
	g.ensureVisible()
	for _, raw := range g.visibleRows {
		w := utf8.StringDisplayWidth(g.rawCell(raw, col))
		if w > max {
			max = w
		}
	}
	// Add 2 cells of padding (the leading space + divider) so text
	// doesn't sit flush against the dividers.
	g.Columns[col].Width = clampWidth(&g.Columns[col], max+2)
	g.markDirty()
}

// AutoFitAll auto-fits every visible column. Convenience wrapper.
func (g *StringGrid) AutoFitAll() {
	for i := range g.Columns {
		if !g.Columns[i].Visible {
			continue
		}
		g.AutoFitColumn(i)
	}
}

// ResetColumnWidth restores the column to MinWidth (when set) or 12.
// Useful after a long auto-fit or manual drag the user wants to undo.
func (g *StringGrid) ResetColumnWidth(col int) {
	if col < 0 || col >= len(g.Columns) {
		return
	}
	w := g.Columns[col].MinWidth
	if w <= 0 {
		w = 12
	}
	g.Columns[col].Width = clampWidth(&g.Columns[col], w)
	g.markDirty()
}

// ToggleColumnVisible flips Visible for col. The grid keeps the
// column's underlying data; it just stops rendering / hit-testing it.
func (g *StringGrid) ToggleColumnVisible(col int) {
	if col < 0 || col >= len(g.Columns) {
		return
	}
	g.Columns[col].Visible = !g.Columns[col].Visible
	g.markDirty()
}

// SetFind installs a case-insensitive incremental-search needle.
// Matching cells light up; FindNext jumps the cursor between them.
// Empty needle clears the highlight.
func (g *StringGrid) SetFind(needle string) {
	g.FindText = needle
}

// FindNext jumps Focus to the next cell whose text contains FindText.
// dir = +1 for forward, -1 for backward. Wraps. No-op when FindText
// is empty or no match exists. Returns true on a successful jump.
func (g *StringGrid) FindNext(dir int) bool {
	if g.FindText == "" {
		return false
	}
	rows := g.RowCount()
	cols := len(g.Columns)
	if rows == 0 || cols == 0 {
		return false
	}
	total := rows * cols
	start := g.Focus.Row*cols + g.Focus.Col
	for i := 1; i <= total; i++ {
		idx := ((start+dir*i)%total + total) % total
		r := idx / cols
		c := idx % cols
		if !g.Columns[c].Visible {
			continue
		}
		if containsFold(g.Cell(r, c), g.FindText) {
			g.MoveTo(c, r)
			return true
		}
	}
	return false
}

// promptFind opens a small modal input dialog asking the user for
// a search needle, then jumps to the first match. Bound to Ctrl+F.
// Needs Owner to host the modal; without it, this is a no-op.
func (g *StringGrid) promptFind() {
	if g.Owner == nil {
		return
	}
	needle, ok := promptString(g.Owner, "Find", "Search for:", g.FindText)
	if !ok {
		return
	}
	g.FindText = needle
	g.FindNext(1)
}

// promptString puts up a small modal dialog with a single InputLine
// and OK / Cancel buttons. Returns the entered value and a bool that
// reports whether the user pressed OK.
func promptString(host *views.Group, title, prompt, initial string) (string, bool) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 48, 8), title)
	d.Insert(dialogs.NewStaticText(geom.NewRect(2, 2, 46, 3), prompt))
	il := dialogs.NewInputLine(geom.NewRect(2, 3, 46, 4), 256)
	il.Data = []rune(initial)
	il.CurPos = len(il.Data)
	d.Insert(il)
	d.Insert(dialogs.NewButton(geom.NewRect(20, 5, 30, 7), "O~K~", consts.CmOK, dialogs.BfDefault))
	d.Insert(dialogs.NewButton(geom.NewRect(31, 5, 41, 7), "Cancel", consts.CmCancel, dialogs.BfNormal))
	if host.ExecView(d) != consts.CmOK {
		return "", false
	}
	return string(il.Data), true
}

// PasteClipboard pastes the clipboard's TSV (or single line) into
// the grid starting at the focused cell. Empty / no clipboard is a
// no-op. Honors grid ReadOnly. Auto-expands the row count if the
// paste overflows past the end (non-virtual mode only).
func (g *StringGrid) PasteClipboard() {
	if g.ReadOnly {
		return
	}
	text := clipboard.GetText()
	if text == "" {
		return
	}
	// Strip trailing newline so a single-line copy doesn't insert an
	// extra blank row.
	for len(text) > 0 && (text[len(text)-1] == '\n' || text[len(text)-1] == '\r') {
		text = text[:len(text)-1]
	}
	lines := splitLines(text)
	startRow := g.Focus.Row
	startCol := g.Focus.Col
	for i, line := range lines {
		cols := splitTabs(line)
		for j, val := range cols {
			r := startRow + i
			c := startCol + j
			if c >= len(g.Columns) {
				break
			}
			if !g.Columns[c].Visible {
				continue
			}
			if g.OnGetCell == nil {
				for r >= len(g.rows) {
					g.rows = append(g.rows, make([]string, len(g.Columns)))
				}
			}
			g.SetCell(r, c, val)
		}
	}
	g.markDirty()
	g.refreshScroll()
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		if s[i] == '\r' {
			out = append(out, cur)
			cur = ""
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			continue
		}
		cur += string(s[i])
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func splitTabs(s string) []string {
	var out []string
	cur := ""
	for _, ch := range s {
		if ch == '\t' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(ch)
	}
	out = append(out, cur)
	return out
}

// showHeaderMenu pops a context menu for the given column at the
// click point. The menu items map to a small set of column-scoped
// commands; selection is applied immediately.
func (g *StringGrid) showHeaderMenu(col int, at geom.Point) {
	if g.Owner == nil {
		return
	}
	items := []string{
		"Sort ascending",
		"Sort descending",
		"Add to sort",
		"Clear sort",
		"Toggle filter row",
		"Auto-fit",
		"Auto-fit all",
		"Reset width",
		"Hide column",
	}
	pop := popupmenu.New(at, items, 30)
	pick := pop.Run(g.Owner)
	switch pick {
	case 0:
		g.Sort(col, SortAsc)
	case 1:
		g.Sort(col, SortDesc)
	case 2:
		g.AddSortKey(col)
	case 3:
		g.ClearSort()
	case 4:
		g.ShowFilter = !g.ShowFilter
	case 5:
		g.AutoFitColumn(col)
	case 6:
		g.AutoFitAll()
	case 7:
		g.ResetColumnWidth(col)
	case 8:
		g.ToggleColumnVisible(col)
	}
	g.markDirty()
}
