package msgbox

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/dialogs"
)

func TestMessageButtonShadowsClearBottomBorder(t *testing.T) {
	tests := []struct {
		name    string
		buttons int
	}{
		{name: "yes-no", buttons: YesNo},
		{name: "ok-cancel", buttons: OKCancel},
		{name: "yes-no-cancel", buttons: YesNoCancel},
		{name: "ok-only", buttons: OKOnly},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := buildDialog("Test", "Message", tt.buttons)
			bottomBorderY := d.Size.Y - 1
			buttonCount := 0

			for _, child := range d.Children {
				button, ok := child.(*dialogs.Button)
				if !ok {
					continue
				}
				buttonCount++
				shadowY := button.Origin.Y + button.Size.Y
				if shadowY >= bottomBorderY {
					t.Errorf("button %q shadow row %d overlaps bottom border row %d", button.Title, shadowY, bottomBorderY)
				}
			}

			if buttonCount == 0 {
				t.Fatal("message box contains no buttons")
			}
		})
	}
}
