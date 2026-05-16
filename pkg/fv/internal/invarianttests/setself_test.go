// Package invarianttests holds cross-package tests that need to import
// every widget package — placed under internal/ so external consumers
// can't accidentally depend on it. The only invariant covered today is
// SetSelf: every widget constructor must wire BaseView().Self() to the
// concrete pointer so virtual-dispatch in Group.HandleEvent / Draw
// reaches the overrides.
package invarianttests

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/menus"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/accordion"
	"github.com/oldwired/fv-go/pkg/fv/widgets/calendar"
	"github.com/oldwired/fv-go/pkg/fv/widgets/clock"
	"github.com/oldwired/fv-go/pkg/fv/widgets/colorsel"
	"github.com/oldwired/fv-go/pkg/fv/widgets/combobox"
	"github.com/oldwired/fv-go/pkg/fv/widgets/grid"
	"github.com/oldwired/fv-go/pkg/fv/widgets/heapview"
	"github.com/oldwired/fv-go/pkg/fv/widgets/hyperlink"
	"github.com/oldwired/fv-go/pkg/fv/widgets/markdown"
	"github.com/oldwired/fv-go/pkg/fv/widgets/progressbar"
	"github.com/oldwired/fv-go/pkg/fv/widgets/spinner"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"
	"github.com/oldwired/fv-go/pkg/fv/widgets/toggle"
	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"
)

// TestAllConstructorsSetSelf instantiates every exported widget
// constructor we cover here and verifies BaseView().Self() points
// back at the concrete value. Forgetting SetSelf is the single most
// common Group/embedding mistake in this codebase — without it,
// HandleEvent / Draw dispatch silently falls through to Base no-ops.
func TestAllConstructorsSetSelf(t *testing.T) {
	r := geom.NewRect(0, 0, 20, 5)

	cases := []struct {
		name string
		make func() views.View
	}{
		{"Button", func() views.View {
			return dialogs.NewButton(r, "ok", 0, 0)
		}},
		{"InputLine", func() views.View {
			return dialogs.NewInputLine(r, 32)
		}},
		{"Cluster", func() views.View {
			return dialogs.NewCluster(r, []string{"a", "b"})
		}},
		{"CheckBoxes", func() views.View {
			return dialogs.NewCheckBoxes(r, []string{"a", "b"})
		}},
		{"History", func() views.View {
			il := dialogs.NewInputLine(r, 32)
			return dialogs.NewHistory(geom.NewRect(20, 0, 21, 1), il, 0)
		}},
		{"RadioButtons", func() views.View {
			return dialogs.NewRadioButtons(r, []string{"a", "b"})
		}},
		{"Dialog", func() views.View {
			return dialogs.NewDialog(r, "title")
		}},
		{"Window", func() views.View {
			return views.NewWindow(r, "w", 0)
		}},
		{"MenuBar", func() views.View {
			return menus.NewMenuBar(geom.NewRect(0, 0, 40, 1), nil)
		}},
		{"StatusLine", func() views.View {
			return menus.NewStatusLine(geom.NewRect(0, 0, 40, 1), nil)
		}},
		{"Accordion", func() views.View {
			return accordion.New(r)
		}},
		{"Calendar", func() views.View {
			return calendar.New(r)
		}},
		{"ClockDigital", func() views.View {
			return clock.NewDigital(r)
		}},
		{"ClockAnalog", func() views.View {
			return clock.NewAnalog(geom.NewRect(0, 0, 21, 11))
		}},
		{"ColorSelector", func() views.View {
			return colorsel.New(r, 0)
		}},
		{"ComboBox", func() views.View {
			return combobox.New(r, []string{"a", "b"}, 32)
		}},
		{"Grid", func() views.View {
			return grid.New(r, []grid.Column{{Title: "a"}}, nil, nil)
		}},
		{"HeapView", func() views.View {
			return heapview.New(r)
		}},
		{"Hyperlink", func() views.View {
			return hyperlink.New(r, "label", "https://example.com")
		}},
		{"Markdown", func() views.View {
			return markdown.New(r, nil)
		}},
		{"ProgressBar", func() views.View {
			return progressbar.New(r, 0, 100)
		}},
		{"Spinner", func() views.View {
			return spinner.New(r)
		}},
		{"Terminal", func() views.View {
			return terminal.New(r)
		}},
		{"Toggle", func() views.View {
			return toggle.New(r, "on", 0, false)
		}},
		{"TreeView", func() views.View {
			return treeview.New(r, nil)
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := c.make()
			if v == nil {
				t.Fatalf("%s constructor returned nil", c.name)
			}
			bv := v.BaseView()
			if bv == nil {
				t.Fatalf("%s.BaseView() returned nil", c.name)
			}
			if bv.Self() == nil {
				t.Fatalf("%s did not call SetSelf — Self() is nil", c.name)
			}
			// Self() must be the concrete value we just constructed.
			// Comparing by interface equality catches the common
			// foot-gun where Self holds a pointer to an embedded
			// Base instead of the outer struct.
			if bv.Self() != v {
				t.Errorf("%s.Self() returned a different value than the constructor; "+
					"got %T, want %T", c.name, bv.Self(), v)
			}
		})
	}
}
