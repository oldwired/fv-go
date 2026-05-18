// wholewin renders the Unicode-demo window into a headless backend
// and prints every cell of the Emoji row plus a few cells of the
// surrounding rows. This isolates "what does fv-go put in each cell"
// from "what does the terminal do with those cells when it renders
// them."
package main

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

func main() {
	const W, H = 80, 24
	h := term.NewHeadless(W, H)
	views.SetRootBackend(h)

	// Mimic the unicode showcase demo verbatim.
	win := views.NewWindow(geom.NewRect(4, 2, 4+64, 2+16), "Unicode",
		int(consts.WfMove|consts.WfClose))
	w, _ := win.Size.X, win.Size.Y

	lines := []string{
		"ASCII:    The quick brown fox jumps over the lazy dog.",
		"Latin-1:  café crème brûlée naïve résumé Zürich",
		"Greek:    Ζεύς απαθηνής λοιπóν με τους θεούς",
		"Hebrew:   שלום עולם — right-to-left",
		"CJK:      你好世界 — 日本語 — 한국어",
		"Emoji:    🍎🍊🍋  👨‍👩‍👧‍👦  🏳️‍🌈  ✨🚀💫",
		"Combine:  a + ̈ = ä  e + ́ = é  o + ̃ = õ",
	}
	for i, line := range lines {
		win.Insert(dialogs.NewStaticText(
			geom.NewRect(2, 1+i, w-2, 2+i), line))
	}
	win.State |= consts.SfVisible | consts.SfExposed
	win.Draw()
	_ = h.Flush()

	// Emoji row is window-local row 6, i.e. global row 2+6 = 8.
	// Window spans global cols 4..(4+64) = 4..68.
	fmt.Println("EMOJI ROW (global row 8, cols 4..68):")
	dumpRow(h, 8, 4, 68)

	fmt.Println()
	fmt.Println("CJK ROW (global row 7, cols 4..68):")
	dumpRow(h, 7, 4, 68)
}

func dumpRow(h *term.Headless, y, fromX, toX int) {
	for x := fromX; x < toX; x++ {
		c := h.GetCell(x, y)
		var ch string
		if c.Ch == "" {
			ch = "<cont>"
		} else if c.Ch == " " {
			ch = "<spc>"
		} else {
			ch = fmt.Sprintf("%q", c.Ch)
		}
		fmt.Printf("  col %2d: %-12s\n", x, ch)
	}
}
