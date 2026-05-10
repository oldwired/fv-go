package dialogs

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Cluster is the abstract base of CheckBoxes / RadioButtons. It owns
// a slice of label strings and a Sel index of the focused row plus a
// uint32 bitmask Value of which rows are "on".
type Cluster struct {
	views.Base

	Strings []string
	Sel     int
	Value   uint32
}

// NewCluster builds a Cluster.
func NewCluster(bounds geom.Rect, items []string) *Cluster {
	c := &Cluster{Base: views.NewBase(bounds), Strings: items}
	c.SetSelf(c)
	c.Options |= consts.OfSelectable | consts.OfFirstClick | consts.OfPreProcess | consts.OfPostProcess
	c.State |= consts.SfCursorVis
	return c
}

// GetTypeID for serial registry.
func (c *Cluster) GetTypeID() string { return "cluster" }

// Mark reports whether the i-th item's bit is set.
func (c *Cluster) Mark(i int) bool { return c.Value&(1<<i) != 0 }

// PressOne sets only the i-th bit (radio behavior).
func (c *Cluster) PressOne(i int) {
	c.Value = 1 << i
}

// Toggle flips the i-th bit (checkbox behavior).
func (c *Cluster) Toggle(i int) {
	c.Value ^= 1 << i
}

// MoveSel changes the focused row and redraws.
func (c *Cluster) MoveSel(i int) {
	if i < 0 {
		i = 0
	}
	if i >= len(c.Strings) {
		i = len(c.Strings) - 1
	}
	if i != c.Sel {
		c.Sel = i
		c.Draw()
	}
}

// CheckBoxes is a Cluster where any subset can be on. Drawn with [ ]/[x].
type CheckBoxes struct {
	Cluster
}

// NewCheckBoxes builds a CheckBoxes.
func NewCheckBoxes(bounds geom.Rect, items []string) *CheckBoxes {
	c := &CheckBoxes{Cluster: *NewCluster(bounds, items)}
	c.SetSelf(c)
	return c
}

// GetTypeID for serial registry.
func (c *CheckBoxes) GetTypeID() string { return "checkboxes" }

// Draw paints checkboxes.
func (c *CheckBoxes) Draw() {
	drawCluster(&c.Cluster, "[", "]", "x", "")
}

// HandleEvent: arrows, space toggles.
func (c *CheckBoxes) HandleEvent(ev *drivers.Event) {
	clusterHandle(&c.Cluster, ev, c.Toggle)
}

// RadioButtons is a Cluster where exactly one is on. Drawn with ( )/(•).
type RadioButtons struct {
	Cluster
}

// NewRadioButtons builds a RadioButtons.
func NewRadioButtons(bounds geom.Rect, items []string) *RadioButtons {
	c := &RadioButtons{Cluster: *NewCluster(bounds, items)}
	c.SetSelf(c)
	return c
}

// GetTypeID for serial registry.
func (c *RadioButtons) GetTypeID() string { return "radiobuttons" }

// Draw paints radio buttons.
func (c *RadioButtons) Draw() {
	drawCluster(&c.Cluster, "(", ")", "•", " ")
}

// HandleEvent: arrows, space presses one.
func (c *RadioButtons) HandleEvent(ev *drivers.Event) {
	clusterHandle(&c.Cluster, ev, func(i int) { c.Cluster.PressOne(i) })
}

func drawCluster(c *Cluster, lb, rb, marked, unmarked string) {
	// Dialog cluster palette: light-gray on blue, with bright-yellow
	// hotkey, and white-on-cyan when focused.
	normal := types.MakeAttr(0x07, 0x01)
	hot := types.MakeAttr(0x0E, 0x01)
	focusNormal := types.MakeAttr(0x0F, 0x03)
	focusHot := types.MakeAttr(0x0E, 0x03)
	for y, label := range c.Strings {
		if y >= c.Size.Y {
			break
		}
		buf := screen.MakeDrawBuffer(c.Size.X)
		n, hk := normal, hot
		if y == c.Sel && c.GetState(consts.SfFocused) {
			n, hk = focusNormal, focusHot
		}
		for x := 0; x < c.Size.X; x++ {
			screen.DrawCell(buf, x, " ", n)
		}
		mark := unmarked
		if c.Mark(y) {
			mark = marked
		}
		screen.DrawStr(buf, 0, lb, n)
		screen.DrawStr(buf, 1, mark, n)
		screen.DrawStr(buf, 2, rb, n)
		screen.DrawCStr(buf, 4, label, n, hk)
		c.WriteLine(0, y, c.Size.X, 1, buf)
		if y == c.Sel {
			c.Cursor = geom.Point{X: 1, Y: y}
		}
	}
}

func clusterHandle(c *Cluster, ev *drivers.Event, toggle func(int)) {
	if ev.What == consts.EvMouseDown {
		local := c.MakeLocal(ev.Where)
		if local.Y >= 0 && local.Y < len(c.Strings) && local.X >= 0 && local.X < c.Size.X {
			c.Sel = local.Y
			toggle(c.Sel)
			if c.Owner != nil {
				c.Owner.Focus(c.Self())
			}
			c.Draw()
			c.ClearEvent(ev)
		}
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbUp:
		c.MoveSel(c.Sel - 1)
	case consts.KbDown:
		c.MoveSel(c.Sel + 1)
	case consts.KbSpaceBar:
		toggle(c.Sel)
		c.Draw()
	default:
		return
	}
	c.ClearEvent(ev)
}

// DataSize/GetData/SetData mirror Pascal: bit-packed Word.
func (c *Cluster) DataSize() int { return 4 }

func (c *Cluster) GetData(buf []byte) {
	if len(buf) < 4 {
		return
	}
	v := c.Value
	buf[0] = byte(v)
	buf[1] = byte(v >> 8)
	buf[2] = byte(v >> 16)
	buf[3] = byte(v >> 24)
}

func (c *Cluster) SetData(buf []byte) {
	if len(buf) < 4 {
		return
	}
	c.Value = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
	c.Draw()
}
