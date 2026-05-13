// Package process provides ProcessView — a simple process-list table:
// PID, CPU%, MEM%, command. Bring-your-own sampler.
package process

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Process is one row.
type Process struct {
	PID     int
	CPU     float64 // [0.0, 1.0]
	Mem     float64 // [0.0, 1.0]
	Command string
}

// Sampler returns the process list.
type Sampler func() []Process

// ProcessView is a non-editing table.
type ProcessView struct {
	views.Base

	Sample   Sampler
	Interval time.Duration
	procs    []Process
	Top      int

	HeaderColor uint16
	RowColor    uint16
}

// New constructs a ProcessView.
func New(bounds geom.Rect, sampler Sampler, interval time.Duration) *ProcessView {
	if interval == 0 {
		interval = 2 * time.Second
	}
	p := &ProcessView{
		Base:        views.NewBase(bounds),
		Sample:      sampler,
		Interval:    interval,
		HeaderColor: theme.Get().NotificationError,
		RowColor:    theme.Get().TreeNormal,
	}
	p.SetSelf(p)
	if sampler != nil {
		anim.Register(p, interval)
	}
	return p
}

// GetTypeID for serial registry.
func (p *ProcessView) GetTypeID() string { return "processview" }

// Tick polls.
func (p *ProcessView) Tick(now time.Time) bool {
	if p.Sample == nil {
		return false
	}
	p.procs = p.Sample()
	return true
}

// Draw paints the header + rows.
//
// Columns are sized so a value of 100.0% still fits without pushing
// COMMAND off:
//
//	" %6s %6s %6s %s"      header
//	" %6d %5.1f%% %5.1f%% %s"  row
//
// "%5.1f%%" is 6 cells wide for any value in [0, 999.9] — the trailing
// "%" lines up with the right edge of the header's "%6s" column.
func (p *ProcessView) Draw() {
	header := fmt.Sprintf(" %6s %6s %6s %s", "PID", "CPU%", "MEM%", "COMMAND")
	if p.Size.Y > 0 {
		buf := screen.MakeDrawBuffer(p.Size.X)
		for x := 0; x < p.Size.X; x++ {
			screen.DrawCell(buf, x, " ", p.HeaderColor)
		}
		screen.DrawStr(buf, 0, header, p.HeaderColor)
		p.WriteLine(0, 0, p.Size.X, 1, buf)
	}
	for r := 1; r < p.Size.Y; r++ {
		buf := screen.MakeDrawBuffer(p.Size.X)
		for x := 0; x < p.Size.X; x++ {
			screen.DrawCell(buf, x, " ", p.RowColor)
		}
		idx := p.Top + r - 1
		if idx >= 0 && idx < len(p.procs) {
			pr := p.procs[idx]
			line := fmt.Sprintf(" %6d %5.1f%% %5.1f%% %s",
				pr.PID, pr.CPU*100, pr.Mem*100, pr.Command)
			screen.DrawStr(buf, 0, line, p.RowColor)
		}
		p.WriteLine(0, r, p.Size.X, 1, buf)
	}
}
