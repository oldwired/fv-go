// Package timeddlg provides TimedDialog — a Dialog that closes itself
// after a configurable timeout, and TimedDialogText for displaying the
// remaining seconds inside.
package timeddlg

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// TimedDialog is a Dialog that calls EndModal(cmCancel) once Timeout
// has elapsed since it was shown.
type TimedDialog struct {
	dialogs.Dialog

	Timeout   time.Duration
	OnTimeout uint16 // command to fire on timeout (default cmCancel)
	expiresAt time.Time
	remaining *TimedDialogText
}

// New constructs a TimedDialog with the given timeout.
func New(bounds geom.Rect, title string, timeout time.Duration) *TimedDialog {
	d := &TimedDialog{
		Timeout:   timeout,
		OnTimeout: consts.CmCancel,
		expiresAt: time.Now().Add(timeout),
	}
	dialogs.InitDialog(&d.Dialog, bounds, title)
	d.SetSelf(d)
	anim.Register(d, 250*time.Millisecond)
	return d
}

// GetTypeID for serial registry.
func (d *TimedDialog) GetTypeID() string { return "timeddialog" }

// AttachCountdown wires a TimedDialogText so it updates each tick.
func (d *TimedDialog) AttachCountdown(t *TimedDialogText) {
	d.remaining = t
}

// Tick is the anim callback. Updates the countdown view (if any) and
// fires the timeout command when the deadline passes.
func (d *TimedDialog) Tick(now time.Time) bool {
	if d.remaining != nil {
		left := d.expiresAt.Sub(now)
		if left < 0 {
			left = 0
		}
		d.remaining.SetSeconds(int(left / time.Second))
	}
	if !now.Before(d.expiresAt) {
		anim.Unregister(d)
		d.EndModal(d.OnTimeout)
		return true
	}
	return d.remaining != nil
}

// TimedDialogText is a StaticText that renders "Closing in N seconds…"
// — fed by TimedDialog.Tick.
type TimedDialogText struct {
	dialogs.StaticText
	Format string
	secs   int
}

// NewText constructs a TimedDialogText. Format is a fmt.Sprintf
// pattern with one %d placeholder (e.g. "Closing in %d…").
func NewText(bounds geom.Rect, format string) *TimedDialogText {
	t := &TimedDialogText{
		StaticText: *dialogs.NewStaticText(bounds, ""),
		Format:     format,
	}
	t.SetSelf(t)
	t.SetSeconds(0)
	return t
}

// GetTypeID for serial registry.
func (t *TimedDialogText) GetTypeID() string { return "timeddialogtext" }

// SetSeconds updates the displayed countdown.
func (t *TimedDialogText) SetSeconds(s int) {
	if s == t.secs {
		return
	}
	t.secs = s
	t.Text = fmt.Sprintf(t.Format, s)
	t.Draw()
}
