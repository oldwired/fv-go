package grid

import (
	"bytes"
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// makeTestGrid builds a 4×3 grid for sort/filter/selection tests.
func makeTestGrid() *StringGrid {
	cols := []Column{
		{Title: "Name", Width: 10, Sortable: true},
		{Title: "Age", Width: 6, Sortable: true},
		{Title: "City", Width: 12, Sortable: true},
	}
	g := New(geom.NewRect(0, 0, 40, 10), cols, nil, nil)
	g.SetRows([][]string{
		{"Charlie", "30", "Berlin"},
		{"Alice", "25", "Munich"},
		{"Bob", "40", "Hamburg"},
		{"Dave", "35", "Berlin"},
	})
	return g
}

func TestSortAscDesc(t *testing.T) {
	g := makeTestGrid()
	g.Sort(0, SortAsc)
	want := []string{"Alice", "Bob", "Charlie", "Dave"}
	for i, w := range want {
		if got := g.Cell(i, 0); got != w {
			t.Errorf("asc row %d: got %q, want %q", i, got, w)
		}
	}
	g.Sort(0, SortDesc)
	for i, w := range []string{"Dave", "Charlie", "Bob", "Alice"} {
		if got := g.Cell(i, 0); got != w {
			t.Errorf("desc row %d: got %q, want %q", i, got, w)
		}
	}
}

func TestCycleSort(t *testing.T) {
	g := makeTestGrid()
	g.CycleSort(2) // City → asc
	if g.SortCol != 2 || g.SortDir != SortAsc {
		t.Fatalf("first cycle: col=%d dir=%d", g.SortCol, g.SortDir)
	}
	g.CycleSort(2) // → desc
	if g.SortCol != 2 || g.SortDir != SortDesc {
		t.Fatalf("second cycle: col=%d dir=%d", g.SortCol, g.SortDir)
	}
	g.CycleSort(2) // → unsorted
	if g.SortCol != -1 {
		t.Fatalf("third cycle should clear sort, got col=%d dir=%d", g.SortCol, g.SortDir)
	}
}

func TestFilter(t *testing.T) {
	g := makeTestGrid()
	g.SetFilter(2, "berlin") // case-insensitive
	want := []string{"Charlie", "Dave"}
	if g.RowCount() != 2 {
		t.Fatalf("filter expected 2 rows, got %d", g.RowCount())
	}
	for i, w := range want {
		if got := g.Cell(i, 0); got != w {
			t.Errorf("filter row %d: got %q, want %q", i, got, w)
		}
	}
	g.ClearFilters()
	if g.RowCount() != 4 {
		t.Fatalf("clear filters: expected 4 rows, got %d", g.RowCount())
	}
}

func TestSortAndFilter(t *testing.T) {
	g := makeTestGrid()
	g.SetFilter(2, "berlin")
	g.Sort(0, SortDesc)
	want := []string{"Dave", "Charlie"}
	for i, w := range want {
		if got := g.Cell(i, 0); got != w {
			t.Errorf("filter+sort row %d: got %q, want %q", i, got, w)
		}
	}
}

func TestSelectionExtend(t *testing.T) {
	g := makeTestGrid()
	g.MoveTo(0, 0)
	g.ExtendTo(1, 2)
	// 2x3 rect selected.
	if !g.inSelection(0, 0) || !g.inSelection(2, 1) {
		t.Errorf("corners should be selected")
	}
	if g.inSelection(3, 0) {
		t.Errorf("row 3 should not be selected")
	}
	if g.inSelection(0, 2) {
		t.Errorf("col 2 should not be selected")
	}
}

func TestCSVRoundTrip(t *testing.T) {
	g := makeTestGrid()
	var buf bytes.Buffer
	if err := g.SaveCSV(&buf, CSVOptions{IncludeHeader: true}); err != nil {
		t.Fatalf("save: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Name,Age,City") {
		t.Errorf("header missing in CSV output:\n%s", out)
	}
	if !strings.Contains(out, "Alice,25,Munich") {
		t.Errorf("data missing in CSV output:\n%s", out)
	}

	g2 := New(geom.NewRect(0, 0, 40, 10), []Column{
		{Title: "", Width: 10}, {Title: "", Width: 6}, {Title: "", Width: 12},
	}, nil, nil)
	if err := g2.LoadCSV(strings.NewReader(out), CSVOptions{IncludeHeader: true}); err != nil {
		t.Fatalf("load: %v", err)
	}
	if got, want := g2.RowCount(), 4; got != want {
		t.Errorf("after load: got %d rows, want %d", got, want)
	}
	if got, want := g2.Cell(1, 0), "Alice"; got != want {
		t.Errorf("row 1 col 0: got %q, want %q", got, want)
	}
}

func TestCSVQuoting(t *testing.T) {
	g := New(geom.NewRect(0, 0, 40, 10), []Column{
		{Title: "A", Width: 10}, {Title: "B", Width: 10},
	}, nil, nil)
	g.SetRows([][]string{
		{"with,comma", `with"quote`},
		{"with\nnewline", "plain"},
	})
	var buf bytes.Buffer
	if err := g.SaveCSV(&buf, CSVOptions{IncludeHeader: false}); err != nil {
		t.Fatalf("save: %v", err)
	}
	g2 := New(geom.NewRect(0, 0, 40, 10), []Column{
		{Title: "A", Width: 10}, {Title: "B", Width: 10},
	}, nil, nil)
	if err := g2.LoadCSV(&buf, CSVOptions{IncludeHeader: false}); err != nil {
		t.Fatalf("load: %v", err)
	}
	for r := 0; r < 2; r++ {
		for c := 0; c < 2; c++ {
			if a, b := g.Cell(r, c), g2.Cell(r, c); a != b {
				t.Errorf("[%d,%d] round-trip: %q → %q", r, c, a, b)
			}
		}
	}
}

func TestVirtualMode(t *testing.T) {
	cols := []Column{{Title: "Idx", Width: 6}, {Title: "Square", Width: 8}}
	g := New(geom.NewRect(0, 0, 30, 10), cols, nil, nil)
	g.VirtualRowCount = 1000
	g.OnGetCell = func(row, col int) string {
		if col == 0 {
			return itoa(row)
		}
		return itoa(row * row)
	}
	if got, want := g.RowCount(), 1000; got != want {
		t.Errorf("RowCount: got %d, want %d", got, want)
	}
	if got, want := g.Cell(7, 1), "49"; got != want {
		t.Errorf("Cell(7,1): got %q, want %q", got, want)
	}
}

// Tiny inline itoa to avoid pulling strconv into the test for one
// call — keeps the test file self-contained.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
