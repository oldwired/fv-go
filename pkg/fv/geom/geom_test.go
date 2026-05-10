package geom

import "testing"

func TestRectBasics(t *testing.T) {
	r := NewRect(2, 3, 12, 8)
	if r.Width() != 10 || r.Height() != 5 {
		t.Fatalf("width/height: got %dx%d want 10x5", r.Width(), r.Height())
	}
	if r.Empty() {
		t.Fatal("non-empty rect reported empty")
	}
	if (Rect{}).Empty() != true {
		t.Fatal("zero rect not reported empty")
	}
}

func TestRectContains(t *testing.T) {
	r := NewRect(0, 0, 10, 5)
	cases := []struct {
		p    Point
		want bool
	}{
		{Point{0, 0}, true},
		{Point{9, 4}, true},
		{Point{10, 4}, false}, // half-open on B
		{Point{9, 5}, false},
		{Point{-1, 0}, false},
	}
	for _, c := range cases {
		if r.Contains(c.p) != c.want {
			t.Errorf("Contains(%v) = %v, want %v", c.p, !c.want, c.want)
		}
	}
}

func TestRectMoveGrow(t *testing.T) {
	r := NewRect(2, 3, 12, 8).Move(1, -1)
	if !r.Equals(NewRect(3, 2, 13, 7)) {
		t.Fatalf("Move: got %v", r)
	}
	g := NewRect(5, 5, 10, 10).Grow(2, 1)
	if !g.Equals(NewRect(3, 4, 12, 11)) {
		t.Fatalf("Grow: got %v", g)
	}
}

func TestRectIntersect(t *testing.T) {
	a := NewRect(0, 0, 10, 10)
	b := NewRect(5, 5, 15, 15)
	got := a.Intersect(b)
	if !got.Equals(NewRect(5, 5, 10, 10)) {
		t.Fatalf("intersect overlap: got %v", got)
	}
	disjoint := a.Intersect(NewRect(20, 20, 30, 30))
	if !disjoint.Empty() {
		t.Fatalf("intersect disjoint: got %v want empty", disjoint)
	}
}

func TestRectUnion(t *testing.T) {
	got := NewRect(0, 0, 5, 5).Union(NewRect(3, 3, 8, 8))
	if !got.Equals(NewRect(0, 0, 8, 8)) {
		t.Fatalf("union: got %v", got)
	}
}

func TestPointAddSub(t *testing.T) {
	p := Point{2, 3}.Add(Point{4, -1})
	if p != (Point{6, 2}) {
		t.Fatalf("Add: got %v", p)
	}
	q := Point{2, 3}.Sub(Point{4, -1})
	if q != (Point{-2, 4}) {
		t.Fatalf("Sub: got %v", q)
	}
}
