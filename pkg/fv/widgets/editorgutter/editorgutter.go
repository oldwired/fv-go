// Package editorgutter provides EditorGutter — a vertical column that
// sits to the left (or right) of an Editor and renders per-line
// metadata: line numbers, bookmarks, breakpoints, diff markers, etc.
//
// The gutter pulls data from a list of Providers. Each provider is
// asked for one cell of metadata per visible line; the gutter
// concatenates them into the final column. Built-in providers cover
// the most common cases; users can add their own.
//
// Ported from EditorGutter.pas. The Pascal version's "click on the
// gutter to toggle a bookmark / breakpoint" interaction goes through
// a callback hook; we expose the same shape.
package editorgutter

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/editor"
)

// Provider supplies a single character (or string) per displayed line.
type Provider interface {
	// Width is the number of cells this provider occupies per row.
	Width() int
	// CellAt returns the rune+attribute to draw for line lineNum (0-based).
	// The gutter passes the editor's view of which lines are visible.
	CellAt(lineNum int) (text string, attr uint16)
}

// EditorGutter is a thin column linked to an Editor. It tracks the
// editor's Top scroll value to keep its rendering in sync.
type EditorGutter struct {
	views.Base

	Editor    *editor.Editor
	Providers []Provider
	Color     uint16
	OnClick   func(lineNum int)
}

// New constructs a gutter for ed with the given provider list.
func New(bounds geom.Rect, ed *editor.Editor, providers ...Provider) *EditorGutter {
	g := &EditorGutter{
		Base:      views.NewBase(bounds),
		Editor:    ed,
		Providers: providers,
		Color:     theme.Get().StatNeutral,
	}
	g.SetSelf(g)
	g.GrowMode = consts.GfGrowHiY // sticks to left edge, stretches vertically
	return g
}

// GetTypeID for serial registry.
func (g *EditorGutter) GetTypeID() string { return "editorgutter" }

// Draw paints one row per visible editor line. Provider strings are
// rendered rune-aware via DrawStr so multi-byte glyphs (★ ● ▲ +) land
// in single cells instead of being split into raw UTF-8 bytes.
func (g *EditorGutter) Draw() {
	if g.Editor == nil {
		return
	}
	for r := 0; r < g.Size.Y; r++ {
		buf := screen.MakeDrawBuffer(g.Size.X)
		for x := 0; x < g.Size.X; x++ {
			screen.DrawCell(buf, x, " ", g.Color)
		}
		lineNum := g.Editor.Top + r
		if lineNum < g.Editor.LineCount() {
			x := 0
			for _, p := range g.Providers {
				text, attr := p.CellAt(lineNum)
				w := p.Width()
				// Pre-fill the provider's slot with its background so
				// short returns don't leak the gutter color through.
				for i := 0; i < w && x+i < g.Size.X; i++ {
					buf[x+i] = types.DrawCell{Ch: " ", Attr: attr}
				}
				screen.DrawStr(buf, x, text, attr)
				x += w
			}
		}
		g.WriteLine(0, r, g.Size.X, 1, buf)
	}
}

// HandleEvent fires the OnClick callback when the user clicks a row.
func (g *EditorGutter) HandleEvent(ev *drivers.Event) {
	if ev.What != consts.EvMouseDown || g.OnClick == nil || g.Editor == nil {
		return
	}
	local := g.MakeLocal(ev.Where)
	if local.Y < 0 || local.Y >= g.Size.Y {
		return
	}
	lineNum := g.Editor.Top + local.Y
	if lineNum >= g.Editor.LineCount() {
		return
	}
	g.OnClick(lineNum)
	g.ClearEvent(ev)
}

// LineNumbers is a Provider that paints the (1-based) line index.
type LineNumbers struct {
	Width_ int // number of digits to pad to
	Attr   uint16
}

// NewLineNumbers builds a LineNumbers provider that pads the printed
// index to width digits (minimum 4 — narrower numbers leave the
// gutter feeling cramped). Uses theme.EditorLineNo for color.
func NewLineNumbers(width int) *LineNumbers {
	if width < 2 {
		width = 4
	}
	return &LineNumbers{Width_: width, Attr: theme.Get().EditorLineNo}
}

// Width reports the gutter column count this provider needs (digits +
// one trailing space).
func (l *LineNumbers) Width() int { return l.Width_ + 1 } // include trailing space

// CellAt renders the 1-based line index right-aligned in the gutter
// column.
func (l *LineNumbers) CellAt(lineNum int) (string, uint16) {
	return fmt.Sprintf("%*d ", l.Width_, lineNum+1), l.Attr
}

// Bookmarks is a Provider that paints '★' on bookmarked lines.
type Bookmarks struct {
	Lines map[int]bool
	Attr  uint16
}

// NewBookmarks builds an empty Bookmarks provider. Use Toggle to flip
// the bookmark state of a 0-based line index.
func NewBookmarks() *Bookmarks {
	return &Bookmarks{Lines: map[int]bool{}, Attr: theme.Get().EditorBookmark}
}

// Toggle flips the bookmark state for line. Adding a line that's
// already bookmarked removes it.
func (b *Bookmarks) Toggle(line int) {
	if b.Lines[line] {
		delete(b.Lines, line)
	} else {
		b.Lines[line] = true
	}
}

// Width reports the two-cell gutter footprint for the marker glyph.
func (b *Bookmarks) Width() int { return 2 }

// CellAt renders '★ ' on bookmarked lines and two spaces elsewhere.
func (b *Bookmarks) CellAt(lineNum int) (string, uint16) {
	if b.Lines[lineNum] {
		return "★ ", b.Attr
	}
	return "  ", b.Attr
}

// Breakpoints is a Provider that paints '●' on breakpoint lines.
type Breakpoints struct {
	Lines map[int]bool
	Attr  uint16
}

// NewBreakpoints builds an empty Breakpoints provider. Use Toggle to
// flip the breakpoint state of a 0-based line index.
func NewBreakpoints() *Breakpoints {
	return &Breakpoints{Lines: map[int]bool{}, Attr: theme.Get().EditorBreakpoint}
}

// Toggle flips the breakpoint state for line.
func (b *Breakpoints) Toggle(line int) {
	if b.Lines[line] {
		delete(b.Lines, line)
	} else {
		b.Lines[line] = true
	}
}

// Width reports the two-cell gutter footprint for the marker glyph.
func (b *Breakpoints) Width() int { return 2 }

// CellAt renders '● ' on breakpoint lines and two spaces elsewhere.
func (b *Breakpoints) CellAt(lineNum int) (string, uint16) {
	if b.Lines[lineNum] {
		return "● ", b.Attr
	}
	return "  ", b.Attr
}

// Diff is a Provider that paints +/- markers for inserted / removed
// lines. Inserted maps line→true, Removed similar.
type Diff struct {
	Inserted map[int]bool
	Removed  map[int]bool
	AddAttr  uint16
	DelAttr  uint16
}

// NewDiff builds an empty Diff provider. Hosts populate Inserted /
// Removed with line indices to mark added/removed lines.
func NewDiff() *Diff {
	return &Diff{
		Inserted: map[int]bool{},
		Removed:  map[int]bool{},
		AddAttr:  theme.Get().GaugeGood,
		DelAttr:  theme.Get().GaugeCrit,
	}
}

// Width reports the two-cell gutter footprint for the marker glyph.
func (d *Diff) Width() int { return 2 }

// CellAt renders '+ ' on inserted lines, '- ' on removed lines, and
// two spaces elsewhere.
func (d *Diff) CellAt(lineNum int) (string, uint16) {
	if d.Inserted[lineNum] {
		return "+ ", d.AddAttr
	}
	if d.Removed[lineNum] {
		return "- ", d.DelAttr
	}
	return "  ", theme.Get().StatNeutral
}
