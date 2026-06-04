// Package dialogs ports Dialogs.pas: Dialog, Button, InputLine, Label,
// StaticText, ParamText, Cluster (CheckBoxes/RadioButtons), ListBox,
// StringListBox, History.
//
// All dialog widgets descend from views.Base (or views.Group for those
// that contain children, like Dialog itself).
package dialogs

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Dialog is a modal Window with default-button + Esc cancel handling.
type Dialog struct {
	views.Window
}

// NewDialog builds a Dialog with title.
func NewDialog(bounds geom.Rect, title string) *Dialog {
	d := &Dialog{}
	InitDialog(d, bounds, title)
	return d
}

// InitDialog initializes d in place. Use this when embedding Dialog
// by value in a larger type — copying the result of NewDialog would
// orphan the inserted Frame's Owner pointer (it would still point to
// the temporary Dialog NewDialog allocated).
func InitDialog(d *Dialog, bounds geom.Rect, title string) {
	views.InitWindow(&d.Window, bounds, title, consts.WnNoNumber)
	d.SetSelf(d)
	d.Options |= consts.OfCentered
	// A modal dialog is movable and closable but not resizable or
	// zoomable — Turbo Vision's TDialog uses wfMove+wfClose. InitWindow
	// seeds the full window flag set (it can't tell it's building a
	// dialog), so drop WfGrow/WfZoom here. Without this the resize handle
	// is live but the fixed-offset children (buttons, pickers) don't
	// reposition or clip, so shrinking the frame leaves them drawn past
	// the border. Apps that want a resizable dialog-like window subclass
	// views.Window directly and declare SizeLimits (see fvmux's browser).
	d.SetFlags(consts.WfMove | consts.WfClose)
	// Dialogs don't grow with the desktop — they keep their constructed
	// size unless the caller explicitly resizes.
	d.GrowMode = 0
}

// GetTypeID for serial registry.
func (d *Dialog) GetTypeID() string { return "dialog" }

// HandleEvent extends Window with Esc -> cmCancel and Enter -> default.
func (d *Dialog) HandleEvent(ev *drivers.Event) {
	d.Window.HandleEvent(ev)
	if ev.What == consts.EvKeyDown {
		switch ev.KeyCode {
		case consts.KbEsc:
			notify := drivers.Event{What: consts.EvCommand, Command: consts.CmCancel}
			d.PutEvent(&notify)
			d.ClearEvent(ev)
		case consts.KbEnter:
			notify := drivers.Event{What: consts.EvBroadcast, Command: consts.CmDefault}
			d.PutEvent(&notify)
			d.ClearEvent(ev)
		}
	}
	if ev.What == consts.EvCommand {
		switch ev.Command {
		case consts.CmOK, consts.CmCancel, consts.CmYes, consts.CmNo:
			d.EndModal(ev.Command)
			d.ClearEvent(ev)
		}
	}
}
