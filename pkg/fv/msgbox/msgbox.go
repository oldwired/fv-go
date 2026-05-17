// Package msgbox ports MsgBox.pas: simple modal information / question /
// confirmation popups that block until the user clicks a button.
package msgbox

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// MessageType selects the icon and default title.
type MessageType int

const (
	Info MessageType = iota
	Warning
	Error
	Question
)

// MessageBox flags.
const (
	YesNo       = 1
	OKCancel    = 2
	YesNoCancel = 3
	OKOnly      = 4
)

// Show displays a message box centered on the desktop and returns the
// command code that closed it (cmYes, cmNo, cmOK, cmCancel).
//
// desktop is the views.Group that the dialog is inserted into for the
// modal loop. Typically *app.Application.Desktop.
func Show(desktop *views.Group, mt MessageType, msg string, buttons int) uint16 {
	title := titleFor(mt)
	d := buildDialog(title, msg, buttons)
	return desktop.ExecView(d)
}

// Showf is Show with sprintf-style formatting.
func Showf(desktop *views.Group, mt MessageType, format string, args []any, buttons int) uint16 {
	return Show(desktop, mt, fmt.Sprintf(format, args...), buttons)
}

func titleFor(mt MessageType) string {
	switch mt {
	case Warning:
		return "Warning"
	case Error:
		return "Error"
	case Question:
		return "Confirm"
	case Info:
		return "Information"
	}
	return "Information"
}

func buildDialog(title, msg string, buttons int) *dialogs.Dialog {
	w := 50
	h := 8
	bounds := geom.NewRect(0, 0, w, h)
	d := dialogs.NewDialog(bounds, title)

	textBounds := geom.NewRect(2, 2, w-2, h-3)
	d.Insert(dialogs.NewStaticText(textBounds, msg))

	addButtons(d, w, h, buttons)
	return d
}

func addButtons(d *dialogs.Dialog, w, h, buttons int) {
	bw := 10
	by := h - 2
	switch buttons {
	case YesNo:
		d.Insert(dialogs.NewButton(geom.NewRect(w/2-bw-1, by, w/2-1, by+1), "~Y~es", consts.CmYes, dialogs.BfDefault))
		d.Insert(dialogs.NewButton(geom.NewRect(w/2+1, by, w/2+bw+1, by+1), "~N~o", consts.CmNo, 0))
	case OKCancel:
		d.Insert(dialogs.NewButton(geom.NewRect(w/2-bw-1, by, w/2-1, by+1), "O~K~", consts.CmOK, dialogs.BfDefault))
		d.Insert(dialogs.NewButton(geom.NewRect(w/2+1, by, w/2+bw+1, by+1), "Cancel", consts.CmCancel, 0))
	case YesNoCancel:
		d.Insert(dialogs.NewButton(geom.NewRect(2, by, 12, by+1), "~Y~es", consts.CmYes, dialogs.BfDefault))
		d.Insert(dialogs.NewButton(geom.NewRect(14, by, 24, by+1), "~N~o", consts.CmNo, 0))
		d.Insert(dialogs.NewButton(geom.NewRect(26, by, 36, by+1), "Cancel", consts.CmCancel, 0))
	default:
		d.Insert(dialogs.NewButton(geom.NewRect(w/2-bw/2, by, w/2+bw/2, by+1), "O~K~", consts.CmOK, dialogs.BfDefault))
	}
}
