package hoverpopup

import (
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

func newHost(w, h int) *views.Group {
	g := views.NewGroup(geom.NewRect(0, 0, w, h))
	g.State |= consts.SfVisible | consts.SfExposed
	return g
}

func TestShowSizesFromContent(t *testing.T) {
	host := newHost(80, 24)
	p := New()
	p.Show(host, geom.Point{X: 10, Y: 5}, "hello\nworld of popups")
	if !p.IsOpen() {
		t.Fatal("popup must be open")
	}
	// Widest line "world of popups" = 15 + 4 (border+padding) = 19.
	if p.Size.X != 19 || p.Size.Y != 4 {
		t.Errorf("size = %v, want 19x4", p.Size)
	}
	if p.Origin.X != 10 || p.Origin.Y != 6 {
		t.Errorf("origin = %v, want below anchor (10,6)", p.Origin)
	}
}

func TestShowWrapsAtMaxWidth(t *testing.T) {
	host := newHost(80, 24)
	p := New()
	p.MaxWidth = 10
	p.Show(host, geom.Point{}, "aaa bbb ccc ddd eee")
	if p.Size.X > 14 {
		t.Errorf("width = %d, want <= MaxWidth+4", p.Size.X)
	}
	if p.Size.Y < 4 {
		t.Errorf("height = %d, want wrapped to multiple lines", p.Size.Y)
	}
}

func TestShowClampsAndFlips(t *testing.T) {
	host := newHost(40, 12)
	p := New()
	// Anchor near the bottom-right: must flip above and clamp X.
	p.Show(host, geom.Point{X: 38, Y: 11}, "line one\nline two\nline three")
	if p.Origin.X+p.Size.X > 40 {
		t.Errorf("popup crosses right edge: origin %v size %v", p.Origin, p.Size)
	}
	if p.Origin.Y+p.Size.Y > 12 {
		t.Errorf("popup crosses bottom edge: origin %v size %v", p.Origin, p.Size)
	}
	if p.Origin.Y >= 11 {
		t.Errorf("popup must flip above the anchor: origin %v", p.Origin)
	}
}

func TestShowDoesNotStealFocus(t *testing.T) {
	host := newHost(80, 24)
	btn := views.NewBase(geom.NewRect(0, 0, 10, 1))
	focusable := &focusableView{Base: btn}
	focusable.SetSelf(focusable)
	focusable.Options |= consts.OfSelectable
	host.Insert(focusable)
	if host.Current() != views.View(focusable) {
		t.Fatal("test premise: dummy focused")
	}
	p := New()
	p.Show(host, geom.Point{X: 5, Y: 5}, "tip")
	if host.Current() != views.View(focusable) {
		t.Error("Show must not steal focus")
	}
}

type focusableView struct{ views.Base }

func TestCloseIdempotent(t *testing.T) {
	host := newHost(80, 24)
	p := New()
	p.Show(host, geom.Point{}, "x")
	p.Close()
	if p.IsOpen() {
		t.Fatal("Close must remove the popup")
	}
	p.Close() // second close: no panic
	if got := len(host.Children); got != 0 {
		t.Errorf("host children after close = %d, want 0", got)
	}
}

func TestAutoDismissOutsideClickClosesWithoutConsuming(t *testing.T) {
	host := newHost(80, 24)
	p := New()
	p.Show(host, geom.Point{X: 10, Y: 5}, "tip")
	ev := drivers.Event{What: consts.EvMouseDown, Buttons: consts.MbLeftButton,
		Where: geom.Point{X: 70, Y: 20}}
	host.HandleEvent(&ev)
	if p.IsOpen() {
		t.Error("outside click must close the popup")
	}
	if ev.What == consts.EvNothing {
		t.Error("outside click must NOT be consumed (it has a real target)")
	}
}

func TestAutoDismissInsideClickClosesAndConsumes(t *testing.T) {
	host := newHost(80, 24)
	p := New()
	p.Show(host, geom.Point{X: 10, Y: 5}, "tip")
	ev := drivers.Event{What: consts.EvMouseDown, Buttons: consts.MbLeftButton,
		Where: geom.Point{X: 11, Y: 7}} // inside the popup box
	host.HandleEvent(&ev)
	if p.IsOpen() {
		t.Error("inside click must close the popup")
	}
	if ev.What != consts.EvNothing {
		t.Error("a click on the popup itself is consumed")
	}
}

func TestAutoDismissEscClosesAndConsumes(t *testing.T) {
	host := newHost(80, 24)
	p := New()
	p.Show(host, geom.Point{}, "tip")
	ev := drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbEsc}
	host.HandleEvent(&ev)
	if p.IsOpen() {
		t.Error("Esc must close the popup")
	}
	if ev.What != consts.EvNothing {
		t.Error("Esc that closed the popup is consumed")
	}
}

func TestAutoDismissOtherKeyClosesAndPassesThrough(t *testing.T) {
	host := newHost(80, 24)
	p := New()
	p.Show(host, geom.Point{}, "tip")
	ev := drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbDown}
	host.HandleEvent(&ev)
	if p.IsOpen() {
		t.Error("any key must close the popup")
	}
	if ev.What == consts.EvNothing {
		t.Error("non-Esc keys must pass through to the focused view")
	}
}

func TestNoAutoDismissPersists(t *testing.T) {
	host := newHost(80, 24)
	p := New()
	p.AutoDismiss = false
	p.Show(host, geom.Point{X: 10, Y: 5}, "tip")
	ev := drivers.Event{What: consts.EvMouseDown, Buttons: consts.MbLeftButton,
		Where: geom.Point{X: 70, Y: 20}}
	host.HandleEvent(&ev)
	if !p.IsOpen() {
		t.Error("without AutoDismiss an outside click must not close the popup")
	}
	p.Close()
}

func TestReShowUpdatesInPlace(t *testing.T) {
	host := newHost(80, 24)
	p := New()
	p.Show(host, geom.Point{X: 5, Y: 5}, "short")
	first := len(host.Children)
	p.Show(host, geom.Point{X: 8, Y: 8}, "a rather longer hover text")
	if got := len(host.Children); got != first {
		t.Errorf("re-Show changed child count: %d → %d", first, got)
	}
	if p.Origin.Y != 9 {
		t.Errorf("re-Show must reposition: origin %v", p.Origin)
	}
}

func TestWrapLine(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  []string
	}{
		{"short", 10, []string{"short"}},
		{"aaa bbb ccc", 7, []string{"aaa bbb", "ccc"}},
		{"aaa bbb", 3, []string{"aaa", "bbb"}},
		{"abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"", 5, []string{""}},
		// Indentation survives on wrapped continuations (code blocks).
		{"    aaa bbb ccc", 8, []string{"    aaa", "    bbb", "    ccc"}},
		{"  short", 10, []string{"  short"}},
	}
	for _, c := range cases {
		got := wrapLine(c.in, c.width)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("wrapLine(%q,%d) = %v, want %v", c.in, c.width, got, c.want)
		}
	}
}
