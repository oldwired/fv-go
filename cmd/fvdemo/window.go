package main

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// windowCount tracks how many non-modal windows have been opened so we
// can give each one a number and cascade their initial position.
var windowCount int

// openNonModalWindow creates a new non-modal window on the desktop.
// Multiple windows can coexist; clicking one raises it to the top of
// the z-order, clicking [■] removes it, and the title bar drag works
// as in dialogs.
func openNonModalWindow(a *app.Application) {
	windowCount++
	n := windowCount

	// Cascade: each new window starts a few cells right and down from
	// the previous, wrapping if it would fall off the desktop.
	cols, rows := a.Desktop.Size.X, a.Desktop.Size.Y
	w, h := 40, 12
	x := 2 + ((n - 1) * 3 % (cols - w - 2))
	y := 1 + ((n - 1) * 2 % (rows - h - 2))
	bounds := geom.NewRect(x, y, x+w, y+h)

	win := views.NewWindow(bounds, fmt.Sprintf("Demo Window %d", n), n)

	// Body content: a static blurb plus a Close button.
	body := dialogs.NewStaticText(
		geom.NewRect(2, 2, w-2, h-3),
		fmt.Sprintf(
			"This is non-modal window #%d.\n\n"+
				"• Drag the title bar to move me.\n"+
				"• Click [■] to close.\n"+
				"• Open more from Test → New Window.",
			n),
	)
	win.Insert(body)

	a.Desktop.InsertWindow(win)
}
