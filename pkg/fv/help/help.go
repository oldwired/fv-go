// Package help provides the context-sensitive help system: callers
// register help text against numeric "context" IDs (the same uint16
// values used by views.Base.HelpCtx), and Show opens a modal window
// displaying the text for a given context. The Program's F1 handler
// looks up the focus chain for a non-zero HelpCtx and calls Show.
//
// Help bodies are plain text with a few formatting conventions: lines
// starting with "## " render as section headers; "  " (two spaces)
// indent is preserved. Bullet lists use "- ". Everything else is
// passed through verbatim.
package help

import (
	"strings"
	"sync"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

var (
	mu    sync.RWMutex
	pages = map[uint16]string{}
)

// Register associates body with help-context ctx. Calling Register
// twice with the same ctx replaces the body. Pass ctx=0 (HcNoContext)
// is a no-op — that's the "no help" sentinel.
func Register(ctx uint16, body string) {
	if ctx == 0 {
		return
	}
	mu.Lock()
	pages[ctx] = body
	mu.Unlock()
}

// Text retrieves the help body for ctx, or "" if none registered.
func Text(ctx uint16) string {
	mu.RLock()
	defer mu.RUnlock()
	return pages[ctx]
}

// Show opens a modal help window for ctx. Cancels silently if no
// help is registered (caller can check Text() first if they want a
// "no help available" toast instead).
func Show(host *views.Group, ctx uint16) {
	if ctx == 0 {
		return
	}
	body := Text(ctx)
	if body == "" {
		body = "(no help available for this context)"
	}
	w, h := 60, 18
	d := dialogs.NewDialog(geom.NewRect(0, 0, w, h), "Help")
	d.Options |= consts.OfCentered
	view := newHelpView(geom.NewRect(2, 2, w-2, h-3), body)
	d.Insert(view)
	d.Insert(dialogs.NewButton(geom.NewRect(w-12, h-3, w-2, h-2), "O~K~",
		consts.CmOK, dialogs.BfDefault))
	host.ExecView(d)
}

// helpView is the scrollable text body inside the help window.
type helpView struct {
	views.Base
	lines []string
	top   int
}

func newHelpView(bounds geom.Rect, body string) *helpView {
	v := &helpView{
		Base:  views.NewBase(bounds),
		lines: strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n"),
	}
	v.SetSelf(v)
	return v
}

func (v *helpView) GetTypeID() string { return "helpview" }

func (v *helpView) Draw() {
	pal := theme.Get()
	bg := pal.HelpBackground
	headingAttr := pal.HelpHeading
	bulletAttr := pal.HelpBullet
	for r := 0; r < v.Size.Y; r++ {
		buf := screen.MakeDrawBuffer(v.Size.X)
		for x := 0; x < v.Size.X; x++ {
			screen.DrawCell(buf, x, " ", bg)
		}
		idx := v.top + r
		if idx >= 0 && idx < len(v.lines) {
			line := v.lines[idx]
			switch {
			case strings.HasPrefix(line, "## "):
				screen.DrawStr(buf, 0, line[3:], headingAttr)
			case strings.HasPrefix(line, "- "):
				screen.DrawCell(buf, 0, "•", bulletAttr)
				screen.DrawStr(buf, 2, line[2:], bg)
			default:
				screen.DrawStr(buf, 0, line, bg)
			}
		}
		v.WriteLine(0, r, v.Size.X, 1, buf)
	}
}
