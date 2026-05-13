package markdown

import (
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/clipboard"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/utf8"
)

func mouseEv(what uint16, buttons byte, x, y int) *drivers.Event {
	return &drivers.Event{What: what, Buttons: buttons, Where: geom.Point{X: x, Y: y}}
}

// helper to find the first rendered line whose Text matches a needle.
func findLine(lines []renderedLine, contains string) *renderedLine {
	for i := range lines {
		if strings.Contains(lines[i].Text, contains) {
			return &lines[i]
		}
	}
	return nil
}

// TestInlineMarkersHidden verifies that bold/italic/code markers are
// stripped from the visible text — the user shouldn't see `**` or
// `_` or “ ` “ in the rendered output.
func TestInlineMarkersHidden(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"**bold**", "bold"},
		{"__bold__", "bold"},
		{"*italic*", "italic"},
		{"_italic_", "italic"},
		{"`code`", "code"},
		{"~~strike~~", "strike"},
		{"plain **bold** more", "plain bold more"},
		{"[link text](https://example.com)", "link text"},
		{`escape \*not bold\*`, "escape *not bold*"},
	}
	for _, tc := range cases {
		lines := parse(tc.in)
		if len(lines) == 0 {
			t.Errorf("parse(%q) returned no lines", tc.in)
			continue
		}
		if lines[0].Text != tc.want {
			t.Errorf("parse(%q).Text = %q, want %q", tc.in, lines[0].Text, tc.want)
		}
	}
}

// TestPlusBullet confirms "+ item" is recognized as a bullet.
func TestPlusBullet(t *testing.T) {
	lines := parse("+ apple\n+ banana\n")
	if len(lines) < 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0].Text, "• apple") {
		t.Errorf("+ bullet not rendered: %q", lines[0].Text)
	}
}

// TestFenceSwallowed verifies fence lines (``` and ```go) don't
// appear in the output — only the code between them.
func TestFenceSwallowed(t *testing.T) {
	md := "```go\nfunc f() {}\n```\n"
	lines := parse(md)
	if findLine(lines, "```") != nil {
		t.Errorf("fence marker survived rendering: %+v", lines)
	}
	if findLine(lines, "func f()") == nil {
		t.Errorf("fence content missing: %+v", lines)
	}
}

// TestSetextHeading verifies that text followed by === gets the
// heading attribute applied retroactively.
func TestSetextHeading(t *testing.T) {
	lines := parse("Title\n=====\n\nBody text\n")
	if len(lines) == 0 {
		t.Fatal("no lines parsed")
	}
	if lines[0].Text != "Title" {
		t.Errorf("first line = %q, want 'Title'", lines[0].Text)
	}
	if len(lines[0].Spans) == 0 {
		t.Fatalf("Title should have heading span, got %+v", lines[0].Spans)
	}
	// The === line itself should NOT appear in output.
	if findLine(lines, "===") != nil {
		t.Errorf("setext underline survived rendering: %+v", lines)
	}
}

// TestATXTrailingHashes verifies "# Heading #" → "Heading".
func TestATXTrailingHashes(t *testing.T) {
	lines := parse("# Hello ###\n")
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	if lines[0].Text != "Hello" {
		t.Errorf("trailing hashes not stripped: %q", lines[0].Text)
	}
}

// TestAutolink verifies <https://...> renders without the angle brackets.
func TestAutolink(t *testing.T) {
	lines := parse("See <https://example.com> for details\n")
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	if strings.Contains(lines[0].Text, "<") || strings.Contains(lines[0].Text, ">") {
		t.Errorf("autolink brackets not stripped: %q", lines[0].Text)
	}
	if !strings.Contains(lines[0].Text, "https://example.com") {
		t.Errorf("autolink content missing: %q", lines[0].Text)
	}
}

// TestEmptyBlockquote verifies "> " (just the marker) or ">" alone
// renders the quote prefix without crashing.
func TestEmptyBlockquote(t *testing.T) {
	lines := parse("> first line\n>\n> after blank")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %+v", len(lines), lines)
	}
	for i, l := range lines {
		if !strings.HasPrefix(l.Text, "│") {
			t.Errorf("line %d missing quote prefix: %q", i, l.Text)
		}
	}
}

// TestIndentedCodeBlock verifies that a 4-space-indented line outside
// a list is treated as code (rendered without the leading spaces).
func TestIndentedCodeBlock(t *testing.T) {
	lines := parse("    int main() {\n    return 0;\n    }\n")
	if findLine(lines, "int main()") == nil {
		t.Errorf("indented code block not recognized: %+v", lines)
	}
	// Should NOT start with 4 spaces (those got consumed by the marker).
	if l := findLine(lines, "int main()"); l != nil && strings.HasPrefix(l.Text, "    ") {
		t.Errorf("indented code block kept leading spaces: %q", l.Text)
	}
}

// TestEscapedAsterisk verifies backslash escapes survive into the
// rendered text and DON'T trigger emphasis.
func TestEscapedAsterisk(t *testing.T) {
	lines := parse(`literal \*stars\*`)
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	if lines[0].Text != "literal *stars*" {
		t.Errorf("escape failed: got %q", lines[0].Text)
	}
}

// TestUnderscoreInsideWord verifies that `snake_case_var` isn't
// chopped up as italic — _ inside a word boundary must stay literal.
func TestUnderscoreInsideWord(t *testing.T) {
	lines := parse("the snake_case_var name\n")
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	if !strings.Contains(lines[0].Text, "snake_case_var") {
		t.Errorf("underscore-in-word got mangled: %q", lines[0].Text)
	}
}

// TestItalicGetsEAItalic verifies that `*italic*` spans carry the
// EAItalic ext-attr so terminals actually slant the glyphs (SGR 3),
// while `**bold**` does NOT — bold is conveyed by color alone so the
// two visually differ.
func TestItalicGetsEAItalic(t *testing.T) {
	lines := parse("plain *slanted* and **bold**\n")
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	var italicFound, boldFound bool
	for _, sp := range lines[0].Spans {
		piece := lines[0].Text[sp.Start:sp.End]
		if piece == "slanted" {
			italicFound = true
			if sp.ExtAttrs&types.EAItalic == 0 {
				t.Errorf("italic span missing EAItalic, ExtAttrs=%#x", sp.ExtAttrs)
			}
		}
		if piece == "bold" {
			boldFound = true
			if sp.ExtAttrs&types.EAItalic != 0 {
				t.Errorf("bold span unexpectedly has EAItalic, ExtAttrs=%#x", sp.ExtAttrs)
			}
		}
	}
	if !italicFound {
		t.Error("italic span not found")
	}
	if !boldFound {
		t.Error("bold span not found")
	}
}

// TestStrikethroughGetsEA confirms ~~strike~~ carries EAStrikethrough.
func TestStrikethroughGetsEA(t *testing.T) {
	lines := parse("a ~~gone~~ b\n")
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	for _, sp := range lines[0].Spans {
		if lines[0].Text[sp.Start:sp.End] == "gone" {
			if sp.ExtAttrs&types.EAStrikethrough == 0 {
				t.Errorf("strike span missing EAStrikethrough, ExtAttrs=%#x", sp.ExtAttrs)
			}
			return
		}
	}
	t.Error("strike span not found")
}

// TestLinkURLPropagates verifies [text](url) and <https://...> both
// store the URL on the span so OSC 8 makes them clickable.
func TestLinkURLPropagates(t *testing.T) {
	lines := parse("Visit [project](https://example.com/p) or <https://github.com>.\n")
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	var sawBracket, sawAuto bool
	for _, sp := range lines[0].Spans {
		piece := lines[0].Text[sp.Start:sp.End]
		if piece == "project" {
			sawBracket = true
			if sp.URL != "https://example.com/p" {
				t.Errorf("[link](url) URL = %q, want https://example.com/p", sp.URL)
			}
		}
		if piece == "https://github.com" {
			sawAuto = true
			if sp.URL != "https://github.com" {
				t.Errorf("autolink URL = %q, want https://github.com", sp.URL)
			}
		}
	}
	if !sawBracket {
		t.Error("bracket link span not found")
	}
	if !sawAuto {
		t.Error("autolink span not found")
	}
}

// TestEmailAutolinkMailto verifies <user@host> picks up a mailto: URL.
func TestEmailAutolinkMailto(t *testing.T) {
	lines := parse("ping <a@b.com>\n")
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	for _, sp := range lines[0].Spans {
		if lines[0].Text[sp.Start:sp.End] == "a@b.com" {
			if sp.URL != "mailto:a@b.com" {
				t.Errorf("email autolink URL = %q, want mailto:a@b.com", sp.URL)
			}
			return
		}
	}
	t.Error("email autolink span not found")
}

// TestTableCellsParseInline verifies that **bold**, *italic*, `code`,
// and [link](url) inside a table cell are processed: the markers are
// stripped from the rendered text and the cell width sizes to the
// visible content.
func TestTableCellsParseInline(t *testing.T) {
	md := "| Name | Note |\n" +
		"| ---- | ---- |\n" +
		"| **a** | `x` |\n" +
		"| [home](https://h) | *y* |\n"
	lines := parse(md)
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	allText := ""
	for _, l := range lines {
		allText += l.Text + "\n"
	}
	// Markers must NOT appear in the rendered table text.
	for _, bad := range []string{"**a**", "[home]", "(https://h)", "`x`", "*y*"} {
		if strings.Contains(allText, bad) {
			t.Errorf("table left raw marker %q in output:\n%s", bad, allText)
		}
	}
	// Cleaned content must.
	for _, good := range []string{"a", "home", "x", "y"} {
		if !strings.Contains(allText, good) {
			t.Errorf("table missing cleaned content %q:\n%s", good, allText)
		}
	}
	// The "home" cell should carry a URL span — confirms the
	// hyperlink survived the table layer.
	var sawURL bool
	for _, l := range lines {
		for _, sp := range l.Spans {
			if sp.URL == "https://h" {
				sawURL = true
			}
		}
	}
	if !sawURL {
		t.Error("table cell link URL did not propagate")
	}
}

// TestSelectionText covers the drag-to-select path: a multi-line
// selection should join the chosen visible runs with '\n', honoring
// display columns (not bytes) so the selection survives wide /
// multi-byte runes intact.
func TestSelectionText(t *testing.T) {
	m := &MarkdownView{}
	m.lines = parse("alpha\nbeta gamma\ndelta\n")
	// Select from (line=0, col=2) through (line=2, col=3) — that's
	// "pha\nbeta gamma\ndel".
	m.selStart = mdPos{line: 0, col: 2}
	m.selEnd = mdPos{line: 2, col: 3}
	m.hasSel = true
	got := m.selectionText()
	want := "pha\nbeta gamma\ndel"
	if got != want {
		t.Errorf("selectionText() = %q, want %q", got, want)
	}
}

// TestSelectionTextSingleLine confirms a within-line selection emits
// just the chosen substring with no trailing newline.
func TestSelectionTextSingleLine(t *testing.T) {
	m := &MarkdownView{}
	m.lines = parse("hello world\n")
	m.selStart = mdPos{line: 0, col: 6}
	m.selEnd = mdPos{line: 0, col: 11}
	m.hasSel = true
	if got := m.selectionText(); got != "world" {
		t.Errorf("selectionText() = %q, want %q", got, "world")
	}
}

// TestSelectionTextEmpty returns "" when no selection is active.
func TestSelectionTextEmpty(t *testing.T) {
	m := &MarkdownView{}
	m.lines = parse("hi\n")
	if got := m.selectionText(); got != "" {
		t.Errorf("expected empty selectionText, got %q", got)
	}
}

// TestDragSelectCopies drives a synthetic mouse drag through
// HandleEvent and asserts that mouse-up populates the clipboard with
// the selected text — the contract fvmux + other hosts rely on.
func TestDragSelectCopies(t *testing.T) {
	m := New(geom.NewRect(0, 0, 40, 6), nil)
	m.State |= consts.SfVisible | consts.SfExposed
	m.SetMarkdown("Alpha bravo charlie\nDelta echo foxtrot\n")
	clipboard.SetText("") // reset
	m.HandleEvent(mouseEv(consts.EvMouseDown, consts.MbLeftButton, 6, 0))
	m.HandleEvent(mouseEv(consts.EvMouseMove, consts.MbLeftButton, 10, 1))
	m.HandleEvent(mouseEv(consts.EvMouseUp, 0, 10, 1))
	got := clipboard.GetText()
	want := "bravo charlie\nDelta echo"
	if got != want {
		t.Errorf("clipboard after drag = %q, want %q", got, want)
	}
}

// TestSmartCopyLinkURL: selecting only the visible label of an inline
// link copies the URL, not the label. This is the headline payoff of
// the "smart per-context" copy rule.
func TestSmartCopyLinkURL(t *testing.T) {
	m := New(geom.NewRect(0, 0, 60, 4), nil)
	m.State |= consts.SfVisible | consts.SfExposed
	m.SetMarkdown("[the project](https://example.com/p)\n")
	// The first rendered line is "the project". Select all 11 cells.
	m.selStart = mdPos{line: 0, col: 0}
	m.selEnd = mdPos{line: 0, col: 11}
	m.hasSel = true
	if got := m.currentCopyText(); got != "https://example.com/p" {
		t.Errorf("currentCopyText() = %q, want URL", got)
	}
}

// TestSmartCopyAutolinkURL: a drag inside an autolink yields the
// underlying URL.
func TestSmartCopyAutolinkURL(t *testing.T) {
	m := New(geom.NewRect(0, 0, 60, 4), nil)
	m.State |= consts.SfVisible | consts.SfExposed
	m.SetMarkdown("<https://github.com>\n")
	m.selStart = mdPos{line: 0, col: 0}
	m.selEnd = mdPos{line: 0, col: 18}
	m.hasSel = true
	if got := m.currentCopyText(); got != "https://github.com" {
		t.Errorf("currentCopyText() = %q, want autolink URL", got)
	}
}

// TestSmartCopyMixedFallsBack: a selection that spans both link and
// non-link cells (e.g., "Visit [link]" plus trailing prose) falls
// back to rendered text — copying just the URL would lose context.
func TestSmartCopyMixedFallsBack(t *testing.T) {
	m := New(geom.NewRect(0, 0, 60, 4), nil)
	m.State |= consts.SfVisible | consts.SfExposed
	m.SetMarkdown("Visit [the project](https://example.com/p) today\n")
	// Whole rendered line: "Visit the project today" (23 cells).
	m.selStart = mdPos{line: 0, col: 0}
	m.selEnd = mdPos{line: 0, col: 23}
	m.hasSel = true
	got := m.currentCopyText()
	if got != "Visit the project today" {
		t.Errorf("currentCopyText() = %q, want rendered prose", got)
	}
}

// TestSmartCopyPlainProse: no URL anywhere → plain rendered text.
func TestSmartCopyPlainProse(t *testing.T) {
	m := New(geom.NewRect(0, 0, 60, 4), nil)
	m.State |= consts.SfVisible | consts.SfExposed
	m.SetMarkdown("just words here\n")
	m.selStart = mdPos{line: 0, col: 0}
	m.selEnd = mdPos{line: 0, col: 15}
	m.hasSel = true
	if got := m.currentCopyText(); got != "just words here" {
		t.Errorf("currentCopyText() = %q, want plain prose", got)
	}
}

// TestSmartCopyImageURL: dragging across the rendered image cells
// (the alt text) copies the image URL, since the alt is rarely what
// you want to paste — you want the file path.
func TestSmartCopyImageURL(t *testing.T) {
	m := New(geom.NewRect(0, 0, 60, 4), nil)
	m.State |= consts.SfVisible | consts.SfExposed
	m.SetMarkdown("![Diagram](images/diagram.png)\n")
	// Rendered as "🖼 Diagram" (🖼 is a wide rune → 2 cells, plus space + 7).
	endCol := utf8.StringDisplayWidth(m.lines[0].Text)
	m.selStart = mdPos{line: 0, col: 0}
	m.selEnd = mdPos{line: 0, col: endCol}
	m.hasSel = true
	if got := m.currentCopyText(); got != "images/diagram.png" {
		t.Errorf("currentCopyText() = %q, want image URL", got)
	}
}

// TestSmartCopyAdjacentLinksFallBack: two distinct links touching
// each other in the selection must NOT collapse to one URL — that
// would be wrong (which URL?). Fall back to text.
func TestSmartCopyAdjacentLinksFallBack(t *testing.T) {
	m := New(geom.NewRect(0, 0, 60, 4), nil)
	m.State |= consts.SfVisible | consts.SfExposed
	m.SetMarkdown("[a](https://1) [b](https://2)\n")
	// Rendered: "a b" (3 cells).
	m.selStart = mdPos{line: 0, col: 0}
	m.selEnd = mdPos{line: 0, col: 3}
	m.hasSel = true
	if got := m.currentCopyText(); got != "a b" {
		t.Errorf("currentCopyText() = %q, want rendered text fallback", got)
	}
}

// TestSelectAll selects every cell in the document.
func TestSelectAll(t *testing.T) {
	m := &MarkdownView{}
	m.lines = parse("one\ntwo\nthree\n")
	m.selectAll()
	if !m.hasSel {
		t.Fatal("selectAll did not set hasSel")
	}
	got := m.selectionText()
	want := "one\ntwo\nthree\n"
	if got != want {
		t.Errorf("select-all text = %q, want %q", got, want)
	}
}

// TestImageRender confirms ![alt](url) renders as 🖼 alt (no url).
func TestImageRender(t *testing.T) {
	lines := parse("![Diagram](pic.png)\n")
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	if !strings.Contains(lines[0].Text, "Diagram") {
		t.Errorf("image alt missing: %q", lines[0].Text)
	}
	if strings.Contains(lines[0].Text, "pic.png") {
		t.Errorf("image url leaked into render: %q", lines[0].Text)
	}
}
