package app

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Desktop is the area between the menu bar and the status line where
// windows live. The first child is the wallpaper Background; subsequent
// children are user-created windows / dialogs.
type Desktop struct {
	views.Group

	Background *views.Background
}

// NewDesktop builds a Desktop with the classic blue '▒' wallpaper.
func NewDesktop(bounds geom.Rect) *Desktop {
	d := &Desktop{}
	views.InitGroup(&d.Group, bounds)
	d.SetSelf(d)
	d.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	d.Background = views.NewBackground(geom.Rect{B: d.Size}, '▒')
	d.Background.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	d.Insert(d.Background)
	return d
}

// GetTypeID for serial registry.
func (d *Desktop) GetTypeID() string { return "desktop" }

// InsertWindow places w on the desktop and gives it focus.
func (d *Desktop) InsertWindow(w views.View) {
	d.Insert(w)
	d.Focus(w)
}

// InsertWindowPassive places w on the desktop without changing focus.
// Used for decorative or background windows (mascots, status overlays,
// watchlists) that should appear on top of the wallpaper but should
// NOT steal keyboard focus from whichever window the user is editing
// in.
//
// Z-order still puts w in front of any earlier insertions because new
// children are appended to the tail of the children list, which the
// dispatcher and renderer treat as the topmost layer.
func (d *Desktop) InsertWindowPassive(w views.View) {
	d.InsertPassive(w)
}
