package grid

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

func threeColGrid() *StringGrid {
	cols := []Column{
		{Title: "A", Width: 6},
		{Title: "B", Width: 6},
		{Title: "C", Width: 6},
	}
	return New(geom.NewRect(0, 0, 30, 10), cols, nil, nil)
}

// SetCell into a ragged physical row (shorter than len(Columns)) must
// pad the row rather than panic. Regression for the out-of-visible-range
// branch that indexed g.rows[row][col] without padding columns.
func TestSetCellRaggedRowNoPanic(t *testing.T) {
	g := threeColGrid()
	// Row 1 is ragged (2 of 3 columns); rows 0/2 are full.
	g.SetRows([][]string{
		{"keep", "1", "x"},
		{"hide", "2"},
		{"hide2", "3", "y"},
	})
	// Filter so only row 0 is visible → visibleRows == [0], len 1.
	g.SetFilter(0, "keep")
	g.ensureVisible()
	if got := len(g.visibleRows); got != 1 {
		t.Fatalf("setup: visibleRows len = %d, want 1", got)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetCell out-of-visible-range panicked: %v", r)
		}
	}()
	// row 1 is out of visible range; col 2 exceeds the ragged hidden
	// row 1's width — must not panic and must not corrupt hidden row 1.
	g.SetCell(1, 2, "boom")

	if g.rows[1][0] != "hide" || g.rows[1][1] != "2" || len(g.rows[1]) != 2 {
		t.Errorf("hidden physical row 1 was corrupted: %#v", g.rows[1])
	}
}

// With an active filter the in-range branch must map the visible index
// through visibleRows to the correct physical row, never the raw index.
func TestSetCellFilteredMapsToPhysicalRow(t *testing.T) {
	g := threeColGrid()
	g.SetRows([][]string{
		{"a", "1", ""},
		{"b", "2", ""},
		{"a", "3", ""},
	})
	g.SetFilter(0, "a") // visible: physical rows 0 and 2
	g.ensureVisible()
	if len(g.visibleRows) != 2 || g.visibleRows[1] != 2 {
		t.Fatalf("setup: visibleRows = %v, want [0 2]", g.visibleRows)
	}
	// Edit visible row 1 → physical row 2, NOT physical row 1.
	g.SetCell(1, 2, "edited")
	if g.rows[2][2] != "edited" {
		t.Errorf("physical row 2 col 2 = %q, want %q", g.rows[2][2], "edited")
	}
	if g.rows[1][2] != "" {
		t.Errorf("physical row 1 col 2 = %q, should be untouched", g.rows[1][2])
	}
}

// swapColumns must remap every multi-column SortKey (and the SortCol
// mirror) so the active sort keeps pointing at the same logical columns
// after a header-drag reorder.
func TestSwapColumnsRemapsSortKeys(t *testing.T) {
	g := threeColGrid()
	g.SortKeys = []SortKey{{Col: 2, Dir: SortAsc}, {Col: 0, Dir: SortDesc}}
	g.syncPrimarySort()
	if g.SortCol != 2 {
		t.Fatalf("setup: SortCol = %d, want 2", g.SortCol)
	}

	// Move column 0 to position 2: order [A,B,C] -> [B,C,A].
	// Original col 2 (C) is now at index 1; original col 0 (A) at index 2.
	g.swapColumns(0, 2)

	want := []SortKey{{Col: 1, Dir: SortAsc}, {Col: 2, Dir: SortDesc}}
	for i, w := range want {
		if g.SortKeys[i].Col != w.Col || g.SortKeys[i].Dir != w.Dir {
			t.Errorf("SortKeys[%d] = %+v, want %+v", i, g.SortKeys[i], w)
		}
	}
	if g.SortCol != 1 {
		t.Errorf("SortCol mirror = %d, want 1", g.SortCol)
	}
}

func TestRemapColumnIndex(t *testing.T) {
	cases := []struct{ idx, from, to, want int }{
		{0, 0, 2, 2}, // moved element
		{1, 0, 2, 0},
		{2, 0, 2, 1},
		{3, 0, 2, 3}, // beyond the moved range, unchanged
		{2, 2, 0, 0}, // moved element (backwards)
		{0, 2, 0, 1},
		{1, 2, 0, 2},
		{3, 2, 0, 3}, // unchanged
	}
	for _, c := range cases {
		if got := remapColumnIndex(c.idx, c.from, c.to); got != c.want {
			t.Errorf("remapColumnIndex(%d,%d,%d) = %d, want %d", c.idx, c.from, c.to, got, c.want)
		}
	}
}
