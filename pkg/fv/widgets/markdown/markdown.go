// Package markdown provides MarkdownView — a read-only widget that
// renders a useful subset of CommonMark / GFM:
//
//   - ATX headings (#..######) with optional trailing #'s stripped
//   - Setext headings (text followed by === or ---)
//   - Paragraph text with **bold**, __bold__, *italic*, _italic_,
//     `inline code`, ~~strikethrough~~, [link text](url),
//     ![image](url), <https://autolink>, <user@email.autolink>, and
//     backslash escapes (\* \_ \\ etc.)
//   - Fenced code blocks (``` …) with optional language hint that is
//     visually consumed (the fence lines themselves don't render)
//   - Indented code blocks (4+ leading spaces or a leading tab)
//   - Bullet lists with - / * / +
//   - Numbered lists (1. 2. 3.)
//   - Block quotes (> …, including the empty > line)
//   - Horizontal rules (--- / *** / ___)
//   - Tabs expanded to 4 spaces at line start
//
// Inline markers are stripped from the visible output — "**bold**"
// renders as "bold" with the bold attribute, not the verbatim string.
//
// Ported from MarkdownView.pas. The Pascal version uses a regex-driven
// pre-pass; this Go port is a hand-rolled line scanner.
package markdown

import (
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/clipboard"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
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

	// Drag-to-select state. selecting is true while the user holds
	// the left mouse button after pressing inside the view; hasSel
	// stays true after release until the next click clears it.
	// Positions are (line, column) where column is a display column
	// (so wide / multi-byte runes are counted as cells, matching the
	// terminal's selection coords).
	selecting bool
	hasSel    bool
	selStart  mdPos
	selEnd    mdPos
}

// mdPos addresses a cell inside the rendered line list.
type mdPos struct {
	line int // 0-based index into m.lines
	col  int // display column inside that line
}

// renderedLine is a pre-formatted display line (after MD parsing).
// Spans are byte ranges within Text and the attribute to paint them.
// Bytes (not columns) are used for span endpoints because Text holds
// UTF-8; Draw converts byte offsets to column offsets at paint time.
type renderedLine struct {
	Text  string
	Spans []span
}

// span carries the color attribute plus optional cell-level extras:
// ExtAttrs (EAItalic / EAStrikethrough / underline-style nibble) and
// URL (OSC 8 hyperlink, so link cells light up clickable in
// terminals that honor the protocol — iTerm2, WezTerm, recent
// gnome-terminal, mintty, Windows Terminal).
type span struct {
	Start, End int
	Attr       uint16
	ExtAttrs   byte
	URL        string
}

// New constructs a viewer.
func New(bounds geom.Rect, v *views.ScrollBar) *MarkdownView {
	m := &MarkdownView{Base: views.NewBase(bounds), VScroll: v}
	m.SetSelf(m)
	m.Options |= consts.OfSelectable
	m.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	// Subscribe to mouse-move + mouse-up too, not just mouse-down,
	// so drag-to-select can extend the selection between press and
	// release.
	m.EventMask = consts.EvKeyDown | consts.EvCommand |
		consts.EvMouseDown | consts.EvMouseUp | consts.EvMouseMove
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
// constructs (headings, bullets, code blocks, hr, tables) are
// recognized here; inline emphasis and code are applied per-line via
// parseInline.
func parse(md string) []renderedLine {
	pal := theme.Get()
	headingAttr := pal.MarkdownHeading
	bulletAttr := pal.MarkdownBullet
	quoteAttr := pal.MarkdownQuote
	codeAttr := pal.MarkdownCode
	ruleAttr := pal.MarkdownRule
	tableHeaderAttr := pal.GridPinned
	tableBodyAttr := pal.GridCell
	tableFrameAttr := pal.EditorComment

	var out []renderedLine
	inFence := false
	rawLines := strings.Split(md, "\n")
	// Expand leading tabs so indented-code detection lines up with
	// CommonMark's "4 spaces" rule. Tabs further inside a line are
	// left alone — they're rare and would need a column-aware expand.
	for i, ln := range rawLines {
		if strings.HasPrefix(ln, "\t") {
			rawLines[i] = strings.Replace(ln, "\t", "    ", 1)
		}
	}

	for li := 0; li < len(rawLines); li++ {
		raw := rawLines[li]

		// Fence open/close. We swallow the fence line itself: rendering
		// "```" makes the output noisy and looks like a content row.
		if t := strings.TrimSpace(raw); !inFence && strings.HasPrefix(t, "```") {
			inFence = true
			continue
		}
		if inFence {
			if strings.TrimSpace(raw) == "```" {
				inFence = false
				continue
			}
			out = append(out, renderedLine{
				Text:  raw,
				Spans: []span{{Start: 0, End: len(raw), Attr: codeAttr}},
			})
			continue
		}

		// Tables.
		if li+1 < len(rawLines) && looksLikeTableHeader(raw) && isTableSeparator(rawLines[li+1]) {
			rows, aligns, consumed := readTable(rawLines[li:])
			out = append(out, renderTable(rows, aligns, pal, tableHeaderAttr, tableBodyAttr, tableFrameAttr)...)
			li += consumed - 1
			continue
		}

		// Indented code block (4+ spaces). Detected only on a line
		// that isn't a list continuation; here we keep it simple and
		// treat any 4-space indent outside lists as code.
		if strings.HasPrefix(raw, "    ") && !looksLikeListItem(strings.TrimLeft(raw, " ")) {
			text := raw[4:]
			out = append(out, renderedLine{
				Text:  text,
				Spans: []span{{Start: 0, End: len(text), Attr: codeAttr}},
			})
			continue
		}

		// Setext heading vs. horizontal rule: when the immediately
		// preceding output line is non-empty paragraph text, `===` or
		// `---` is a setext underline and we retro-style that previous
		// line as a heading. Otherwise the same characters form a
		// horizontal rule. Must check setext FIRST or we'd swallow the
		// underline as an HR before it has a chance to upgrade the
		// paragraph above.
		trim := strings.TrimSpace(raw)
		if li > 0 && len(out) > 0 && isSetextUnderline(trim) {
			prev := &out[len(out)-1]
			if strings.TrimSpace(prev.Text) != "" && !isPrevDecorated(prev) {
				prev.Spans = []span{{Start: 0, End: len(prev.Text), Attr: headingAttr}}
				continue
			}
		}
		if isHorizontalRule(trim) {
			rule := strings.Repeat("─", 60)
			out = append(out, renderedLine{
				Text:  rule,
				Spans: []span{{Start: 0, End: len(rule), Attr: ruleAttr}},
			})
			continue
		}

		// ATX heading.
		if hashes := countLeading(raw, '#'); hashes >= 1 && hashes <= 6 && hashes < len(raw) && raw[hashes] == ' ' {
			text := stripTrailingHashes(raw[hashes+1:])
			out = append(out, renderedLine{
				Text:  text,
				Spans: []span{{Start: 0, End: len(text), Attr: headingAttr}},
			})
			continue
		}

		// Block quote — "> text" or just ">" for an empty quote line.
		if raw == ">" || strings.HasPrefix(raw, "> ") {
			body := ""
			if len(raw) > 1 {
				body = raw[2:]
			}
			cleaned, inner := parseInline(body, pal)
			text := "│ " + cleaned
			rl := renderedLine{Text: text}
			rl.Spans = append(rl.Spans, span{Start: 0, End: len("│ "), Attr: quoteAttr})
			// Inner spans shift by the "│ " prefix length.
			for _, sp := range inner {
				rl.Spans = append(rl.Spans, span{
					Start: sp.Start + len("│ "),
					End:   sp.End + len("│ "),
					Attr:  sp.Attr,
				})
			}
			// Color any plain stretch with quoteAttr too so the line
			// reads as a single quote block (not pop-out paragraph).
			rl.Spans = append(rl.Spans, span{
				Start: len("│ "),
				End:   len(text),
				Attr:  quoteAttr,
			})
			out = append(out, rl)
			continue
		}

		// Bullet list: - / * / + prefix.
		if isBulletPrefix(raw) {
			body := raw[2:]
			cleaned, inner := parseInline(body, pal)
			text := "• " + cleaned
			rl := renderedLine{Text: text}
			rl.Spans = append(rl.Spans, span{Start: 0, End: len("• "), Attr: bulletAttr})
			for _, sp := range inner {
				rl.Spans = append(rl.Spans, span{
					Start: sp.Start + len("• "),
					End:   sp.End + len("• "),
					Attr:  sp.Attr,
				})
			}
			out = append(out, rl)
			continue
		}

		// Numbered list.
		if i := scanNumberedListPrefix(raw); i > 0 {
			body := raw[i:]
			cleaned, inner := parseInline(body, pal)
			text := raw[:i] + cleaned
			rl := renderedLine{Text: text}
			rl.Spans = append(rl.Spans, span{Start: 0, End: i, Attr: bulletAttr})
			for _, sp := range inner {
				rl.Spans = append(rl.Spans, span{
					Start: sp.Start + i,
					End:   sp.End + i,
					Attr:  sp.Attr,
				})
			}
			out = append(out, rl)
			continue
		}

		// Plain paragraph line.
		cleaned, inner := parseInline(raw, pal)
		out = append(out, renderedLine{Text: cleaned, Spans: inner})
	}
	return out
}

// parseInline walks raw, hides inline markdown delimiters, and emits
// the visible (cleaned) text plus a list of styled byte-ranges inside
// that cleaned text. Nested emphasis is parsed recursively so
// "**outer *inner* outer**" gets both styles applied where they
// overlap.
//
// Markers handled:
//
//	**bold**, __bold__, *italic*, _italic_, `code`, ~~strike~~,
//	[text](url), ![alt](url), <auto://link>, <user@host>, and
//	backslash escapes (\* \_ \\ \` etc.).
func parseInline(raw string, pal *theme.Palette) (string, []span) {
	var b strings.Builder
	var spans []span

	// Find the byte index of close inside raw[from:], stopping at
	// raw's end. Skips backslash-escaped occurrences. Returns -1 if
	// not found.
	findClose := func(from int, close string) int {
		i := from
		for i+len(close) <= len(raw) {
			if raw[i] == '\\' && i+1 < len(raw) {
				i += 2
				continue
			}
			if raw[i:i+len(close)] == close {
				return i
			}
			i++
		}
		return -1
	}

	// Emit a sub-segment of raw under attr / ext / url, recursively
	// running parseInline on it so nested emphasis works. Nested
	// spans keep their own attrs but inherit any URL / ExtAttrs we
	// set here (so `*[italic link](url)*` is italic AND clickable).
	emit := func(inner string, attr uint16, ext byte, url string) {
		start := b.Len()
		cleanInner, innerSpans := parseInline(inner, pal)
		b.WriteString(cleanInner)
		spans = append(spans, span{Start: start, End: b.Len(), Attr: attr, ExtAttrs: ext, URL: url})
		for _, sp := range innerSpans {
			spans = append(spans, span{
				Start:    start + sp.Start,
				End:      start + sp.End,
				Attr:     sp.Attr,
				ExtAttrs: sp.ExtAttrs | ext,
				URL: func() string {
					if sp.URL != "" {
						return sp.URL
					}
					return url
				}(),
			})
		}
	}

	i := 0
	for i < len(raw) {
		c := raw[i]

		// Backslash escape.
		if c == '\\' && i+1 < len(raw) {
			b.WriteByte(raw[i+1])
			i += 2
			continue
		}

		// Inline code `…` — does NOT recurse (the contents are literal).
		if c == '`' {
			if end := strings.Index(raw[i+1:], "`"); end >= 0 {
				start := b.Len()
				b.WriteString(raw[i+1 : i+1+end])
				spans = append(spans, span{Start: start, End: b.Len(), Attr: pal.MarkdownCode})
				i = i + 1 + end + 1
				continue
			}
		}

		// Image ![alt](url) — render as "alt" prefixed with a small
		// icon to differentiate from plain links in the terminal.
		// The URL also rides along on the cells so terminals that
		// honor OSC 8 light up the alt text as clickable.
		if c == '!' && i+1 < len(raw) && raw[i+1] == '[' {
			if rb := strings.Index(raw[i+2:], "]"); rb >= 0 {
				close := i + 2 + rb
				if close+1 < len(raw) && raw[close+1] == '(' {
					if rp := strings.Index(raw[close+2:], ")"); rp >= 0 {
						alt := raw[i+2 : close]
						url := raw[close+2 : close+2+rp]
						start := b.Len()
						b.WriteString("🖼 ")
						b.WriteString(alt)
						spans = append(spans, span{
							Start: start, End: b.Len(),
							Attr: pal.MarkdownImage, URL: url,
						})
						i = close + 2 + rp + 1
						continue
					}
				}
			}
		}

		// Link [text](url) — visible text only; the url rides on
		// HyperlinkURL so terminals that honor OSC 8 make it clickable.
		// Underline style cues the hint visually for terminals that
		// don't support OSC 8.
		if c == '[' {
			if rb := strings.Index(raw[i+1:], "]"); rb >= 0 {
				close := i + 1 + rb
				if close+1 < len(raw) && raw[close+1] == '(' {
					if rp := strings.Index(raw[close+2:], ")"); rp >= 0 {
						url := raw[close+2 : close+2+rp]
						emit(raw[i+1:close], pal.MarkdownLink, linkExt(), url)
						i = close + 2 + rp + 1
						continue
					}
				}
			}
		}

		// Autolink <url> or <email>. Same OSC 8 treatment as bracketed
		// links — the bare URL renders clickable.
		if c == '<' {
			if end := strings.Index(raw[i+1:], ">"); end > 0 {
				inner := raw[i+1 : i+1+end]
				if isAutolink(inner) {
					url := inner
					if strings.IndexByte(inner, '@') > 0 && !strings.Contains(inner, "://") {
						url = "mailto:" + inner
					}
					start := b.Len()
					b.WriteString(inner)
					spans = append(spans, span{
						Start: start, End: b.Len(),
						Attr: pal.MarkdownLink, ExtAttrs: linkExt(), URL: url,
					})
					i = i + 1 + end + 1
					continue
				}
			}
		}

		// Strong **…** / __…__. Same color as italic but no italic
		// glyph attr — the brighter palette index reads as bold.
		if (c == '*' || c == '_') && i+1 < len(raw) && raw[i+1] == c {
			marker := raw[i : i+2]
			if end := findClose(i+2, marker); end > i+2 {
				emit(raw[i+2:end], pal.MarkdownEmph, 0, "")
				i = end + 2
				continue
			}
		}

		// Emphasis *…* / _…_. Italic uses the EAItalic SGR-3 attr so
		// the glyphs actually slant; without it, italic and bold look
		// identical because both share the MarkdownEmph color.
		// Underscores require word boundaries to avoid mangling
		// identifiers like `snake_case_var`.
		if c == '*' || c == '_' {
			if c == '_' && i > 0 && isWordByte(raw[i-1]) {
				b.WriteByte(c)
				i++
				continue
			}
			if end := findClose(i+1, string(c)); end > i+1 {
				if c == '_' && end+1 < len(raw) && isWordByte(raw[end+1]) {
					// Inside a word — not emphasis.
				} else {
					emit(raw[i+1:end], pal.MarkdownEmph, types.EAItalic, "")
					i = end + 1
					continue
				}
			}
		}

		// Strikethrough ~~…~~. EAStrikethrough lets terminals that
		// honor SGR 9 draw the strike line; the dim color is a
		// fallback cue for terminals that don't.
		if c == '~' && i+1 < len(raw) && raw[i+1] == '~' {
			if end := findClose(i+2, "~~"); end > i+2 {
				emit(raw[i+2:end], pal.MarkdownStrike, types.EAStrikethrough, "")
				i = end + 2
				continue
			}
		}

		// Plain character.
		b.WriteByte(c)
		i++
	}

	return b.String(), spans
}

// isWordByte reports whether b is an identifier-class byte that
// disqualifies an adjacent `_` from acting as italic.
func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}

// linkExt returns the ExtAttrs flags for a hyperlink cell: a single
// underline so terminals without OSC 8 still hint at clickability.
func linkExt() byte {
	return types.UnderSingle << types.EAUnderShift
}

// isAutolink returns true for strings that look like URLs or emails —
// the conservative check stops "<not a url>" from being silently eaten.
func isAutolink(s string) bool {
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "ftp://") || strings.HasPrefix(s, "mailto:") {
		return true
	}
	if at := strings.IndexByte(s, '@'); at > 0 && at < len(s)-1 &&
		!strings.ContainsAny(s, " \t<>") &&
		strings.Contains(s[at+1:], ".") {
		return true
	}
	return false
}

// isBulletPrefix reports whether s starts with "- ", "* ", or "+ ".
func isBulletPrefix(s string) bool {
	return strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ") || strings.HasPrefix(s, "+ ")
}

// looksLikeListItem checks both bullet and numbered prefixes; used to
// keep indented-list items from being mis-detected as code blocks.
func looksLikeListItem(s string) bool {
	if isBulletPrefix(s) {
		return true
	}
	return scanNumberedListPrefix(s) > 0
}

// isHorizontalRule returns true for "---", "***", "___" (with at
// least three of the same char, no other content).
func isHorizontalRule(s string) bool {
	if len(s) < 3 {
		return false
	}
	c := s[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			return false
		}
	}
	return true
}

// isSetextUnderline returns true for a setext heading underline:
// at least three '=' (h1) or three '-' (h2), nothing else.
func isSetextUnderline(s string) bool {
	if len(s) < 3 {
		return false
	}
	c := s[0]
	if c != '=' && c != '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			return false
		}
	}
	return true
}

// isPrevDecorated reports whether the previous rendered line already
// carries a block-level decoration (bullet, quote, code, heading) —
// those should NOT be retroactively re-styled as a setext heading
// just because the next line happens to be `---`.
func isPrevDecorated(prev *renderedLine) bool {
	if strings.HasPrefix(prev.Text, "│") || strings.HasPrefix(prev.Text, "• ") {
		return true
	}
	// Heuristic: if the whole line is already one span (heading /
	// code / rule), don't override.
	if len(prev.Spans) == 1 && prev.Spans[0].Start == 0 && prev.Spans[0].End == len(prev.Text) {
		return true
	}
	return false
}

// stripTrailingHashes removes the optional " # # #" suffix CommonMark
// allows on ATX headings (e.g., "# Heading #").
func stripTrailingHashes(s string) string {
	s = strings.TrimRight(s, " ")
	if !strings.HasSuffix(s, "#") {
		return s
	}
	end := len(s)
	for end > 0 && s[end-1] == '#' {
		end--
	}
	// Must be preceded by a space (otherwise it's part of the heading text).
	if end > 0 && s[end-1] != ' ' {
		return s
	}
	return strings.TrimRight(s[:end], " ")
}

// looksLikeTableHeader returns true for "| Col A | Col B |"-style
// lines: leading "|", at least one column. Tolerates whitespace.
func looksLikeTableHeader(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "|") && strings.Count(s, "|") >= 2
}

// isTableSeparator returns true for the "| --- | :---: |" alignment
// row. Each cell must be non-empty dashes optionally bracketed by
// colons. The colons signal column alignment.
func isTableSeparator(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "|") {
		return false
	}
	parts := splitTableRow(s)
	if len(parts) == 0 {
		return false
	}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return false
		}
		p = strings.TrimPrefix(p, ":")
		p = strings.TrimSuffix(p, ":")
		if p == "" {
			return false
		}
		for _, c := range p {
			if c != '-' {
				return false
			}
		}
	}
	return true
}

// splitTableRow splits a "| a | b | c |" row into ["a", "b", "c"].
// Strips leading/trailing "|" and surrounding whitespace per cell.
func splitTableRow(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	parts := strings.Split(s, "|")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// readTable parses a contiguous run of table lines from start. Returns
// the parsed rows (header + body), per-column alignment hints, and the
// number of source lines consumed.
func readTable(lines []string) ([][]string, []Alignment, int) {
	header := splitTableRow(lines[0])
	sepParts := splitTableRow(lines[1])
	aligns := make([]Alignment, len(sepParts))
	for i, p := range sepParts {
		l := strings.HasPrefix(p, ":")
		r := strings.HasSuffix(p, ":")
		switch {
		case l && r:
			aligns[i] = AlignCenter
		case r:
			aligns[i] = AlignRight
		default:
			aligns[i] = AlignLeft
		}
	}
	rows := [][]string{header}
	consumed := 2
	for i := 2; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || !strings.HasPrefix(line, "|") {
			break
		}
		rows = append(rows, splitTableRow(lines[i]))
		consumed = i + 1
	}
	return rows, aligns, consumed
}

// Alignment is the column-alignment hint produced by table parsing.
type Alignment int

const (
	AlignLeft Alignment = iota
	AlignCenter
	AlignRight
)

// parsedCell holds a table cell after inline parsing: the cleaned
// visible text, the colored spans inside it, and the display column
// width. We pre-compute all three so column-width sizing can use the
// real (post-marker-stripping) widths and row rendering can shift the
// inline spans by each cell's starting offset.
type parsedCell struct {
	Clean string
	Spans []span
	Width int // display columns of Clean
}

// renderTable formats parsed rows + alignments into a set of
// renderedLines with box-drawing borders. Each cell's inline markdown
// (bold, italic, links, code, …) is parsed before width measurement so
// a cell like "**hello**" sizes to 5, not 9, and its content is bold
// in the rendered row.
func renderTable(rows [][]string, aligns []Alignment, pal *theme.Palette, headerAttr, bodyAttr, frameAttr uint16) []renderedLine {
	if len(rows) == 0 {
		return nil
	}
	ncols := 0
	for _, r := range rows {
		if len(r) > ncols {
			ncols = len(r)
		}
	}
	for len(aligns) < ncols {
		aligns = append(aligns, AlignLeft)
	}
	// Pre-parse every cell once so column widths are based on the
	// visible (cleaned) display width.
	parsed := make([][]parsedCell, len(rows))
	widths := make([]int, ncols)
	for i, r := range rows {
		parsed[i] = make([]parsedCell, ncols)
		for c := 0; c < ncols; c++ {
			cell := ""
			if c < len(r) {
				cell = r[c]
			}
			clean, sp := parseInline(cell, pal)
			w := utf8.StringDisplayWidth(clean)
			parsed[i][c] = parsedCell{Clean: clean, Spans: sp, Width: w}
			if w > widths[c] {
				widths[c] = w
			}
		}
	}
	// Minimum 3 chars per column (so "─" runs are visible).
	for c := range widths {
		if widths[c] < 3 {
			widths[c] = 3
		}
	}
	var out []renderedLine
	out = append(out, tableBorder(widths, "┌", "┬", "┐", frameAttr))
	out = append(out, tableDataRow(parsed[0], widths, aligns, headerAttr, frameAttr))
	out = append(out, tableBorder(widths, "├", "┼", "┤", frameAttr))
	for r := 1; r < len(parsed); r++ {
		out = append(out, tableDataRow(parsed[r], widths, aligns, bodyAttr, frameAttr))
	}
	out = append(out, tableBorder(widths, "└", "┴", "┘", frameAttr))
	return out
}

func tableBorder(widths []int, left, mid, right string, attr uint16) renderedLine {
	var sb strings.Builder
	sb.WriteString(left)
	for i, w := range widths {
		sb.WriteString(strings.Repeat("─", w+2))
		if i+1 < len(widths) {
			sb.WriteString(mid)
		}
	}
	sb.WriteString(right)
	text := sb.String()
	return renderedLine{
		Text:  text,
		Spans: []span{{Start: 0, End: len(text), Attr: attr}},
	}
}

// tableDataRow paints "│ cell │ cell │ cell │" for one row. Frame
// glyphs get frameAttr; cell bodies get cellAttr with each cell's
// pre-parsed inline spans overlaid on top so bold/italic/links inside
// cells render correctly.
func tableDataRow(cells []parsedCell, widths []int, aligns []Alignment, cellAttr, frameAttr uint16) renderedLine {
	var sb strings.Builder
	var spans []span

	frameStart := sb.Len()
	sb.WriteString("│")
	spans = append(spans, span{Start: frameStart, End: sb.Len(), Attr: frameAttr})

	for c, w := range widths {
		var cell parsedCell
		if c < len(cells) {
			cell = cells[c]
		}
		// Left/right padding per alignment.
		pad := w - cell.Width
		if pad < 0 {
			pad = 0
		}
		leftPad, rightPad := 0, pad
		switch aligns[c] {
		case AlignRight:
			leftPad, rightPad = pad, 0
		case AlignCenter:
			leftPad, rightPad = pad/2, pad-pad/2
		}
		// Surrounding cell margin spaces.
		bodyStart := sb.Len()
		sb.WriteString(" ")
		sb.WriteString(strings.Repeat(" ", leftPad))
		cellStart := sb.Len()
		sb.WriteString(cell.Clean)
		sb.WriteString(strings.Repeat(" ", rightPad))
		sb.WriteString(" ")
		spans = append(spans, span{Start: bodyStart, End: sb.Len(), Attr: cellAttr})
		// Inline spans (bold / italic / code / link / strike / image)
		// shifted by the cell's starting byte offset.
		for _, sp := range cell.Spans {
			spans = append(spans, span{
				Start:    cellStart + sp.Start,
				End:      cellStart + sp.End,
				Attr:     sp.Attr,
				ExtAttrs: sp.ExtAttrs,
				URL:      sp.URL,
			})
		}
		// Right frame.
		frameStart := sb.Len()
		sb.WriteString("│")
		spans = append(spans, span{Start: frameStart, End: sb.Len(), Attr: frameAttr})
	}
	return renderedLine{Text: sb.String(), Spans: spans}
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

// byteToCol converts a byte offset into s to a column offset, honoring
// multi-byte runes and wide / zero-width cell widths. Needed in Draw
// so spans (byte-indexed into the cleaned text) land in the right
// terminal cells even when the line contains CJK, emoji, etc.
func byteToCol(s string, byteOff int) int {
	if byteOff <= 0 {
		return 0
	}
	if byteOff >= len(s) {
		return utf8.StringDisplayWidth(s)
	}
	return utf8.StringDisplayWidth(s[:byteOff])
}

// Draw paints visible lines.
func (m *MarkdownView) Draw() {
	bg := theme.Get().EditorText
	selA, selB, hasSel := m.normalizedSel()
	for r := 0; r < m.Size.Y; r++ {
		buf := screen.MakeDrawBuffer(m.Size.X)
		for x := 0; x < m.Size.X; x++ {
			screen.DrawCell(buf, x, " ", bg)
		}
		idx := m.Top + r
		if idx >= 0 && idx < len(m.lines) {
			line := m.lines[idx]
			text := utf8.CopyDisplayCells(line.Text, 0, m.Size.X)
			screen.DrawStr(buf, 0, text, bg)
			// Re-paint colored spans on top. Spans are byte-indexed
			// into line.Text; convert to columns to land in the
			// right cells when the line contains wide / multi-byte
			// runes. After painting the color via DrawStr, walk the
			// cells in the range and OR-in any ExtAttrs (italic /
			// strike / underline) and set HyperlinkURL — DrawStr
			// only sets Ch + Attr, but we need the richer fields for
			// italic glyphs and OSC 8 click targets.
			for _, sp := range line.Spans {
				if sp.Start >= len(line.Text) || sp.Start >= sp.End {
					continue
				}
				e := sp.End
				if e > len(line.Text) {
					e = len(line.Text)
				}
				piece := line.Text[sp.Start:e]
				col := byteToCol(line.Text, sp.Start)
				if col >= m.Size.X {
					continue
				}
				clipped := utf8.CopyDisplayCells(piece, 0, m.Size.X-col)
				screen.DrawStr(buf, col, clipped, sp.Attr)
				if sp.ExtAttrs == 0 && sp.URL == "" {
					continue
				}
				pieceCols := utf8.StringDisplayWidth(clipped)
				for x := col; x < col+pieceCols && x < m.Size.X; x++ {
					buf[x].ExtAttrs |= sp.ExtAttrs
					if sp.URL != "" {
						buf[x].HyperlinkURL = sp.URL
					}
				}
			}
		}
		// Selection overlay. Reverse the cell's attr (fg ↔ bg) so the
		// styling (italic, bold color) of the underlying content still
		// shows but the highlighted rectangle is unmistakable. Done
		// last so it sits on top of everything else.
		if hasSel {
			line := m.Top + r
			for x := 0; x < m.Size.X; x++ {
				if inSelection(line, x, selA, selB) {
					buf[x].Attr = reverseAttr(buf[x].Attr)
				}
			}
		}
		m.WriteLine(0, r, m.Size.X, 1, buf)
	}
}

// reverseAttr flips the foreground and background nibbles of a TV-
// packed attribute byte while preserving any bright/intensity bits.
// Same flip the terminal widget uses for its drag-to-select highlight.
func reverseAttr(attr uint16) uint16 {
	fg := attr & 0x000F
	bg := (attr >> 8) & 0x000F
	fgRest := attr & 0x00F0
	bgRest := (attr >> 8) & 0x00F0
	return bg | bgRest | (fg << 8) | (fgRest << 8)
}

// inSelection reports whether display-cell (line, col) is inside the
// normalized [a, b) selection rectangle. End-exclusive on the right
// edge so a single click (start == end) yields no selected cells.
func inSelection(line, col int, a, b mdPos) bool {
	if line < a.line || line > b.line {
		return false
	}
	if line == a.line && col < a.col {
		return false
	}
	if line == b.line && col >= b.col {
		return false
	}
	return true
}

// normalizedSel returns the selection sorted top-left → bottom-right,
// plus a bool indicating whether anything is selected at all.
func (m *MarkdownView) normalizedSel() (a, b mdPos, ok bool) {
	if !m.hasSel && !m.selecting {
		return mdPos{}, mdPos{}, false
	}
	a, b = m.selStart, m.selEnd
	if a.line > b.line || (a.line == b.line && a.col > b.col) {
		a, b = b, a
	}
	if a == b {
		return mdPos{}, mdPos{}, false
	}
	return a, b, true
}

// posFromMouse maps a screen-coordinate mouse event to a (line, col)
// position. Coords past the end of the viewport / line list are
// clamped — pasting past EOL or below the last line still produces a
// valid endpoint.
func (m *MarkdownView) posFromMouse(where geom.Point) mdPos {
	local := m.MakeLocal(where)
	col := local.X
	if col < 0 {
		col = 0
	}
	if col > m.Size.X {
		col = m.Size.X
	}
	line := m.Top + local.Y
	if line < 0 {
		line = 0
	}
	if line > len(m.lines) {
		line = len(m.lines)
	}
	return mdPos{line: line, col: col}
}

// urlAtCol returns the OSC 8 URL covering display column col on line,
// or "" if no URL-bearing span covers that cell. The line's spans are
// byte-indexed into line.Text; we convert to columns to match the
// caller's coordinate system.
func urlAtCol(line renderedLine, col int) string {
	for _, sp := range line.Spans {
		if sp.URL == "" {
			continue
		}
		s := byteToCol(line.Text, sp.Start)
		e := byteToCol(line.Text, sp.End)
		if col >= s && col < e {
			return sp.URL
		}
	}
	return ""
}

// selectionURL returns a single URL when every cell inside the
// current selection is covered by spans that all share the same
// non-empty URL. Mixed selections (some plain cells, some link cells,
// or multiple distinct URLs) return "" so the caller falls back to
// rendered text. Used to make "drag-select exactly a link → clipboard
// gets the URL, not the visible label" work for links / autolinks /
// emails / images.
func (m *MarkdownView) selectionURL() string {
	a, b, ok := m.normalizedSel()
	if !ok {
		return ""
	}
	var firstURL string
	first := true
	for li := a.line; li <= b.line && li < len(m.lines); li++ {
		line := m.lines[li]
		startCol := 0
		endCol := utf8.StringDisplayWidth(line.Text)
		if li == a.line {
			startCol = a.col
		}
		if li == b.line {
			endCol = b.col
		}
		for col := startCol; col < endCol; col++ {
			u := urlAtCol(line, col)
			if first {
				firstURL = u
				first = false
				continue
			}
			if u != firstURL {
				return ""
			}
		}
	}
	return firstURL
}

// currentCopyText decides what the clipboard gets when the user
// completes a drag or fires CmCopy: the URL when the selection sits
// entirely within a single link, otherwise the rendered text.
func (m *MarkdownView) currentCopyText() string {
	if u := m.selectionURL(); u != "" {
		return u
	}
	return m.selectionText()
}

// selectionText extracts the visible text inside the current
// selection. Slicing is column-aware via utf8.CopyDisplayCells so
// wide / multi-byte runes survive intact.
func (m *MarkdownView) selectionText() string {
	a, b, ok := m.normalizedSel()
	if !ok {
		return ""
	}
	var sb strings.Builder
	for li := a.line; li <= b.line && li < len(m.lines); li++ {
		text := m.lines[li].Text
		startCol := 0
		endCol := utf8.StringDisplayWidth(text)
		if li == a.line {
			startCol = a.col
		}
		if li == b.line {
			endCol = b.col
		}
		if endCol <= startCol {
			if li != b.line {
				sb.WriteByte('\n')
			}
			continue
		}
		piece := utf8.CopyDisplayCells(text, startCol, endCol-startCol)
		sb.WriteString(piece)
		if li != b.line {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// selectAll selects the entire rendered document.
func (m *MarkdownView) selectAll() {
	if len(m.lines) == 0 {
		return
	}
	m.selStart = mdPos{line: 0, col: 0}
	last := len(m.lines) - 1
	m.selEnd = mdPos{line: last, col: utf8.StringDisplayWidth(m.lines[last].Text)}
	m.hasSel = true
	m.selecting = false
}

// HandleEvent dispatches:
//
//   - Mouse wheel: scroll
//   - Mouse-down + drag: select text; on release copy to clipboard
//     via OSC 52 (same path the terminal widget uses)
//   - CmCopy: copy current selection
//   - Ctrl+A: select all
//   - Esc: clear selection
//   - Arrows / PgUp / PgDn / Home / End: scroll
func (m *MarkdownView) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvCommand && ev.Command == consts.CmCopy {
		if text := m.currentCopyText(); text != "" {
			clipboard.SetText(text)
		}
		m.ClearEvent(ev)
		return
	}
	switch ev.What {
	case consts.EvMouseDown:
		if ev.Buttons&(consts.MbScrollWheelUp|consts.MbScrollWheelDown) != 0 {
			step := 3
			if ev.Buttons&consts.MbScrollWheelUp != 0 {
				step = -step
			}
			m.scrollBy(step)
			m.ClearEvent(ev)
			return
		}
		if ev.Buttons&consts.MbLeftButton != 0 {
			if m.Owner != nil {
				m.Owner.Focus(m.Self())
			}
			pos := m.posFromMouse(ev.Where)
			m.selStart = pos
			m.selEnd = pos
			m.selecting = true
			// Clear any previous "released" selection — start fresh.
			m.hasSel = false
			m.Draw()
			m.ClearEvent(ev)
			return
		}
		if m.Owner != nil {
			m.Owner.Focus(m.Self())
		}
		m.ClearEvent(ev)
		return
	case consts.EvMouseMove:
		if !m.selecting {
			return
		}
		m.selEnd = m.posFromMouse(ev.Where)
		m.Draw()
		m.ClearEvent(ev)
		return
	case consts.EvMouseUp:
		if !m.selecting {
			return
		}
		// Commit selection BEFORE flipping selecting=false. The
		// normalizedSel() / currentCopyText() helpers gate on
		// (hasSel || selecting); if we clear selecting first, hasSel
		// is still false from MouseDown and the snapshot is empty.
		m.selEnd = m.posFromMouse(ev.Where)
		if _, _, ok := m.normalizedSel(); ok {
			if text := m.currentCopyText(); text != "" {
				clipboard.SetText(text)
			}
			m.hasSel = true
		} else {
			m.hasSel = false
		}
		m.selecting = false
		m.Draw()
		m.ClearEvent(ev)
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	// Ctrl+A → select all. The reader puts the scan-coded form in
	// KeyCode; we match that rather than UnicodeChar so plain typing
	// of 'a' inside a focused parent still flows through.
	if ev.KeyCode == consts.KbCtrlA {
		m.selectAll()
		m.Draw()
		m.ClearEvent(ev)
		return
	}
	if ev.KeyCode == consts.KbEsc && m.hasSel {
		m.hasSel = false
		m.Draw()
		m.ClearEvent(ev)
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
