// dump renders the demo's emoji line into a headless cellbuf and
// prints what each cell ends up holding. Compare the cellbuf cell
// positions against what your terminal actually renders to confirm
// (or rule out) drift between our cellbuf state and the terminal's
// cursor advance.
package main

import (
	"fmt"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/screen"
)

func main() {
	const width = 60
	buf := screen.MakeDrawBuffer(width)
	for x := 0; x < width; x++ {
		screen.DrawCell(buf, x, " ", 0)
	}
	line := "Emoji:    🍎🍊🍋  👨‍👩‍👧‍👦  🏳️‍🌈  ✨🚀💫"
	screen.DrawStr(buf, 0, line, 0)

	for i, c := range buf {
		marker := ""
		if c.Ch == "" {
			marker = " <continuation>"
		} else if c.Ch == " " {
			marker = " <space>"
		}
		fmt.Printf("col %2d: %s\n", i, asciiSafe(c.Ch)+marker)
	}
}

func asciiSafe(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r < 0x20 {
			sb.WriteString(fmt.Sprintf("\\u%04X", r))
		} else if r == 0x200D {
			sb.WriteString("\\u200D")
		} else if r == 0xFE0F {
			sb.WriteString("\\uFE0F")
		} else {
			sb.WriteRune(r)
		}
	}
	return fmt.Sprintf("%-30q", sb.String())
}
