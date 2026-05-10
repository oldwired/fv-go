// Package grid provides StringGrid — a terminal-mode data grid with
// rows, named columns, cell editing, mouse navigation, and per-column
// alignment.
//
// Ported from Grid.pas (TStringGrid). The Pascal version has a richer
// feature set: change log + undo, sortable columns, complex selection
// modes, CSV import/export, custom cell renderers, validation events.
// This port is the working core: enough to display tabular data,
// navigate by mouse / keyboard, and edit cells in place. The omitted
// features can layer on top without changing this surface.
package grid

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Alignment picks how a cell's text aligns within its column.
type Alignment int

const (
	AlignLeft Alignment = iota
	AlignCenter
	AlignRight
)

// Column describes one column.
type Column struct {
	Title    string
	Width    int
	Align    Alignment
	ReadOnly bool
}

// Cell coords. (0, 0) is the top-left data cell — the header row /
// column don't have addresses in this scheme.
type Cell struct{ Col, Row int }

// StringGrid is a 2D grid of text cells.
type StringGrid struct {
	views.Base

	Columns []Column
	rows    [][]string // rows[row][col]

	Focused   Cell
	Top       int // first visible row
	LeftCol   int // first visible column
	HasHeader bool

	// Edit mode.
	editing bool
	editBuf []rune
	editPos int

	HScroll *views.ScrollBar
	VScroll *views.ScrollBar
}

// New constructs an empty grid with the given columns.
func New(bounds geom.Rect, cols []Column, h, v *views.ScrollBar) *StringGrid {
	g := &StringGrid{
		Base:      views.NewBase(bounds),
		Columns:   cols,
		HasHeader: true,
		HScroll:   h,
		VScroll:   v,
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
	if g.Focused.Row >= len(rows) {
		g.Focused.Row = len(rows) - 1
	}
	if g.Focused.Row < 0 {
		g.Focused.Row = 0
	}
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
	g.refreshScroll()
}

// Cell returns the (row, col) value, or "" if out of range.
func (g *StringGrid) Cell(row, col int) string {
	if row < 0 || row >= len(g.rows) {
		return ""
	}
	if col < 0 || col >= len(g.rows[row]) {
		return ""
	}
	return g.rows[row][col]
}

// SetCell updates the (row, col) value.
func (g *StringGrid) SetCell(row, col int, value string) {
	for row >= len(g.rows) {
		g.rows = append(g.rows, make([]string, len(g.Columns)))
	}
	for col >= len(g.rows[row]) {
		g.rows[row] = append(g.rows[row], "")
	}
	g.rows[row][col] = value
}

// RowCount / ColCount accessors.
func (g *StringGrid) RowCount() int { return len(g.rows) }
func (g *StringGrid) ColCount() int { return len(g.Columns) }

// columnX returns the screen X position of column c relative to the
// grid's left edge, accounting for LeftCol scroll.
func (g *StringGrid) columnX(c int) int {
	x := 0
	for i := g.LeftCol; i < c; i++ {
		if i >= 0 && i < len(g.Columns) {
			x += g.Columns[i].Width
		}
	}
	return x
}

// columnAt returns the column index containing screen-relative x, or -1.
func (g *StringGrid) columnAt(x int) int {
	cur := 0
	for i := g.LeftCol; i < len(g.Columns); i++ {
		w := g.Columns[i].Width
		if x >= cur && x < cur+w {
			return i
		}
		cur += w
	}
	return -1
}

// dataRows returns how many rows the body can show.
func (g *StringGrid) dataRows() int {
	if g.HasHeader {
		return g.Size.Y - 1
	}
	return g.Size.Y
}

// MoveTo sets focused cell to (col, row), clamping and scrolling.
func (g *StringGrid) MoveTo(col, row int) {
	if g.editing {
		g.commitEdit()
	}
	if row < 0 {
		row = 0
	}
	if row >= len(g.rows) {
		row = len(g.rows) - 1
	}
	if col < 0 {
		col = 0
	}
	if col >= len(g.Columns) {
		col = len(g.Columns) - 1
	}
	if row < 0 {
		row = 0
	}
	if col < 0 {
		col = 0
	}
	g.Focused = Cell{Col: col, Row: row}
	g.adjustScroll()
}

// adjustScroll keeps the focused cell visible.
func (g *StringGrid) adjustScroll() {
	rows := g.dataRows()
	if g.Focused.Row < g.Top {
		g.Top = g.Focused.Row
	} else if g.Focused.Row >= g.Top+rows {
		g.Top = g.Focused.Row - rows + 1
	}
	if g.Focused.Col < g.LeftCol {
		g.LeftCol = g.Focused.Col
	}
	// Right-edge horizontal scroll: ensure focused col fits.
	for g.LeftCol < g.Focused.Col && g.columnX(g.Focused.Col)+columnWidth(g.Columns, g.Focused.Col) > g.Size.X {
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
		g.VScroll.SetRange(0, len(g.rows))
		g.VScroll.SetValue(g.Top)
	}
	if g.HScroll != nil {
		g.HScroll.SetRange(0, len(g.Columns))
		g.HScroll.SetValue(g.LeftCol)
	}
}

// Draw paints header (if any) + visible rows.
func (g *StringGrid) Draw() {
	headerColor := types.MakeAttr(0x0F, 0x04)
	cellColor := types.MakeAttr(0x07, 0x01)
	focusColor := types.MakeAttr(0x0F, 0x06)
	editColor := types.MakeAttr(0x0F, 0x02)

	rowOff := 0
	if g.HasHeader {
		buf := screen.MakeDrawBuffer(g.Size.X)
		for x := 0; x < g.Size.X; x++ {
			screen.DrawCell(buf, x, " ", headerColor)
		}
		x := 0
		for c := g.LeftCol; c < len(g.Columns); c++ {
			col := g.Columns[c]
			label := truncate(col.Title, col.Width)
			pad := alignText(label, col.Width, col.Align)
			for i, ch := range pad {
				if x+i < g.Size.X {
					buf[x+i] = types.DrawCell{Ch: string(ch), Attr: headerColor}
				}
			}
			x += col.Width
			if x >= g.Size.X {
				break
			}
		}
		g.WriteLine(0, 0, g.Size.X, 1, buf)
		rowOff = 1
	}
	for r := 0; r < g.dataRows(); r++ {
		buf := screen.MakeDrawBuffer(g.Size.X)
		for x := 0; x < g.Size.X; x++ {
			screen.DrawCell(buf, x, " ", cellColor)
		}
		rowIdx := g.Top + r
		if rowIdx >= len(g.rows) {
			g.WriteLine(0, rowOff+r, g.Size.X, 1, buf)
			continue
		}
		x := 0
		for c := g.LeftCol; c < len(g.Columns); c++ {
			col := g.Columns[c]
			text := g.Cell(rowIdx, c)
			cellAttr := cellColor
			if rowIdx == g.Focused.Row && c == g.Focused.Col {
				cellAttr = focusColor
				if g.editing {
					cellAttr = editColor
					text = string(g.editBuf)
				}
			}
			// Fill column cells with the chosen color.
			for i := 0; i < col.Width && x+i < g.Size.X; i++ {
				buf[x+i] = types.DrawCell{Ch: " ", Attr: cellAttr}
			}
			pad := alignText(truncate(text, col.Width-1), col.Width, col.Align)
			for i, ch := range pad {
				if x+i < g.Size.X {
					buf[x+i] = types.DrawCell{Ch: string(ch), Attr: cellAttr}
				}
			}
			x += col.Width
			if x >= g.Size.X {
				break
			}
		}
		g.WriteLine(0, rowOff+r, g.Size.X, 1, buf)
	}
	// Caret.
	if g.editing {
		x := g.columnX(g.Focused.Col) + g.editPos
		y := g.Focused.Row - g.Top + rowOff
		g.Cursor = geom.Point{X: x, Y: y}
	}
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

// HandleEvent: navigation, mouse focus, F2/Enter to start edit, in-place editing.
func (g *StringGrid) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		local := g.MakeLocal(ev.Where)
		rowOff := 0
		if g.HasHeader {
			rowOff = 1
		}
		if local.Y >= rowOff {
			row := g.Top + local.Y - rowOff
			col := g.columnAt(local.X)
			if col >= 0 && row >= 0 && row < len(g.rows) {
				g.MoveTo(col, row)
				g.Draw()
			}
		}
		if g.Owner != nil {
			g.Owner.Focus(g.Self())
		}
		g.ClearEvent(ev)
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	if g.editing {
		g.handleEditKey(ev)
		return
	}
	switch ev.KeyCode {
	case consts.KbLeft:
		g.MoveTo(g.Focused.Col-1, g.Focused.Row)
	case consts.KbRight:
		g.MoveTo(g.Focused.Col+1, g.Focused.Row)
	case consts.KbUp:
		g.MoveTo(g.Focused.Col, g.Focused.Row-1)
	case consts.KbDown:
		g.MoveTo(g.Focused.Col, g.Focused.Row+1)
	case consts.KbHome:
		g.MoveTo(0, g.Focused.Row)
	case consts.KbEnd:
		g.MoveTo(len(g.Columns)-1, g.Focused.Row)
	case consts.KbCtrlHome:
		g.MoveTo(0, 0)
	case consts.KbCtrlEnd:
		g.MoveTo(len(g.Columns)-1, len(g.rows)-1)
	case consts.KbPgUp:
		g.MoveTo(g.Focused.Col, g.Focused.Row-g.dataRows())
	case consts.KbPgDn:
		g.MoveTo(g.Focused.Col, g.Focused.Row+g.dataRows())
	case consts.KbEnter, consts.KbF2:
		g.beginEdit()
	default:
		// Any printable starts edit + replaces cell with that char.
		if ev.UnicodeChar >= ' ' {
			g.beginEdit()
			g.editBuf = []rune{ev.UnicodeChar}
			g.editPos = 1
		} else {
			return
		}
	}
	g.Draw()
	g.ClearEvent(ev)
}

// beginEdit puts the focused cell into edit mode.
func (g *StringGrid) beginEdit() {
	if g.Focused.Col < 0 || g.Focused.Col >= len(g.Columns) {
		return
	}
	if g.Columns[g.Focused.Col].ReadOnly {
		return
	}
	g.editing = true
	g.editBuf = []rune(g.Cell(g.Focused.Row, g.Focused.Col))
	g.editPos = len(g.editBuf)
}

// commitEdit writes the edit buffer back to the cell and exits edit mode.
func (g *StringGrid) commitEdit() {
	if !g.editing {
		return
	}
	g.SetCell(g.Focused.Row, g.Focused.Col, string(g.editBuf))
	g.editing = false
	g.editBuf = nil
}

// handleEditKey processes keystrokes while a cell is in edit mode.
func (g *StringGrid) handleEditKey(ev *drivers.Event) {
	switch ev.KeyCode {
	case consts.KbEsc:
		g.editing = false
		g.editBuf = nil
	case consts.KbEnter:
		g.commitEdit()
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
		}
	case consts.KbDel:
		if g.editPos < len(g.editBuf) {
			g.editBuf = append(g.editBuf[:g.editPos], g.editBuf[g.editPos+1:]...)
		}
	default:
		if ev.UnicodeChar >= ' ' {
			g.editBuf = append(g.editBuf[:g.editPos], append([]rune{ev.UnicodeChar}, g.editBuf[g.editPos:]...)...)
			g.editPos++
		} else {
			return
		}
	}
	g.Draw()
	g.ClearEvent(ev)
}
