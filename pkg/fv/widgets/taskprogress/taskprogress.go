// Package taskprogress provides TaskProgress — a multi-row progress
// view: caption · spinner · bar · percent · ETA per task. Designed for
// "running multiple async jobs at once" dashboards.
package taskprogress

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Task is one row.
type Task struct {
	Caption   string
	Min, Max  int
	Value     int
	Done      bool
	Failed    bool
	StartedAt time.Time
}

// Progress reports v / max.
func (t *Task) Progress() float64 {
	span := t.Max - t.Min
	if span <= 0 {
		return 0
	}
	return float64(t.Value-t.Min) / float64(span)
}

// ETA estimates remaining wall-clock time based on linear progress.
// Returns 0 if there's not enough data.
func (t *Task) ETA(now time.Time) time.Duration {
	p := t.Progress()
	if p <= 0 || t.StartedAt.IsZero() {
		return 0
	}
	elapsed := now.Sub(t.StartedAt)
	total := time.Duration(float64(elapsed) / p)
	rem := total - elapsed
	if rem < 0 {
		return 0
	}
	return rem
}

// TaskProgress paints one row per Task.
type TaskProgress struct {
	views.Base

	Tasks []*Task
	frame int

	HeaderColor uint16
	BarColor    uint16
	BackColor   uint16
}

// New constructs a multi-task progress view.
func New(bounds geom.Rect) *TaskProgress {
	t := &TaskProgress{
		Base:        views.NewBase(bounds),
		HeaderColor: theme.Get().StatHeader,
		BarColor:    theme.Get().ProgressFilled,
		BackColor:   theme.Get().EditorComment,
	}
	t.SetSelf(t)
	anim.Register(t, 100*time.Millisecond)
	return t
}

// GetTypeID for serial registry.
func (t *TaskProgress) GetTypeID() string { return "taskprogress" }

// AddTask appends a task with the given caption and range.
func (t *TaskProgress) AddTask(caption string, min, max int) *Task {
	task := &Task{Caption: caption, Min: min, Max: max, Value: min, StartedAt: time.Now()}
	t.Tasks = append(t.Tasks, task)
	return task
}

// SetProgress updates a task's value (clamped to range).
func (t *TaskProgress) SetProgress(task *Task, v int) {
	if v < task.Min {
		v = task.Min
	}
	if v > task.Max {
		v = task.Max
	}
	task.Value = v
	if v >= task.Max {
		task.Done = true
	}
}

// Tick advances the spinner frame.
func (t *TaskProgress) Tick(now time.Time) bool {
	t.frame++
	for _, task := range t.Tasks {
		if !task.Done && !task.Failed {
			return true
		}
	}
	return false
}

// Draw paints rows.
func (t *TaskProgress) Draw() {
	spinFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	now := time.Now()

	for y := 0; y < t.Size.Y; y++ {
		buf := screen.MakeDrawBuffer(t.Size.X)
		for x := 0; x < t.Size.X; x++ {
			screen.DrawCell(buf, x, " ", t.HeaderColor)
		}
		if y < len(t.Tasks) {
			task := t.Tasks[y]
			// Spinner / status icon.
			icon := spinFrames[t.frame%len(spinFrames)]
			if task.Done {
				icon = "✓"
			} else if task.Failed {
				icon = "✗"
			}
			screen.DrawStr(buf, 0, icon+" ", t.HeaderColor)
			// Caption, max 24 chars.
			caption := task.Caption
			if len(caption) > 24 {
				caption = caption[:24]
			}
			screen.DrawStr(buf, 2, caption, t.HeaderColor)
			// Bar.
			barX := 28
			barW := t.Size.X - barX - 14
			if barW < 4 {
				barW = 4
			}
			fill := int(task.Progress() * float64(barW))
			for x := 0; x < barW; x++ {
				ch := "░"
				attr := t.BackColor
				if x < fill {
					ch = "█"
					attr = t.BarColor
				}
				if barX+x < t.Size.X {
					buf[barX+x] = types.DrawCell{Ch: ch, Attr: attr}
				}
			}
			// Percent + ETA.
			pct := int(task.Progress() * 100)
			info := fmt.Sprintf(" %3d%%", pct)
			if eta := task.ETA(now); eta > 0 && !task.Done {
				info += fmt.Sprintf(" %s", eta.Truncate(time.Second))
			}
			if barX+barW+1 < t.Size.X {
				screen.DrawStr(buf, barX+barW+1, info, t.HeaderColor)
			}
		}
		t.WriteLine(0, y, t.Size.X, 1, buf)
	}
}
