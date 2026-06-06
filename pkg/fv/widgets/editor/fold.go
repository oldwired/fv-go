package editor

import (
	"bytes"
	"sort"
)

// Code folding. Fold regions are per-Editor view state (two panes over
// one Buffer fold independently) but are anchored as byte spans so the
// shared span machinery keeps them attached to their text across edits
// from any pane. A collapsed region shows only its header line, with
// the hidden line count rendered as a summary suffix; all coordinate
// math (Draw, scrolling, navigation, mouse, gutter) flows through the
// visual-row mapping below.
//
// Region line extents are CACHED (startLine/endLine fields), shifted
// by LinesDelta on disjoint splices and recomputed only when a splice
// actually intersects a region. Deriving them from the spans on every
// read would put a full-prefix bytes.Count behind every gutter cell
// and Draw row — the same per-row rescan pathology the Draw loop
// itself was already cured of.

// FoldRegion is one foldable line range, 0-based and inclusive. Hosts
// derive these from language structure (gopls folding ranges, brace
// matching, indentation).
type FoldRegion struct {
	StartLine, EndLine int
}

// foldRegion anchors a region as a byte span [start of StartLine, end
// of EndLine including its newline) with its line extent cached. The
// cache is always derived from the span (see recomputeRegionLines) so
// build-time and post-splice values can never disagree — an input
// region ending on the empty last line normalizes to the last content
// line once, not on the first unrelated edit.
type foldRegion struct {
	s         span
	startLine int
	endLine   int
	collapsed bool
}

// lineInterval is an inclusive range of hidden buffer lines.
type lineInterval struct{ lo, hi int }

// recomputeRegionLines derives the cached line extent from the span:
// startLine is the line containing s.Start, endLine the line
// containing s.End-1.
func (e *Editor) recomputeRegionLines(f *foldRegion) {
	f.startLine = e.lineNumber(f.s.Start)
	if f.s.End > f.s.Start {
		f.endLine = f.startLine + bytes.Count(e.Buf.data[f.s.Start:f.s.End-1], nlByte)
	} else {
		f.endLine = f.startLine
	}
}

func (e *Editor) invalidateFolds() { e.hiddenValid = false }

// hiddenIntervals returns the sorted, merged intervals of hidden lines
// (every collapsed region hides startLine+1..endLine). Rebuilt lazily
// from the cached line extents; invalidated by splices and fold
// toggles.
func (e *Editor) hiddenIntervals() []lineInterval {
	if e.hiddenValid {
		return e.hiddenCache
	}
	e.hiddenCache = e.hiddenCache[:0]
	for _, f := range e.folds {
		if !f.collapsed {
			continue
		}
		lo := f.startLine + 1
		hi := f.endLine
		if hi >= lo {
			e.hiddenCache = append(e.hiddenCache, lineInterval{lo, hi})
		}
	}
	sort.Slice(e.hiddenCache, func(a, b int) bool {
		return e.hiddenCache[a].lo < e.hiddenCache[b].lo
	})
	merged := e.hiddenCache[:0]
	for _, iv := range e.hiddenCache {
		if n := len(merged); n > 0 && iv.lo <= merged[n-1].hi+1 {
			if iv.hi > merged[n-1].hi {
				merged[n-1].hi = iv.hi
			}
			continue
		}
		merged = append(merged, iv)
	}
	e.hiddenCache = merged
	e.hiddenValid = true
	return e.hiddenCache
}

// IsLineVisible reports whether line is currently shown (not hidden
// inside a collapsed region). Header lines are always visible.
func (e *Editor) IsLineVisible(line int) bool {
	for _, iv := range e.hiddenIntervals() {
		if line < iv.lo {
			return true
		}
		if line <= iv.hi {
			return false
		}
	}
	return true
}

// nextVisibleLine returns the smallest visible line >= l (may be past
// the last line when everything below is hidden).
func (e *Editor) nextVisibleLine(l int) int {
	for _, iv := range e.hiddenIntervals() {
		if l < iv.lo {
			break
		}
		if l <= iv.hi {
			l = iv.hi + 1
		}
	}
	return l
}

// prevVisibleLine returns the largest visible line <= l. Line 0 is
// always visible (hidden intervals start at a header+1), so the result
// is never negative for l >= 0.
func (e *Editor) prevVisibleLine(l int) int {
	ivs := e.hiddenIntervals()
	for i := len(ivs) - 1; i >= 0; i-- {
		if l > ivs[i].hi {
			break
		}
		if l >= ivs[i].lo {
			l = ivs[i].lo - 1
		}
	}
	return l
}

// hiddenBefore counts hidden lines strictly before line.
func (e *Editor) hiddenBefore(line int) int {
	n := 0
	for _, iv := range e.hiddenIntervals() {
		if iv.lo >= line {
			break
		}
		hi := iv.hi
		if hi >= line {
			hi = line - 1
		}
		n += hi - iv.lo + 1
	}
	return n
}

// visIndex maps a buffer line to its index among visible lines.
func (e *Editor) visIndex(line int) int { return line - e.hiddenBefore(line) }

// lineAtVisIndex maps a visible-line index back to a buffer line.
func (e *Editor) lineAtVisIndex(v int) int {
	l := v
	for _, iv := range e.hiddenIntervals() {
		if iv.lo > l {
			break
		}
		l += iv.hi - iv.lo + 1
	}
	return l
}

// VisibleLineCount returns the number of lines currently shown.
func (e *Editor) VisibleLineCount() int {
	total := e.LineCount()
	hidden := 0
	for _, iv := range e.hiddenIntervals() {
		hi := iv.hi
		if hi > total-1 {
			hi = total - 1
		}
		if hi >= iv.lo {
			hidden += hi - iv.lo + 1
		}
	}
	return total - hidden
}

// RowOfLine returns the editor row line is rendered on, relative to
// Top. Negative when the line is hidden or scrolled above the view;
// compare against Size.Y for "below the view".
func (e *Editor) RowOfLine(line int) int {
	if line < 0 || line >= e.LineCount() || !e.IsLineVisible(line) {
		return -1
	}
	return e.visIndex(line) - e.visIndex(e.Top)
}

// LineAtRow returns the buffer line rendered at editor row (0-based),
// or -1 when the row is past the end of the buffer. The gutter uses
// this for both rendering and click mapping.
func (e *Editor) LineAtRow(row int) int {
	if row < 0 {
		return -1
	}
	line := e.lineAtVisIndex(e.visIndex(e.Top) + row)
	if line >= e.LineCount() {
		return -1
	}
	return line
}

// lineWalkVisible moves delta visible lines from a starting line
// (negative = up), clamping at the buffer edges.
func (e *Editor) lineWalkVisible(from, delta int) int {
	l := from
	lineCount := e.LineCount()
	for ; delta > 0; delta-- {
		n := e.nextVisibleLine(l + 1)
		if n >= lineCount {
			break
		}
		l = n
	}
	for ; delta < 0; delta++ {
		if l == 0 {
			break
		}
		l = e.prevVisibleLine(l - 1)
	}
	return l
}

// SetFoldRegions replaces the fold-region set (the host re-supplies it
// whenever its language analysis refreshes). Collapsed state is
// preserved for regions whose header line matches a currently
// collapsed one, so a gopls refresh doesn't pop every fold open.
// Regions must be multi-line and nest cleanly; overlapping-but-not-
// nested regions are dropped (first wins). Single-line and
// out-of-range regions are ignored. Line→offset anchors are resolved
// in ONE pass over the buffer, not one scan per region.
func (e *Editor) SetFoldRegions(regions []FoldRegion) {
	collapsed := map[int]bool{}
	for _, f := range e.folds {
		if f.collapsed {
			collapsed[f.startLine] = true
		}
	}
	sorted := append([]FoldRegion(nil), regions...)
	sort.Slice(sorted, func(a, b int) bool {
		if sorted[a].StartLine != sorted[b].StartLine {
			return sorted[a].StartLine < sorted[b].StartLine
		}
		return sorted[a].EndLine > sorted[b].EndLine
	})
	lineCount := e.LineCount()
	accepted := sorted[:0]
	var stack []FoldRegion
	for _, r := range sorted {
		if r.StartLine < 0 || r.EndLine <= r.StartLine || r.StartLine >= lineCount {
			continue
		}
		if r.EndLine >= lineCount {
			r.EndLine = lineCount - 1
			if r.EndLine <= r.StartLine {
				continue
			}
		}
		for len(stack) > 0 && stack[len(stack)-1].EndLine < r.StartLine {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 && r.EndLine > stack[len(stack)-1].EndLine {
			continue // overlaps without nesting
		}
		stack = append(stack, r)
		accepted = append(accepted, r)
	}

	// Resolve every needed line→offset boundary in a single walk.
	needed := make([]int, 0, len(accepted)*2)
	for _, r := range accepted {
		needed = append(needed, r.StartLine)
		if r.EndLine+1 < lineCount {
			needed = append(needed, r.EndLine+1)
		}
	}
	sort.Ints(needed)
	offsets := make(map[int]int, len(needed))
	data := e.Buf.data
	line, pos := 0, 0
	for _, want := range needed {
		if _, ok := offsets[want]; ok {
			continue
		}
		for line < want {
			nl := bytes.IndexByte(data[pos:], '\n')
			if nl < 0 {
				pos = len(data)
				line = want
				break
			}
			pos += nl + 1
			line++
		}
		offsets[want] = pos
	}

	e.folds = e.folds[:0]
	for _, r := range accepted {
		start := offsets[r.StartLine]
		end := len(data)
		if r.EndLine+1 < lineCount {
			end = offsets[r.EndLine+1]
		}
		f := foldRegion{
			s:         span{Start: start, End: end},
			collapsed: collapsed[r.StartLine],
		}
		e.recomputeRegionLines(&f)
		if f.endLine <= f.startLine {
			continue
		}
		e.folds = append(e.folds, f)
	}
	e.invalidateFolds()
	e.evictCaretsFromHidden()
	e.adjustScroll()
}

// FoldRegions returns the current region set as line ranges.
func (e *Editor) FoldRegions() []FoldRegion {
	out := make([]FoldRegion, len(e.folds))
	for i, f := range e.folds {
		out[i] = FoldRegion{StartLine: f.startLine, EndLine: f.endLine}
	}
	return out
}

// Fold collapses the region whose header is line. No-op when line
// heads no region.
func (e *Editor) Fold(line int) { e.setFolded(line, true) }

// Unfold expands the region whose header is line.
func (e *Editor) Unfold(line int) { e.setFolded(line, false) }

// ToggleFold flips the region whose header is line.
func (e *Editor) ToggleFold(line int) {
	for i := range e.folds {
		if e.folds[i].startLine == line {
			e.setFolded(line, !e.folds[i].collapsed)
			return
		}
	}
}

func (e *Editor) setFolded(line int, collapsed bool) {
	changed := false
	for i := range e.folds {
		if e.folds[i].startLine == line && e.folds[i].collapsed != collapsed {
			e.folds[i].collapsed = collapsed
			changed = true
		}
	}
	if !changed {
		return
	}
	e.invalidateFolds()
	if collapsed {
		e.evictCaretsFromHidden()
	}
	e.adjustScroll()
}

// FoldAll collapses every region; UnfoldAll expands every region.
func (e *Editor) FoldAll() {
	for i := range e.folds {
		e.folds[i].collapsed = true
	}
	e.invalidateFolds()
	e.evictCaretsFromHidden()
	e.adjustScroll()
}

func (e *Editor) UnfoldAll() {
	for i := range e.folds {
		e.folds[i].collapsed = false
	}
	e.invalidateFolds()
	e.adjustScroll()
}

// IsFolded reports whether line is the header of a collapsed region.
func (e *Editor) IsFolded(line int) bool {
	for _, f := range e.folds {
		if f.collapsed && f.startLine == line {
			return true
		}
	}
	return false
}

// FoldMarkerAt returns the gutter affordance for line: '▸' for a
// collapsed header, '▾' for an expanded header, 0 for plain lines.
func (e *Editor) FoldMarkerAt(line int) rune {
	marker := rune(0)
	for _, f := range e.folds {
		if f.startLine != line {
			continue
		}
		if f.collapsed {
			return '▸'
		}
		marker = '▾'
	}
	return marker
}

// foldedLinesAt returns how many lines the collapsed region headed by
// line hides (0 when not a collapsed header). Nested collapsed
// regions report the outermost extent.
func (e *Editor) foldedLinesAt(line int) int {
	n := 0
	for _, f := range e.folds {
		if f.collapsed && f.startLine == line {
			if l := f.endLine - f.startLine; l > n {
				n = l
			}
		}
	}
	return n
}

// unfoldAt expands every collapsed region hiding line (all nesting
// levels), restoring the caret-never-hidden invariant.
func (e *Editor) unfoldAt(line int) {
	changed := false
	for i := range e.folds {
		f := &e.folds[i]
		if !f.collapsed {
			continue
		}
		if line > f.startLine && line <= f.endLine {
			f.collapsed = false
			changed = true
		}
	}
	if changed {
		e.invalidateFolds()
	}
}

// evictCaretsFromHidden moves any caret sitting on a now-hidden line
// to the end of the header line above it.
func (e *Editor) evictCaretsFromHidden() {
	evict := func(pos int) int {
		line := e.lineNumber(pos)
		if e.IsLineVisible(line) {
			return pos
		}
		header := e.prevVisibleLine(line)
		_, le := e.lineByIndex(header)
		return le
	}
	e.Cursor = evict(e.Cursor)
	if e.SelAnchor >= 0 {
		e.SelAnchor = evict(e.SelAnchor)
		if e.SelAnchor == e.Cursor {
			e.SelAnchor = -1
		}
	}
	for i := range e.extraCarets {
		e.extraCarets[i].Pos = evict(e.extraCarets[i].Pos)
		if e.extraCarets[i].Anchor >= 0 {
			e.extraCarets[i].Anchor = evict(e.extraCarets[i].Anchor)
		}
	}
	e.normalizeCarets()
}

// adjustFoldsForSplice remaps region anchors across an edit and
// applies the safety rules: a region that lost its multi-line shape is
// dropped; a collapsed region whose interior was touched (content
// edited inside the hidden range, or its line structure changed)
// auto-unfolds so the user sees what changed.
func (e *Editor) adjustFoldsForSplice(sp Splice) {
	if len(e.folds) == 0 {
		return
	}
	kept := e.folds[:0]
	for _, f := range e.folds {
		ns, ok := adjustSpan(f.s, sp, false)
		if !ok {
			continue
		}
		nf := f
		nf.s = ns
		// A splice that never touched the span shifts the region's
		// lines by the splice's newline delta (when before it) or
		// not at all (when after it) — no rescan. This is the hot
		// path: it runs for every region on every keystroke.
		switch {
		case sp.Start+sp.OldLen <= f.s.Start:
			nf.startLine += sp.LinesDelta
			nf.endLine += sp.LinesDelta
		case sp.Start >= f.s.End:
			// untouched
		default:
			if bytes.IndexByte(e.Buf.data[ns.Start:ns.End], '\n') < 0 {
				continue // single line now — nothing foldable
			}
			oldLines := f.endLine - f.startLine
			e.recomputeRegionLines(&nf)
			if nf.collapsed {
				headerEnd := e.lineEnd(ns.Start)
				interiorEdit := sp.Start > headerEnd && sp.Start < ns.End
				if nf.endLine-nf.startLine != oldLines || interiorEdit {
					nf.collapsed = false
				}
			}
		}
		kept = append(kept, nf)
	}
	e.folds = kept
	e.invalidateFolds()
}
