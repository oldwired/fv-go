package menus

import (
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// TestMenuBoxShortcut verifies that Item.Shortcut renders right-aligned
// on the row and widens the box enough to fit.
func TestMenuBoxShortcut(t *testing.T) {
	h := term.NewHeadless(40, 8)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	menu := NewMenu(
		&Item{Name: "Save", Command: 100, Shortcut: "Ctrl+S"},
		&Item{Name: "Quit", Command: 101, Shortcut: "Alt+X"},
	)
	mb := NewMenuBox(geom.Point{X: 0, Y: 0}, menu)
	mb.State |= consts.SfExposed | consts.SfVisible
	mb.Draw()
	_ = h.Flush()

	snap := h.Snapshot()
	// Both shortcuts must appear in the rendered text.
	if !strings.Contains(snap, "Ctrl+S") {
		t.Errorf("shortcut 'Ctrl+S' missing from snapshot:\n%s", snap)
	}
	if !strings.Contains(snap, "Alt+X") {
		t.Errorf("shortcut 'Alt+X' missing from snapshot:\n%s", snap)
	}
	// The shortcut on a row should sit to the right of the name. Find
	// the first content row and confirm "Save" comes before "Ctrl+S".
	for _, line := range strings.Split(snap, "\n") {
		if strings.Contains(line, "Save") {
			savePos := strings.Index(line, "Save")
			scPos := strings.Index(line, "Ctrl+S")
			if scPos <= savePos {
				t.Errorf("shortcut not right-aligned on Save row: %q", line)
			}
			break
		}
	}
}

// TestMenuBoxSizerNoShortcut confirms that menus without any shortcut
// don't get the extra right column reserved (width matches the legacy
// "name + borders" formula).
func TestMenuBoxSizerNoShortcut(t *testing.T) {
	menu := NewMenu(&Item{Name: "Save", Command: 100})
	w, _ := menuBoxSize(menu)
	if w != 8 { // 4 ("Save") + 4 (borders/padding)
		t.Errorf("size without shortcut = %d, want 8", w)
	}
	menuWithSc := NewMenu(&Item{Name: "Save", Command: 100, Shortcut: "Ctrl+S"})
	w2, _ := menuBoxSize(menuWithSc)
	if w2 <= w {
		t.Errorf("size with shortcut (%d) should exceed size without (%d)", w2, w)
	}
}
