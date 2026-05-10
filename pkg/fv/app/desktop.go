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
