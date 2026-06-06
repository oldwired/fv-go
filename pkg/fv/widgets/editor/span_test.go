package editor

import "testing"

func TestAdjustPos(t *testing.T) {
	ins := Splice{Start: 5, OldLen: 0, NewLen: 3}  // insert 3 at 5
	del := Splice{Start: 5, OldLen: 4, NewLen: 0}  // delete [5,9)
	repl := Splice{Start: 5, OldLen: 4, NewLen: 2} // replace [5,9) with 2

	cases := []struct {
		name string
		p    int
		sp   Splice
		b    bias
		want int
	}{
		{"before insert", 3, ins, biasLeft, 3},
		{"before insert right", 3, ins, biasRight, 3},
		{"at insert left", 5, ins, biasLeft, 5},
		{"at insert right", 5, ins, biasRight, 8},
		{"after insert", 7, ins, biasLeft, 10},
		{"before delete", 4, del, biasLeft, 4},
		{"at delete start", 5, del, biasLeft, 5},
		{"at delete start right", 5, del, biasRight, 5},
		{"inside delete left", 7, del, biasLeft, 5},
		{"inside delete right", 7, del, biasRight, 5},
		{"at delete end", 9, del, biasLeft, 5},
		{"after delete", 12, del, biasLeft, 8},
		{"at replace start", 5, repl, biasLeft, 5},
		{"inside replace left", 7, repl, biasLeft, 5},
		{"inside replace right", 7, repl, biasRight, 7},
		{"at replace end", 9, repl, biasLeft, 7},
		{"after replace", 11, repl, biasLeft, 9},
	}
	for _, c := range cases {
		if got := adjustPos(c.p, c.sp, c.b); got != c.want {
			t.Errorf("%s: adjustPos(%d) = %d, want %d", c.name, c.p, got, c.want)
		}
	}
}

func TestAdjustCaret(t *testing.T) {
	cases := []struct {
		name string
		p    int
		sp   Splice
		want int
	}{
		{"insert at caret pushes right", 5, Splice{Start: 5, OldLen: 0, NewLen: 3}, 8},
		{"inside replace clamps to start", 7, Splice{Start: 5, OldLen: 4, NewLen: 2}, 5},
		{"at replace start stays", 5, Splice{Start: 5, OldLen: 4, NewLen: 2}, 5},
		{"at replace end follows", 9, Splice{Start: 5, OldLen: 4, NewLen: 2}, 7},
	}
	for _, c := range cases {
		if got := adjustCaret(c.p, c.sp); got != c.want {
			t.Errorf("%s: adjustCaret(%d) = %d, want %d", c.name, c.p, got, c.want)
		}
	}
}

func TestAdjustSpan(t *testing.T) {
	s := span{Start: 5, End: 10}
	cases := []struct {
		name   string
		sp     Splice
		grow   bool
		want   span
		wantOK bool
	}{
		{"insert at start grow", Splice{Start: 5, OldLen: 0, NewLen: 2}, true, span{5, 12}, true},
		{"insert at start nogrow", Splice{Start: 5, OldLen: 0, NewLen: 2}, false, span{7, 12}, true},
		{"insert at end grow", Splice{Start: 10, OldLen: 0, NewLen: 2}, true, span{5, 12}, true},
		{"insert at end nogrow", Splice{Start: 10, OldLen: 0, NewLen: 2}, false, span{5, 10}, true},
		{"insert inside", Splice{Start: 7, OldLen: 0, NewLen: 2}, false, span{5, 12}, true},
		{"delete left overlap", Splice{Start: 3, OldLen: 4, NewLen: 0}, false, span{3, 6}, true},
		{"delete right overlap", Splice{Start: 8, OldLen: 5, NewLen: 0}, false, span{5, 8}, true},
		{"delete exact cover grow", Splice{Start: 5, OldLen: 5, NewLen: 0}, true, span{5, 5}, true},
		{"delete exact cover nogrow", Splice{Start: 5, OldLen: 5, NewLen: 0}, false, span{5, 5}, false},
		{"delete full cover", Splice{Start: 3, OldLen: 10, NewLen: 0}, true, span{3, 3}, false},
		{"replace exact grow keeps content", Splice{Start: 5, OldLen: 5, NewLen: 3}, true, span{5, 8}, true},
		{"replace inside", Splice{Start: 6, OldLen: 2, NewLen: 5}, false, span{5, 13}, true},
		{"untouched before", Splice{Start: 20, OldLen: 1, NewLen: 0}, false, span{5, 10}, true},
		{"untouched after shifts", Splice{Start: 0, OldLen: 0, NewLen: 4}, false, span{9, 14}, true},
	}
	for _, c := range cases {
		got, ok := adjustSpan(s, c.sp, c.grow)
		if got != c.want || ok != c.wantOK {
			t.Errorf("%s: adjustSpan = (%v, %v), want (%v, %v)", c.name, got, ok, c.want, c.wantOK)
		}
	}
}

func TestAdjustSpanEmptySpanSurvives(t *testing.T) {
	// A zero-length grow span (empty tab stop) grows when typed into.
	s := span{Start: 5, End: 5}
	got, ok := adjustSpan(s, Splice{Start: 5, OldLen: 0, NewLen: 3}, true)
	if !ok || got != (span{5, 8}) {
		t.Errorf("empty grow span after insert = (%v, %v), want ({5 8}, true)", got, ok)
	}
	// A zero-length span survives a delete elsewhere.
	got, ok = adjustSpan(s, Splice{Start: 0, OldLen: 2, NewLen: 0}, true)
	if !ok || got != (span{3, 3}) {
		t.Errorf("empty span after delete above = (%v, %v), want ({3 3}, true)", got, ok)
	}
}
