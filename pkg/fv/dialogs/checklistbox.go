package dialogs

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// CheckListBox is a StringListBox where each row carries an
// independent boolean. Space toggles the focused row; Enter still
// fires cmListItemSelected like the base list. Mirrors
// VSoft.AnsiConsole's MultiSelect prompt.
type CheckListBox struct {
	StringListBox
	checked []bool
}

// NewCheckListBox builds a CheckListBox seeded with items.
func NewCheckListBox(bounds geom.Rect, vScroll *views.ScrollBar, items []string) *CheckListBox {
	c := &CheckListBox{StringListBox: *NewStringListBox(bounds, vScroll, items)}
	c.SetSelf(c)
	c.checked = make([]bool, len(items))
	c.GetText = func(i int) string {
		if i < 0 || i >= len(c.Items) {
			return ""
		}
		mark := "[ ] "
		if i < len(c.checked) && c.checked[i] {
			mark = "[x] "
		}
		return mark + c.Items[i]
	}
	return c
}

// GetTypeID for serial registry.
func (c *CheckListBox) GetTypeID() string { return "checklistbox" }

// SetItems replaces the contents and clears all checks.
func (c *CheckListBox) SetItems(items []string) {
	c.Items = items
	c.checked = make([]bool, len(items))
	c.SetRange(len(items))
	c.Focused = 0
	c.Draw()
}

// IsChecked reports the i-th row's state.
func (c *CheckListBox) IsChecked(i int) bool {
	if i < 0 || i >= len(c.checked) {
		return false
	}
	return c.checked[i]
}

// SetChecked sets the i-th row's state and redraws.
func (c *CheckListBox) SetChecked(i int, v bool) {
	if i < 0 || i >= len(c.checked) {
		return
	}
	if c.checked[i] != v {
		c.checked[i] = v
		c.Draw()
	}
}

// ToggleChecked flips the i-th row.
func (c *CheckListBox) ToggleChecked(i int) {
	if i < 0 || i >= len(c.checked) {
		return
	}
	c.checked[i] = !c.checked[i]
	c.Draw()
}

// CheckAll sets every row to v.
func (c *CheckListBox) CheckAll(v bool) {
	for i := range c.checked {
		c.checked[i] = v
	}
	c.Draw()
}

// CheckedItems returns the captions of every checked row.
func (c *CheckListBox) CheckedItems() []string {
	out := make([]string, 0, len(c.checked))
	for i, on := range c.checked {
		if on && i < len(c.Items) {
			out = append(out, c.Items[i])
		}
	}
	return out
}

// HandleEvent: Space (or click) toggles the focused row; everything
// else defers to the embedded list (arrow keys, scroll, etc.).
func (c *CheckListBox) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvKeyDown && ev.KeyCode == consts.KbSpaceBar {
		c.ToggleChecked(c.Focused)
		c.ClearEvent(ev)
		return
	}
	if ev.What == consts.EvMouseDown {
		// Let the base list focus the clicked row first, then toggle
		// at the *new* Focused index.
		c.StringListBox.HandleEvent(ev)
		c.ToggleChecked(c.Focused)
		return
	}
	c.StringListBox.HandleEvent(ev)
}
