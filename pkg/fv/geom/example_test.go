package geom_test

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// Two overlapping rectangles, the part they share, and a point check
// against the result.
func ExampleRect_Intersect() {
	a := geom.NewRect(0, 0, 10, 5)
	b := geom.NewRect(4, 2, 14, 8)

	share := a.Intersect(b)
	fmt.Printf("intersect: A=%v B=%v\n", share.A, share.B)
	fmt.Println("contains (5,3):", share.Contains(geom.Point{X: 5, Y: 3}))
	fmt.Println("contains (1,1):", share.Contains(geom.Point{X: 1, Y: 1}))
	// Output:
	// intersect: A={4 2} B={10 5}
	// contains (5,3): true
	// contains (1,1): false
}
