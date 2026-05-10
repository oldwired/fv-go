// Package markdown provides MarkdownView — a read-only widget that
// renders a basic subset of Markdown:
//
//   - Headings (#..######)
//   - Paragraph text with **bold** and *italic*
//   - Inline `code`
//   - Fenced ```code blocks```
//   - Bullet lists (- / *)
//   - Numbered lists (1. 2. 3.)
//   - Block quotes (>)
//   - Horizontal rules (--- / ***)
//   - [link text](url) — rendered as styled text; no actual hyperlinking
//
// Ported from MarkdownView.pas. The Pascal version uses regex-driven
// pre-pass; this Go port does a straightforward line-by-line scan.
package markdown

import (
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// MarkdownView is the read-only renderer.
type MarkdownView struct {
	views.Base

	source  []byte
	lines   []renderedLine
	Top     int
	VScroll *views.ScrollBar
}

// renderedLine is a pre-formatted display line (after MD parsing).
// Spans are byte ranges within Text and the attribute to paint them.
type renderedLine struct {
	Text  string
	Spans []span
}

type span struct {
	Start, End int
	Attr       uint16
}

// New constructs a viewer.
func New(bounds geom.Rect, v *views.ScrollBar) *MarkdownView {
	m := &MarkdownView{Base: views.NewBase(bounds), VScroll: v}
	m.SetSelf(m)
	m.Options |= consts.OfSelectable
	m.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	return m
}

// GetTypeID for serial registry.
func (m *MarkdownView) GetTypeID() string { return "markdownview" }

// SetMarkdown parses md and renders it into displayable lines.
func (m *MarkdownView) SetMarkdown(md string) {
	m.source = []byte(md)
	m.lines = parse(md)
	m.Top = 0
	m.refreshScroll()
}

func (m *MarkdownView) refreshScroll() {
	if m.VScroll != nil {
		m.VScroll.SetRange(0, len(m.lines))
		m.VScroll.SetValue(m.Top)
	}
}

// parse converts md into a flat list of renderedLines. Block-level
// constructs (headings, bullets, code blocks, hr) are recognized
// here; inline emphasis and code are applied per-line.
func parse(md string) []renderedLine {
	headingAttr := types.MakeAttr(0x0E, 0x01)
	textAttr := types.MakeAttr(0x07, 0x01)
	bulletAttr := types.MakeAttr(0x0B, 0x01)
	quoteAttr := types.MakeAttr(0x06, 0x01)
	codeAttr := types.MakeAttr(0x0A, 0x01)
	ruleAttr := types.MakeAttr(0x08, 0x01)
	emphAttr := types.MakeAttr(0x0F, 0x01)
	linkAttr := types.MakeAttr(0x09, 0x01)

	var out []renderedLine
	inFence := false
	fenceAttr := codeAttr

	for _, raw := range strings.Split(md, "\n") {
		// Fenced code block toggle.
		if strings.HasPrefix(raw, "```") {
			inFence = !inFence
			out = append(out, renderedLine{Text: raw, Spans: nil})
			continue
		}
		if inFence {
			out = append(out, renderedLine{
				Text:  raw,
				Spans: []span{{Start: 0, End: len(raw), Attr: fenceAttr}},
			})
			continue
		}
		// Horizontal rule.
		trim := strings.TrimSpace(raw)
		if trim == "---" || trim == "***" || trim == "___" {
			out = append(out, renderedLine{
				Text:  strings.Repeat("─", 60),
				Spans: []span{{Start: 0, End: len(strings.Repeat("─", 60)), Attr: ruleAttr}},
			})
			continue
		}
		// Heading.
		if hashes := countLeading(raw, '#'); hashes >= 1 && hashes <= 6 && hashes < len(raw) && raw[hashes] == ' ' {
			text := raw[hashes+1:]
			out = append(out, renderedLine{
				Text:  text,
				Spans: []span{{Start: 0, End: len(text), Attr: headingAttr}},
			})
			continue
		}
		// Block quote.
		if strings.HasPrefix(raw, "> ") {
			text := "│ " + raw[2:]
			out = append(out, renderedLine{
				Text:  text,
				Spans: []span{{Start: 0, End: len(text), Attr: quoteAttr}},
			})
			continue
		}
		// Bullet list.
		if strings.HasPrefix(raw, "- ") || strings.HasPrefix(raw, "* ") {
			text := "• " + raw[2:]
			rl := renderedLine{Text: text}
			rl.Spans = append(rl.Spans, span{Start: 0, End: len("• "), Attr: bulletAttr})
			rl.Spans = append(rl.Spans, applyInline(text, len("• "), len(text), textAttr, emphAttr, codeAttr, linkAttr)...)
			out = append(out, rl)
			continue
		}
		// Numbered list.
		if i := scanNumberedListPrefix(raw); i > 0 {
			rl := renderedLine{Text: raw}
			rl.Spans = append(rl.Spans, span{Start: 0, End: i, Attr: bulletAttr})
			rl.Spans = append(rl.Spans, applyInline(raw, i, len(raw), textAttr, emphAttr, codeAttr, linkAttr)...)
			out = append(out, rl)
			continue
		}
		// Plain paragraph line.
		rl := renderedLine{Text: raw}
		rl.Spans = applyInline(raw, 0, len(raw), textAttr, emphAttr, codeAttr, linkAttr)
		out = append(out, rl)
	}
	return out
}

// countLeading counts how many of c appear at the start of s.
func countLeading(s string, c byte) int {
	n := 0
	for n < len(s) && s[n] == c {
		n++
	}
	return n
}

// scanNumberedListPrefix returns the byte index just after a "12. "
// numbered-list marker, or 0 if s doesn't start with one.
func scanNumberedListPrefix(s string) int {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(s) || s[i] != '.' || s[i+1] != ' ' {
		return 0
	}
	return i + 2
}

// applyInline produces spans for **bold**, *italic*, `code`, and
// [link](url) within s[start:end]. The remainder is colored textAttr.
func applyInline(s string, start, end int, textAttr, emphAttr, codeAttr, linkAttr uint16) []span {
	out := []span{{Start: start, End: end, Attr: textAttr}}
	overlay := func(s, e int, attr uint16) {
		// Replace any plain-text overlap in `out` with the new span,
		// keeping the rest of the line styled.
		var next []span
		for _, sp := range out {
			if sp.End <= s || sp.Start >= e {
				next = append(next, sp)
				continue
			}
			if sp.Start < s {
				next = append(next, span{Start: sp.Start, End: s, Attr: sp.Attr})
			}
			if sp.End > e {
				next = append(next, span{Start: e, End: sp.End, Attr: sp.Attr})
			}
		}
		next = append(next, span{Start: s, End: e, Attr: attr})
		out = next
	}
	scanRange := func(open, close string, attr uint16) {
		i := start
		for i < end {
			j := strings.Index(s[i:end], open)
			if j < 0 {
				return
			}
			j += i
			k := strings.Index(s[j+len(open):end], close)
			if k < 0 {
				return
			}
			k += j + len(open)
			overlay(j, k+len(close), attr)
			i = k + len(close)
		}
	}
	scanRange("**", "**", emphAttr)
	scanRange("*", "*", emphAttr)
	scanRange("`", "`", codeAttr)
	// [text](url)
	i := start
	for i < end {
		lb := strings.Index(s[i:end], "[")
		if lb < 0 {
			break
		}
		lb += i
		rb := strings.Index(s[lb:end], "]")
		if rb < 0 {
			break
		}
		rb += lb
		if rb+1 >= end || s[rb+1] != '(' {
			i = rb + 1
			continue
		}
		rp := strings.Index(s[rb:end], ")")
		if rp < 0 {
			break
		}
		rp += rb
		overlay(lb, rp+1, linkAttr)
		i = rp + 1
	}
	return out
}

// Draw paints visible lines.
func (m *MarkdownView) Draw() {
	for r := 0; r < m.Size.Y; r++ {
		buf := screen.MakeDrawBuffer(m.Size.X)
		bg := types.MakeAttr(0x07, 0x01)
		for x := 0; x < m.Size.X; x++ {
			screen.DrawCell(buf, x, " ", bg)
		}
		idx := m.Top + r
		if idx >= 0 && idx < len(m.lines) {
			line := m.lines[idx]
			text := utf8.CopyDisplayCells(line.Text, 0, m.Size.X)
			screen.DrawStr(buf, 0, text, bg)
			// Re-paint colored spans on top.
			for _, sp := range line.Spans {
				if sp.Start >= len(line.Text) {
					continue
				}
				e := sp.End
				if e > len(line.Text) {
					e = len(line.Text)
				}
				piece := line.Text[sp.Start:e]
				screen.DrawStr(buf, sp.Start, utf8.CopyDisplayCells(piece, 0, m.Size.X-sp.Start), sp.Attr)
			}
		}
		m.WriteLine(0, r, m.Size.X, 1, buf)
	}
}

// HandleEvent: arrows / wheel / pageup / pagedown.
func (m *MarkdownView) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		if ev.Buttons&(consts.MbScrollWheelUp|consts.MbScrollWheelDown) != 0 {
			step := 3
			if ev.Buttons&consts.MbScrollWheelUp != 0 {
				step = -step
			}
			m.scrollBy(step)
			m.ClearEvent(ev)
			return
		}
		if m.Owner != nil {
			m.Owner.Focus(m.Self())
		}
		m.ClearEvent(ev)
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbUp:
		m.scrollBy(-1)
	case consts.KbDown:
		m.scrollBy(+1)
	case consts.KbPgUp:
		m.scrollBy(-m.Size.Y)
	case consts.KbPgDn:
		m.scrollBy(+m.Size.Y)
	case consts.KbHome:
		m.Top = 0
	case consts.KbEnd:
		m.Top = len(m.lines) - m.Size.Y
		if m.Top < 0 {
			m.Top = 0
		}
	default:
		return
	}
	m.refreshScroll()
	m.Draw()
	m.ClearEvent(ev)
}

func (m *MarkdownView) scrollBy(d int) {
	m.Top += d
	if m.Top < 0 {
		m.Top = 0
	}
	max := len(m.lines) - m.Size.Y
	if max < 0 {
		max = 0
	}
	if m.Top > max {
		m.Top = max
	}
	m.refreshScroll()
	m.Draw()
}
