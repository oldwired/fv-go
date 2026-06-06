package editor

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

// LSP snippet support (the subset gopls completions actually use):
// tab stops $1…$n / ${n} / ${n:default}, the terminal stop $0, and
// variables $VAR / ${VAR} / ${VAR:default} resolved through the
// host's SnippetVars callback (else their default, else empty).
// Repeated stops are live mirrors — selecting the stop puts a caret
// on every instance, so typing edits all of them via the multi-cursor
// machinery. Not supported (treated as literal text or a parse
// error): nested placeholders, choices ${1|a,b|}, and transforms.

type parsedStop struct {
	index   int
	offsets []int // instance offsets within the expanded text
	length  int   // seed text length (same for every instance)
}

type sessionStop struct {
	index int
	spans []span
}

// snippetSession tracks an active snippet: its stop spans (kept
// attached to the text through the span machinery) and the overall
// bounds. The session ends on $0, Esc, CancelSnippet, the caret
// leaving bounds, or any splice that isn't this editor's own typing
// inside bounds.
type snippetSession struct {
	stops  []sessionStop // navigation order: 1..n, then 0
	cur    int
	bounds span
}

type snipSegment struct {
	text string
	stop int // -1 = literal text
	def  string
}

func snipIsDigit(c byte) bool { return c >= '0' && c <= '9' }

func snipIsIdentStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func snipIsIdentChar(c byte) bool { return snipIsIdentStart(c) || snipIsDigit(c) }

// snipUnescape resolves \$ \\ \} inside default text; any other
// backslash stays literal.
func snipUnescape(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '$', '\\', '}':
				b.WriteByte(s[i+1])
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// scanBraced returns the raw content between body[from] and the first
// unescaped '}', plus the index just past it.
func snipScanBraced(body string, from int) (string, int, error) {
	for i := from; i < len(body); i++ {
		switch body[i] {
		case '\\':
			i++
		case '}':
			return body[from:i], i + 1, nil
		}
	}
	return "", 0, errors.New("snippet: unterminated ${…}")
}

// parseSnippet expands body: variables resolved, escapes applied,
// every stop instance seeded with the stop's default text (the first
// instance carrying a default seeds all mirrors, per LSP).
func parseSnippet(body string, resolve func(string) (string, bool)) (string, []parsedStop, error) {
	resolveVar := func(name, def string) string {
		if resolve != nil {
			if v, ok := resolve(name); ok {
				return v
			}
		}
		return def
	}

	var segs []snipSegment
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			segs = append(segs, snipSegment{text: text.String(), stop: -1})
			text.Reset()
		}
	}
	i := 0
	for i < len(body) {
		c := body[i]
		if c == '\\' && i+1 < len(body) {
			switch body[i+1] {
			case '$', '\\', '}':
				text.WriteByte(body[i+1])
				i += 2
				continue
			}
			text.WriteByte(c)
			i++
			continue
		}
		if c != '$' {
			text.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(body) {
			text.WriteByte('$')
			i++
			continue
		}
		j := i + 1
		switch {
		case snipIsDigit(body[j]):
			k := j
			for k < len(body) && snipIsDigit(body[k]) {
				k++
			}
			n, _ := strconv.Atoi(body[j:k])
			flush()
			segs = append(segs, snipSegment{stop: n})
			i = k
		case snipIsIdentStart(body[j]):
			k := j
			for k < len(body) && snipIsIdentChar(body[k]) {
				k++
			}
			text.WriteString(resolveVar(body[j:k], ""))
			i = k
		case body[j] == '{':
			content, next, err := snipScanBraced(body, j+1)
			if err != nil {
				return "", nil, err
			}
			name, def, hasDef := strings.Cut(content, ":")
			if hasDef {
				def = snipUnescape(def)
			}
			switch {
			case name != "" && strings.IndexFunc(name, func(r rune) bool {
				return r < '0' || r > '9'
			}) < 0:
				n, _ := strconv.Atoi(name)
				flush()
				segs = append(segs, snipSegment{stop: n, def: def})
			case name != "" && snipIsIdentStart(name[0]):
				for k := 1; k < len(name); k++ {
					if !snipIsIdentChar(name[k]) {
						return "", nil, errors.New("snippet: bad placeholder name ${" + content + "}")
					}
				}
				text.WriteString(resolveVar(name, def))
			default:
				return "", nil, errors.New("snippet: bad placeholder ${" + content + "}")
			}
			i = next
		default:
			text.WriteByte('$')
			i++
		}
	}
	flush()

	// Seed every mirror with the first instance that carries a default.
	seeds := map[int]string{}
	for _, sg := range segs {
		if sg.stop >= 0 && sg.def != "" {
			if _, ok := seeds[sg.stop]; !ok {
				seeds[sg.stop] = sg.def
			}
		}
	}
	var expanded strings.Builder
	offsets := map[int][]int{}
	for _, sg := range segs {
		if sg.stop < 0 {
			expanded.WriteString(sg.text)
			continue
		}
		offsets[sg.stop] = append(offsets[sg.stop], expanded.Len())
		expanded.WriteString(seeds[sg.stop])
	}

	indices := make([]int, 0, len(offsets))
	for n := range offsets {
		indices = append(indices, n)
	}
	sort.Ints(indices)
	// $0 navigates last.
	if len(indices) > 0 && indices[0] == 0 {
		indices = append(indices[1:], 0)
	}
	stops := make([]parsedStop, 0, len(indices))
	for _, n := range indices {
		stops = append(stops, parsedStop{index: n, offsets: offsets[n], length: len(seeds[n])})
	}
	return expanded.String(), stops, nil
}

// SnippetActive reports whether a snippet session is in progress
// (Tab/Shift-Tab navigate stops instead of inserting/changing focus).
func (e *Editor) SnippetActive() bool { return e.snippet != nil }

// CancelSnippet ends the session, leaving the inserted text in place.
func (e *Editor) CancelSnippet() {
	if e.snippet == nil {
		return
	}
	e.snippet = nil
	e.CollapseCarets()
}

// InsertSnippet expands an LSP snippet at the caret (replacing any
// selection) as one undo entry, then starts a tab-stop session: the
// first stop's text is selected (mirrors get one caret each), Tab /
// Shift-Tab navigate, $0 — or Tab past the last stop — ends the
// session. A snippet without stops is a plain insertion. Respects
// ReadOnly (no-op). Malformed snippet syntax returns an error and
// inserts nothing.
func (e *Editor) InsertSnippet(body string) error {
	if e.ReadOnly {
		return nil
	}
	expanded, stops, err := parseSnippet(body, e.SnippetVars)
	if err != nil {
		return err
	}
	e.snippet = nil
	e.CollapseCarets()
	start, end := e.Cursor, e.Cursor
	if e.HasSelection() {
		start, end = e.selRange()
	}
	e.applyChange(start, end, []byte(expanded), start+len(expanded))
	e.SelAnchor = -1
	if len(stops) == 0 {
		e.adjustScroll()
		return nil
	}
	sess := &snippetSession{bounds: span{Start: start, End: start + len(expanded)}}
	for _, ps := range stops {
		st := sessionStop{index: ps.index}
		for _, off := range ps.offsets {
			st.spans = append(st.spans, span{Start: start + off, End: start + off + ps.length})
		}
		sess.stops = append(sess.stops, st)
	}
	e.snippet = sess
	e.snippetSelect(0)
	return nil
}

// snippetSelect activates stop i: primary selection over the first
// instance, one caret per mirror. Selecting $0 places the caret and
// ends the session.
func (e *Editor) snippetSelect(i int) {
	s := e.snippet
	s.cur = i
	st := s.stops[i]
	cs := make([]Caret, 0, len(st.spans))
	for _, sp := range st.spans {
		if sp.Start == sp.End {
			cs = append(cs, Caret{Pos: sp.Start, Anchor: -1})
		} else {
			cs = append(cs, Caret{Pos: sp.End, Anchor: sp.Start})
		}
	}
	e.SetCarets(cs)
	if st.index == 0 {
		e.snippet = nil
		e.CollapseCarets()
	}
}

func (e *Editor) snippetNext() {
	s := e.snippet
	if s.cur+1 < len(s.stops) {
		e.snippetSelect(s.cur + 1)
		return
	}
	// Implicit $0: caret to the end of the snippet, session over.
	end := s.bounds.End
	e.snippet = nil
	e.CollapseCarets()
	e.MoveCursor(end, false)
}

func (e *Editor) snippetPrev() {
	s := e.snippet
	if s.cur > 0 {
		e.snippetSelect(s.cur - 1)
	}
}

// checkSnippetBounds ends the session when the primary caret has
// navigated outside the snippet (click elsewhere, Ctrl+End, …).
func (e *Editor) checkSnippetBounds() {
	s := e.snippet
	if s == nil {
		return
	}
	if e.Cursor < s.bounds.Start || e.Cursor > s.bounds.End {
		e.snippet = nil
		e.CollapseCarets()
	}
}

// adjustSnippetForSplice keeps the session's spans attached across
// this editor's own typing inside the snippet; anything else — a
// foreign edit, an own edit outside bounds, an undo that removes the
// insertion — ends the session. Ending here also drops the mirror
// carets: leaving them live after SnippetActive() reports false would
// silently rewrite the (formerly) mirrored text on the next
// keystroke.
func (e *Editor) adjustSnippetForSplice(sp Splice) {
	s := e.snippet
	if s == nil {
		return
	}
	inside := sp.Start >= s.bounds.Start && sp.Start+sp.OldLen <= s.bounds.End
	if !inside || sp.Origin != e {
		e.snippet = nil
		e.CollapseCarets()
		return
	}
	nb, ok := adjustSpan(s.bounds, sp, true)
	if !ok || nb.Start == nb.End {
		e.snippet = nil
		e.CollapseCarets()
		return
	}
	s.bounds = nb
	for i := range s.stops {
		// Only the actively-edited stop grows when typing lands on
		// its edges; the others keep edge insertions outside. A
		// zero-width $0 sitting right after the current stop would
		// otherwise absorb every typed character, and Tab would then
		// select (and the next keystroke overwrite) text the user
		// just typed.
		grow := i == s.cur
		st := &s.stops[i]
		for j := range st.spans {
			ns, _ := adjustSpan(st.spans[j], sp, grow)
			st.spans[j] = ns
		}
	}
}
