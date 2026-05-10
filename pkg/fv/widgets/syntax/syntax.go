// Package syntax is a small token-coloring engine. A Highlighter
// holds a list of rules (keyword sets, regex patterns, line/block
// comment markers). Tokenize takes a line of text and returns a list
// of (start, end, attr) spans the editor can use to color the line.
//
// This is a faithful-but-pragmatic port of SyntaxHighlight.pas: the
// Pascal version uses its own scanner; we lean on Go's `regexp`
// package which already handles UTF-8.
package syntax

import (
	"regexp"
	"sort"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/types"
)

// Span describes one colored region in a line.
type Span struct {
	Start, End int    // byte offsets [start, end)
	Attr       uint16 // packed FG/BG (use types.MakeAttr)
}

// Rule is one coloring rule. Exactly one of {Pattern, Keywords,
// Range} should be populated.
type Rule struct {
	Pattern      *regexp.Regexp // matches anywhere in the line
	Keywords     []string       // exact whole-word matches; case-insensitive if FoldKeywords is set
	Range        *RangeRule     // matches text between Start and End markers (e.g., strings, comments)
	Attr         uint16
	FoldKeywords bool
}

// RangeRule matches text between two markers on the same line. Set
// EndMarker to the empty string for "to end of line" (line comments).
type RangeRule struct {
	StartMarker string
	EndMarker   string
	Escape      string // optional escape char (e.g. "\\")
}

// Highlighter is a list of rules applied in order. Earlier rules win
// when ranges overlap.
type Highlighter struct {
	Rules []Rule

	// Default colors for plain text (used by callers when no rule
	// matches a position).
	DefaultAttr uint16
}

// Tokenize returns Spans covering parts of line that any rule matches.
// Returned spans are non-overlapping and sorted by Start.
func (h *Highlighter) Tokenize(line string) []Span {
	var spans []Span
	covered := make([]bool, len(line))

	mark := func(s, e int, a uint16) {
		if s < 0 || e > len(line) || s >= e {
			return
		}
		// Skip already-colored bytes (first rule wins).
		for i := s; i < e; i++ {
			if covered[i] {
				return
			}
		}
		for i := s; i < e; i++ {
			covered[i] = true
		}
		spans = append(spans, Span{Start: s, End: e, Attr: a})
	}

	for _, r := range h.Rules {
		switch {
		case r.Range != nil:
			h.applyRange(line, r, mark)
		case r.Pattern != nil:
			for _, m := range r.Pattern.FindAllStringIndex(line, -1) {
				mark(m[0], m[1], r.Attr)
			}
		case len(r.Keywords) > 0:
			h.applyKeywords(line, r, mark)
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	return spans
}

// applyRange matches text between markers.
func (h *Highlighter) applyRange(line string, r Rule, mark func(int, int, uint16)) {
	rr := r.Range
	if rr.StartMarker == "" {
		return
	}
	i := 0
	for i < len(line) {
		s := strings.Index(line[i:], rr.StartMarker)
		if s < 0 {
			return
		}
		s += i
		end := len(line)
		if rr.EndMarker != "" {
			j := s + len(rr.StartMarker)
			for j < len(line) {
				e := strings.Index(line[j:], rr.EndMarker)
				if e < 0 {
					end = len(line)
					break
				}
				e += j
				if rr.Escape != "" && e > len(rr.Escape) && line[e-len(rr.Escape):e] == rr.Escape {
					j = e + len(rr.EndMarker)
					continue
				}
				end = e + len(rr.EndMarker)
				break
			}
		}
		mark(s, end, r.Attr)
		i = end
	}
}

// applyKeywords colors whole-word keyword occurrences.
func (h *Highlighter) applyKeywords(line string, r Rule, mark func(int, int, uint16)) {
	for _, kw := range r.Keywords {
		i := 0
		for i < len(line) {
			var idx int
			if r.FoldKeywords {
				idx = strings.Index(strings.ToLower(line[i:]), strings.ToLower(kw))
			} else {
				idx = strings.Index(line[i:], kw)
			}
			if idx < 0 {
				break
			}
			start := i + idx
			end := start + len(kw)
			if isWordBoundary(line, start) && isWordBoundary(line, end) {
				mark(start, end, r.Attr)
			}
			i = end
		}
	}
}

func isWordBoundary(s string, i int) bool {
	if i <= 0 || i >= len(s) {
		return true
	}
	return !isWordChar(s[i-1]) || !isWordChar(s[i])
}

func isWordChar(c byte) bool {
	return c == '_' ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9')
}

// Standard pre-built highlighters --------------------------------------

// GoSyntax returns a Highlighter for Go source.
func GoSyntax() *Highlighter {
	keywordAttr := types.MakeAttr(0x0E, 0x01)
	stringAttr := types.MakeAttr(0x0A, 0x01)
	commentAttr := types.MakeAttr(0x08, 0x01)
	numberAttr := types.MakeAttr(0x0B, 0x01)
	return &Highlighter{
		DefaultAttr: types.MakeAttr(0x07, 0x01),
		Rules: []Rule{
			{Range: &RangeRule{StartMarker: "//"}, Attr: commentAttr},
			{Range: &RangeRule{StartMarker: "/*", EndMarker: "*/"}, Attr: commentAttr},
			{Range: &RangeRule{StartMarker: `"`, EndMarker: `"`, Escape: `\`}, Attr: stringAttr},
			{Range: &RangeRule{StartMarker: "`", EndMarker: "`"}, Attr: stringAttr},
			{Pattern: regexp.MustCompile(`\b\d+(\.\d+)?\b`), Attr: numberAttr},
			{Keywords: []string{
				"break", "case", "chan", "const", "continue", "default", "defer",
				"else", "fallthrough", "for", "func", "go", "goto", "if", "import",
				"interface", "map", "package", "range", "return", "select", "struct",
				"switch", "type", "var",
				"true", "false", "nil", "iota",
			}, Attr: keywordAttr},
		},
	}
}

// MarkdownSyntax returns a Highlighter for Markdown.
func MarkdownSyntax() *Highlighter {
	headingAttr := types.MakeAttr(0x0E, 0x01)
	emphAttr := types.MakeAttr(0x0F, 0x01)
	codeAttr := types.MakeAttr(0x0B, 0x01)
	linkAttr := types.MakeAttr(0x09, 0x01)
	return &Highlighter{
		DefaultAttr: types.MakeAttr(0x07, 0x01),
		Rules: []Rule{
			{Pattern: regexp.MustCompile(`^#{1,6} .*$`), Attr: headingAttr},
			{Range: &RangeRule{StartMarker: "**", EndMarker: "**"}, Attr: emphAttr},
			{Range: &RangeRule{StartMarker: "`", EndMarker: "`"}, Attr: codeAttr},
			{Pattern: regexp.MustCompile(`\[[^\]]+\]\([^)]+\)`), Attr: linkAttr},
		},
	}
}
