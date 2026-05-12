package app

import (
	"encoding/json"
	"io"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// desktopSnapshot is the on-disk shape produced by SaveDesktop. Each
// entry captures the bare framing of a window — type ID, bounds,
// title, flags. Inner content isn't serialized: widgets that want to
// participate must implement their own JSON via the serial registry
// and have the window pull from it. For now this is enough to restore
// window layout across a quit / reopen, which is the headline use.
type desktopSnapshot struct {
	Version int              `json:"version"`
	Windows []windowSnapshot `json:"windows"`
}

type windowSnapshot struct {
	TypeID string    `json:"type"`
	Title  string    `json:"title"`
	Bounds geom.Rect `json:"bounds"`
	Flags  byte      `json:"flags,omitempty"`
}

// SaveDesktop writes the current desktop's windows to w as JSON.
// The format is intentionally minimal so adding fields later doesn't
// break older snapshots: unknown JSON keys are tolerated by Go's
// decoder, and missing keys leave their fields at their zero value.
func (a *Application) SaveDesktop(w io.Writer) error {
	snap := desktopSnapshot{Version: 1}
	for _, c := range a.Desktop.Children {
		win, ok := c.(*views.Window)
		if !ok {
			continue
		}
		bv := win.BaseView()
		snap.Windows = append(snap.Windows, windowSnapshot{
			TypeID: win.GetTypeID(),
			Title:  win.Title(),
			Bounds: geom.NewRect(bv.Origin.X, bv.Origin.Y,
				bv.Origin.X+bv.Size.X, bv.Origin.Y+bv.Size.Y),
			Flags: win.Flags(),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(snap)
}

// LoadDesktop replaces the current windows with those described by
// the snapshot at r. Closes existing windows first. Restored windows
// are bare — empty `Window` instances at the saved bounds + title;
// widget content is not preserved (this is a layout-restore, not a
// session-restore). Apps that want full session restore can use this
// as the framing and rehydrate inner content from their own state.
func (a *Application) LoadDesktop(r io.Reader) error {
	var snap desktopSnapshot
	if err := json.NewDecoder(r).Decode(&snap); err != nil {
		return err
	}
	// Tear down existing windows.
	for i := len(a.Desktop.Children) - 1; i >= 0; i-- {
		c := a.Desktop.Children[i]
		if _, ok := c.(*views.Window); ok {
			a.Desktop.Delete(c)
		}
	}
	// Recreate.
	for _, ws := range snap.Windows {
		win := views.NewWindow(ws.Bounds, ws.Title, int(ws.Flags))
		a.Desktop.InsertWindow(win)
	}
	return nil
}
