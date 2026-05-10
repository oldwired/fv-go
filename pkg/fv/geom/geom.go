// Package geom provides Point and Rect — the integer-coordinate geometry
// types used everywhere in fv-go.
//
// Ported from Drivers.pas (TPoint / TRect). Coordinates are 0-based and
// rectangles use the half-open [A, B) convention: A is inclusive, B is
// exclusive — so a Rect from (0,0) to (10,5) covers ten columns and five
// rows. Empty when A.X >= B.X or A.Y >= B.Y. Methods mutate by value
// (returning a new Rect/Point) instead of mutating receivers, which is
// idiomatic Go and avoids the &-passing the Pascal API required.
package geom

// Point is a column/row pair.
type Point struct {
	X, Y int
}

// Add returns p+q componentwise.
func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }

// Sub returns p-q componentwise.
func (p Point) Sub(q Point) Point { return Point{p.X - q.X, p.Y - q.Y} }

// Rect is a half-open rectangle [A, B).
type Rect struct {
	A, B Point
}

// NewRect returns the rectangle with corners (xa,ya)-(xb,yb).
func NewRect(xa, ya, xb, yb int) Rect {
	return Rect{Point{xa, ya}, Point{xb, yb}}
}

// Width returns B.X - A.X.
func (r Rect) Width() int { return r.B.X - r.A.X }

// Height returns B.Y - A.Y.
func (r Rect) Height() int { return r.B.Y - r.A.Y }

// Empty reports whether the rectangle has no area.
func (r Rect) Empty() bool { return r.A.X >= r.B.X || r.A.Y >= r.B.Y }

// Equals reports whether r and s have identical corners.
func (r Rect) Equals(s Rect) bool { return r.A == s.A && r.B == s.B }

// Contains reports whether p lies inside r (half-open semantics).
func (r Rect) Contains(p Point) bool {
	return p.X >= r.A.X && p.X < r.B.X && p.Y >= r.A.Y && p.Y < r.B.Y
}

// Move returns r translated by (dx, dy).
func (r Rect) Move(dx, dy int) Rect {
	return Rect{
		Point{r.A.X + dx, r.A.Y + dy},
		Point{r.B.X + dx, r.B.Y + dy},
	}
}

// Grow returns r expanded by (dx, dy) on each side. Negative shrinks.
// If the result collapses to or past zero area, it is returned as-is —
// callers test with Empty().
func (r Rect) Grow(dx, dy int) Rect {
	return Rect{
		Point{r.A.X - dx, r.A.Y - dy},
		Point{r.B.X + dx, r.B.Y + dy},
	}
}

// Intersect returns the intersection of r and s. If they don't overlap,
// the result is the zero rectangle (matching TRect.Intersect semantics).
func (r Rect) Intersect(s Rect) Rect {
	out := r
	if s.A.X > out.A.X {
		out.A.X = s.A.X
	}
	if s.A.Y > out.A.Y {
		out.A.Y = s.A.Y
	}
	if s.B.X < out.B.X {
		out.B.X = s.B.X
	}
	if s.B.Y < out.B.Y {
		out.B.Y = s.B.Y
	}
	if out.Empty() {
		return Rect{}
	}
	return out
}

// Union returns the smallest rectangle containing both r and s.
func (r Rect) Union(s Rect) Rect {
	out := r
	if s.A.X < out.A.X {
		out.A.X = s.A.X
	}
	if s.A.Y < out.A.Y {
		out.A.Y = s.A.Y
	}
	if s.B.X > out.B.X {
		out.B.X = s.B.X
	}
	if s.B.Y > out.B.Y {
		out.B.Y = s.B.Y
	}
	return out
}
