package dialogs_test

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// TestDialogNotResizable locks in Turbo Vision's TDialog flag set
// (wfMove+wfClose): a modal dialog must not carry the grow/zoom bits.
// With them set, the live resize handle shrinks the frame while the
// dialog's fixed-offset children (buttons, the color/calendar/ascii
// pickers) stay drawn past the border — the framework does no
// parent-bounds clipping. Apps that want a resizable dialog-like window
// subclass views.Window and declare SizeLimits instead.
func TestDialogNotResizable(t *testing.T) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 40, 14), "x")
	flags := d.Flags()
	if flags&consts.WfGrow != 0 {
		t.Error("dialog has WfGrow set; modal dialogs must not be resizable")
	}
	if flags&consts.WfZoom != 0 {
		t.Error("dialog has WfZoom set; modal dialogs must not be zoomable")
	}
	if flags&consts.WfMove == 0 || flags&consts.WfClose == 0 {
		t.Errorf("dialog should keep WfMove|WfClose, got flags=%#x", flags)
	}
}
