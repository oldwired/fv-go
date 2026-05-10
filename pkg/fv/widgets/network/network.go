// Package network provides NetworkView — interface-by-interface
// throughput display with sparklines.
package network

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Interface is one network device.
type Interface struct {
	Name       string
	BytesIn    uint64
	BytesOut   uint64
	BytesInPS  float64 // bytes per second this sample
	BytesOutPS float64
	HistoryIn  []float64 // last N samples for the sparkline
	HistoryOut []float64
}

// Sampler returns the current set of interfaces.
type Sampler func() []Interface

// NetworkView paints one row per interface with throughput numbers.
type NetworkView struct {
	views.Base

	Sample   Sampler
	Interval time.Duration
	ifs      []Interface

	InColor    uint16
	OutColor   uint16
	LabelColor uint16
}

// New constructs a NetworkView.
func New(bounds geom.Rect, sampler Sampler, interval time.Duration) *NetworkView {
	if interval == 0 {
		interval = time.Second
	}
	n := &NetworkView{
		Base:       views.NewBase(bounds),
		Sample:     sampler,
		Interval:   interval,
		InColor:    types.MakeAttr(0x0A, 0x00),
		OutColor:   types.MakeAttr(0x0B, 0x00),
		LabelColor: types.MakeAttr(0x0F, 0x00),
	}
	n.SetSelf(n)
	if sampler != nil {
		anim.Register(n, interval)
	}
	return n
}

// GetTypeID for serial registry.
func (n *NetworkView) GetTypeID() string { return "networkview" }

// Tick polls.
func (n *NetworkView) Tick(now time.Time) bool {
	if n.Sample == nil {
		return false
	}
	n.ifs = n.Sample()
	return true
}

// Draw paints rows.
func (n *NetworkView) Draw() {
	for y := 0; y < n.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(n.Size.X)
		for x := 0; x < n.Size.X; x++ {
			screen.DrawCell(buf, x, " ", n.LabelColor)
		}
		if y < len(n.ifs) {
			ifc := n.ifs[y]
			label := fmt.Sprintf("%-8s ↓ %8s ↑ %8s",
				truncate(ifc.Name, 8),
				humanRate(ifc.BytesInPS),
				humanRate(ifc.BytesOutPS))
			screen.DrawStr(buf, 0, label, n.LabelColor)
		}
		n.WriteLine(0, y, n.Size.X, 1, buf)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func humanRate(bps float64) string {
	const k = 1024.0
	switch {
	case bps >= k*k*k:
		return fmt.Sprintf("%.1fGB/s", bps/(k*k*k))
	case bps >= k*k:
		return fmt.Sprintf("%.1fMB/s", bps/(k*k))
	case bps >= k:
		return fmt.Sprintf("%.1fKB/s", bps/k)
	}
	return fmt.Sprintf("%4.0fB/s", bps)
}
