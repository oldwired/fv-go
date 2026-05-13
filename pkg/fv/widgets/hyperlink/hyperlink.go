// Package hyperlink provides Hyperlink — a clickable text view that
// renders as a standard underlined "link" and additionally wraps its
// cells in the OSC 8 hyperlink escape so terminals that support the
// protocol (iTerm2, WezTerm, recent gnome-terminal, mintty, Windows
// Terminal …) make the text actually clickable.
//
// Terminals without OSC 8 support silently ignore the escape; the
// underlined text remains and serves as the visual cue.
package hyperlink

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Hyperlink is a single-line text view that emits OSC 8.
type Hyperlink struct {
	views.Base

	URL  string
	Text string

	// Attr is the SGR attribute pack for the rendered text. Default
	// is bright cyan underlined.
	Attr uint16
}

// New constructs a Hyperlink. Bounds typically a single row tall.
func New(bounds geom.Rect, text, url string) *Hyperlink {
	h := &Hyperlink{
		Base: views.NewBase(bounds),
		Text: text,
		URL:  url,
		Attr: theme.Get().HyperlinkNormal,
	}
	h.SetSelf(h)
	return h
}

// GetTypeID for serial registry.
func (h *Hyperlink) GetTypeID() string { return "hyperlink" }

// Draw renders the text, painting underline via ExtAttrs and the OSC
// 8 wrap via the HyperlinkURL field on DrawCell.
func (h *Hyperlink) Draw() {
	w := h.Size.X
	if w <= 0 {
		return
	}
	buf := screen.MakeDrawBuffer(w)
	// Background fill.
	for x := 0; x < w; x++ {
		buf[x] = types.DrawCell{Ch: " ", Attr: h.Attr}
	}
	// Each visible rune carries the underline ExtAttr + the URL. The
	// term backend coalesces consecutive identical-URL cells into a
	// single OSC 8 ; ; URL … OSC 8 ; ; pair.
	x := 0
	for _, r := range h.Text {
		if x >= w {
			break
		}
		cell := types.DrawCell{
			Ch:           string(r),
			Attr:         h.Attr,
			HyperlinkURL: h.URL,
		}
		cell.SetUnderlineStyle(types.UnderSingle)
		buf[x] = cell
		x++
	}
	h.WriteLine(0, 0, w, 1, buf)
}

// We don't make Hyperlink selectable / focusable by default — the
// link is meant to be clicked via the host terminal's mouse handling,
// not the app's. If a caller wants the inner app to also react, they
// can set OfSelectable + add a HandleEvent override.
var _ = consts.SfVisible
