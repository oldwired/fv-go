package term_test

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/types"
)

// Headless is the in-memory Backend used by tests. It captures cell
// writes and exposes Snapshot() so a test can assert on rendered
// output without touching a real tty.
func ExampleNewHeadless() {
	h := term.NewHeadless(20, 3)
	h.SetCell(0, 0, types.DrawCell{Ch: "h"})
	h.SetCell(1, 0, types.DrawCell{Ch: "i"})
	cols, rows := h.Size()
	fmt.Printf("viewport: %dx%d\n", cols, rows)
	fmt.Printf("first line: %q\n", firstLine(h.Snapshot()))
	// Output:
	// viewport: 20x3
	// first line: "hi"
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
