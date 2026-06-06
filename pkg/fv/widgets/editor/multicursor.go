package editor

import (
	"sort"
	"strings"
	stdutf8 "unicode/utf8"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Multi-cursor support. The primary caret stays in Cursor/SelAnchor
// (so every single-caret code path keeps working); secondary carets
// live in extraCarets, kept sorted by position with overlapping
// selections merged. Edits apply at every caret as one undo group.

// Caret is one insertion point with an optional selection anchor
// (-1 = no selection).
type Caret struct {
	Pos    int
	Anchor int
}

func (c Caret) hasSel() bool { return c.Anchor >= 0 && c.Anchor != c.Pos }

func (c Caret) lo() int {
	if c.Anchor >= 0 && c.Anchor < c.Pos {
		return c.Anchor
	}
	return c.Pos
}

func (c Caret) hi() int {
	if c.Anchor > c.Pos {
		return c.Anchor
	}
	return c.Pos
}

// HasMultipleCarets reports whether secondary carets exist.
func (e *Editor) HasMultipleCarets() bool { return len(e.extraCarets) > 0 }

// Carets returns every caret, primary first, secondaries in position
// order.
func (e *Editor) Carets() []Caret {
	out := make([]Caret, 0, len(e.extraCarets)+1)
	out = append(out, Caret{Pos: e.Cursor, Anchor: e.SelAnchor})
	return append(out, e.extraCarets...)
}

// SetCarets installs a caret set; the first element becomes the
// primary. Positions are clamped/rune-snapped, duplicates and
// overlapping selections merge. An empty slice collapses to a single
// caret at the current position.
func (e *Editor) SetCarets(cs []Caret) {
	if len(cs) == 0 {
		e.CollapseCarets()
		return
	}
	// Clamp the primary here — normalizeCarets early-returns when no
	// secondaries exist, and an unclamped Cursor would panic the next
	// splice or column computation.
	e.Cursor = e.Buf.clampToRuneStart(cs[0].Pos)
	e.SelAnchor = cs[0].Anchor
	if e.SelAnchor >= 0 {
		e.SelAnchor = e.Buf.clampToRuneStart(e.SelAnchor)
		if e.SelAnchor == e.Cursor {
			e.SelAnchor = -1
		}
	}
	e.extraCarets = append(e.extraCarets[:0:0], cs[1:]...)
	e.normalizeCarets()
	e.adjustScroll()
}

// AddCaret adds a secondary caret at pos (no selection). A caret
// already there is left alone.
func (e *Editor) AddCaret(pos int) {
	pos = e.Buf.clampToRuneStart(pos)
	e.extraCarets = append(e.extraCarets, Caret{Pos: pos, Anchor: -1})
	e.normalizeCarets()
}

// CollapseCarets drops all secondary carets, keeping the primary.
func (e *Editor) CollapseCarets() { e.extraCarets = nil }

// toggleCaretAt adds a caret at pos — or removes the one already
// sitting there (Alt+click semantics). A click anywhere on a caret's
// selection (including its boundary cells) counts as a hit: without
// the inclusive-boundary check, Alt+click at a selection's end would
// add a coexisting zero-width caret that doubles every edit. The
// primary cannot be removed.
func (e *Editor) toggleCaretAt(pos int) {
	pos = e.Buf.clampToRuneStart(pos)
	for i, c := range e.extraCarets {
		if (!c.hasSel() && c.Pos == pos) ||
			(c.hasSel() && pos >= c.lo() && pos <= c.hi()) {
			e.extraCarets = append(e.extraCarets[:i], e.extraCarets[i+1:]...)
			return
		}
	}
	primary := Caret{Pos: e.Cursor, Anchor: e.SelAnchor}
	if pos == e.Cursor || (primary.hasSel() && pos >= primary.lo() && pos <= primary.hi()) {
		return
	}
	e.AddCaret(pos)
}

// normalizeCarets restores the invariants after any caret mutation:
// all positions rune-snapped and clamped, secondaries sorted by
// position, carets at the same point deduped, overlapping selections
// merged (union). The primary survives a merge as the primary.
func (e *Editor) normalizeCarets() {
	if len(e.extraCarets) == 0 {
		return
	}
	all := e.Carets()
	for i := range all {
		all[i].Pos = e.Buf.clampToRuneStart(all[i].Pos)
		if all[i].Anchor >= 0 {
			all[i].Anchor = e.Buf.clampToRuneStart(all[i].Anchor)
			if all[i].Anchor == all[i].Pos {
				all[i].Anchor = -1
			}
		}
	}
	order := make([]int, len(all))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return all[order[a]].lo() < all[order[b]].lo()
	})

	merged := make([]Caret, 0, len(all))
	primOut := -1
	for _, oi := range order {
		c := all[oi]
		if len(merged) > 0 {
			last := &merged[len(merged)-1]
			// Strict overlap merges; touching carets merge when at
			// least one is zero-width (a bare caret riding a
			// selection's boundary would otherwise double every
			// edit). Two selections that merely touch stay distinct.
			overlaps := c.lo() < last.hi() ||
				(c.lo() == last.hi() && (!c.hasSel() || !last.hasSel()))
			if overlaps {
				if c.hi() > last.hi() {
					if last.hasSel() || c.hasSel() {
						*last = Caret{Pos: c.hi(), Anchor: last.lo()}
					} else {
						*last = Caret{Pos: c.hi(), Anchor: -1}
					}
				}
				if oi == 0 {
					primOut = len(merged) - 1
				}
				continue
			}
		}
		if oi == 0 {
			primOut = len(merged)
		}
		merged = append(merged, c)
	}
	p := merged[primOut]
	e.Cursor, e.SelAnchor = p.Pos, p.Anchor
	e.extraCarets = append(merged[:primOut:primOut], merged[primOut+1:]...)
}

// editAtCarets applies fn at every caret in ascending position order
// as one undo group. fn receives the caret with all earlier splices
// already accounted for and returns the splice to perform (apply=false
// skips that caret, e.g. Backspace at offset 0). Each edited caret
// collapses to caretAfter.
func (e *Editor) editAtCarets(fn func(c Caret) (start, oldEnd int, repl []byte, caretAfter int, apply bool)) {
	carets := e.Carets()
	primary := 0
	order := make([]int, len(carets))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return carets[order[a]].lo() < carets[order[b]].lo()
	})
	e.Buf.BeginGroup()
	delta := 0
	for _, oi := range order {
		c := carets[oi]
		c.Pos += delta
		if c.Anchor >= 0 {
			c.Anchor += delta
		}
		start, oldEnd, repl, after, apply := fn(c)
		if !apply {
			carets[oi] = c
			continue
		}
		e.Buf.splice(e, start, oldEnd, repl, c.Pos, after)
		carets[oi] = Caret{Pos: after, Anchor: -1}
		delta += len(repl) - (oldEnd - start)
	}
	// Write the final caret state back BEFORE EndGroup: the group's
	// single OnChange fires from EndGroup and its handlers must see
	// the post-edit carets.
	e.Cursor, e.SelAnchor = carets[primary].Pos, carets[primary].Anchor
	e.extraCarets = append(carets[:0:0], carets[1:]...)
	e.normalizeCarets()
	e.Buf.EndGroup()
	e.adjustScroll()
}

// moveCarets applies mover to every caret independently, with the
// usual Shift-extends-selection semantics per caret. A step that
// lands on a fold-hidden line (Left/Right crossing the newline at a
// collapsed header's edge) snaps to the nearest visible position in
// the travel direction — the movers themselves stay fold-blind
// because deleteForwardAtCarets uses moverRight as a delete extent
// and must never skip a folded region.
func (e *Editor) moveCarets(preserveSel bool, mover func(pos int) int) {
	apply := func(c Caret) Caret {
		np := e.Buf.clampToRuneStart(mover(c.Pos))
		if len(e.folds) > 0 {
			if line := e.lineNumber(np); !e.IsLineVisible(line) {
				if np > c.Pos {
					ls, _ := e.lineByIndex(e.nextVisibleLine(line + 1))
					np = ls
				} else if np < c.Pos {
					_, le := e.lineByIndex(e.prevVisibleLine(line - 1))
					np = le
				}
			}
		}
		if !preserveSel {
			c.Anchor = -1
		} else if c.Anchor < 0 {
			c.Anchor = c.Pos
		}
		c.Pos = np
		return c
	}
	p := apply(Caret{Pos: e.Cursor, Anchor: e.SelAnchor})
	e.Cursor, e.SelAnchor = p.Pos, p.Anchor
	for i := range e.extraCarets {
		e.extraCarets[i] = apply(e.extraCarets[i])
	}
	e.normalizeCarets()
	e.checkSnippetBounds()
	e.adjustScroll()
}

// moverLeft / moverRight / moverVertical are the per-caret position
// steppers shared by the navigation keys.
func (e *Editor) moverLeft(pos int) int {
	if pos == 0 {
		return 0
	}
	_, sz := stdutf8.DecodeLastRune(e.Buf.data[:pos])
	return pos - sz
}

func (e *Editor) moverRight(pos int) int {
	if pos >= e.Buf.Len() {
		return pos
	}
	_, sz := stdutf8.DecodeRune(e.Buf.data[pos:])
	return pos + sz
}

func (e *Editor) moverVertical(dir int) func(pos int) int {
	return func(pos int) int {
		line := e.lineNumber(pos)
		var target int
		if dir < 0 {
			if line == 0 {
				return pos
			}
			target = e.prevVisibleLine(line - 1)
		} else {
			target = e.nextVisibleLine(line + 1)
			if target >= e.LineCount() {
				return pos
			}
		}
		col := e.columnAt(pos)
		ls, le := e.lineByIndex(target)
		return e.posAtCol(ls, le, col)
	}
}

// addCaretVertically adds a caret on the visible line above (dir=-1)
// or below (dir=+1) the current caret stack, at the primary's display
// column. Fold-hidden lines are skipped — a caret must never land
// where Draw can't show it.
func (e *Editor) addCaretVertically(dir int) {
	extreme := e.lineNumber(e.Cursor)
	for _, c := range e.extraCarets {
		l := e.lineNumber(c.Pos)
		if (dir < 0 && l < extreme) || (dir > 0 && l > extreme) {
			extreme = l
		}
	}
	var target int
	if dir < 0 {
		if extreme == 0 {
			return
		}
		target = e.prevVisibleLine(extreme - 1)
	} else {
		target = e.nextVisibleLine(extreme + 1)
		if target >= e.LineCount() {
			return
		}
	}
	col := e.columnAt(e.Cursor)
	ls, le := e.lineByIndex(target)
	e.AddCaret(e.posAtCol(ls, le, col))
}

// ColumnSelect installs a rectangular (block) selection: one caret per
// VISIBLE line in [fromLine, toLine], each selecting the display-column
// range [fromCol, toCol). Fold-hidden lines are skipped — a block
// selection must cover what the user sees, never invisible text.
// Lines shorter than fromCol get an empty caret at their end. The
// caret on toLine (snapped to a visible line) becomes the primary.
// Display columns account for tab expansion and wide glyphs. Lines
// are clamped to the buffer.
func (e *Editor) ColumnSelect(fromLine, toLine, fromCol, toCol int) {
	clampLine := func(l int) int {
		if l < 0 {
			return 0
		}
		if l >= e.LineCount() {
			return e.LineCount() - 1
		}
		return l
	}
	fromLine, toLine = clampLine(fromLine), clampLine(toLine)
	if !e.IsLineVisible(toLine) {
		toLine = e.prevVisibleLine(toLine)
	}
	loCol, hiCol := fromCol, toCol
	if loCol > hiCol {
		loCol, hiCol = hiCol, loCol
	}
	step := 1
	if toLine < fromLine {
		step = -1
	}
	var cs []Caret
	for ln := toLine; ; ln -= step {
		if e.IsLineVisible(ln) {
			ls, le := e.lineByIndex(ln)
			a := e.posAtCol(ls, le, loCol)
			p := e.posAtCol(ls, le, hiCol)
			if a == p {
				cs = append(cs, Caret{Pos: p, Anchor: -1})
			} else {
				cs = append(cs, Caret{Pos: p, Anchor: a})
			}
		}
		if ln == fromLine || (step > 0 && ln < fromLine) || (step < 0 && ln > fromLine) {
			break
		}
	}
	e.SetCarets(cs)
}

// selectionRanges returns the selected byte ranges of every caret,
// sorted and non-overlapping (normalizeCarets guarantees the latter).
func (e *Editor) selectionRanges() []span {
	var out []span
	for _, c := range e.Carets() {
		if c.hasSel() {
			out = append(out, span{Start: c.lo(), End: c.hi()})
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Start < out[b].Start })
	return out
}

// extraCaretPositions returns the secondary caret offsets, sorted.
func (e *Editor) extraCaretPositions() []int {
	if len(e.extraCarets) == 0 {
		return nil
	}
	out := make([]int, len(e.extraCarets))
	for i, c := range e.extraCarets {
		out[i] = c.Pos
	}
	sort.Ints(out)
	return out
}

func (e *Editor) insertAtCarets(s string) {
	e.editAtCarets(func(c Caret) (int, int, []byte, int, bool) {
		start, end := c.Pos, c.Pos
		if c.hasSel() {
			start, end = c.lo(), c.hi()
		}
		return start, end, []byte(s), start + len(s), true
	})
}

func (e *Editor) backspaceAtCarets() {
	e.editAtCarets(func(c Caret) (int, int, []byte, int, bool) {
		if c.hasSel() {
			return c.lo(), c.hi(), nil, c.lo(), true
		}
		if c.Pos == 0 {
			return 0, 0, nil, 0, false
		}
		prev := e.Buf.clampToRuneStart(c.Pos - 1)
		return prev, c.Pos, nil, prev, true
	})
}

func (e *Editor) deleteForwardAtCarets() {
	e.editAtCarets(func(c Caret) (int, int, []byte, int, bool) {
		if c.hasSel() {
			return c.lo(), c.hi(), nil, c.lo(), true
		}
		if c.Pos >= e.Buf.Len() {
			return 0, 0, nil, 0, false
		}
		return c.Pos, e.moverRight(c.Pos), nil, c.Pos, true
	})
}

// multiSelectionText joins every caret's selected text in document
// order with newlines (the conventional multi-cursor clipboard shape).
func (e *Editor) multiSelectionText() (string, bool) {
	ranges := e.selectionRanges()
	if len(ranges) == 0 {
		return "", false
	}
	parts := make([]string, len(ranges))
	for i, r := range ranges {
		parts[i] = string(e.Buf.data[r.Start:r.End])
	}
	return strings.Join(parts, "\n"), true
}

// deleteSelections removes every caret's selection as one undo group.
func (e *Editor) deleteSelections() {
	e.editAtCarets(func(c Caret) (int, int, []byte, int, bool) {
		if !c.hasSel() {
			return 0, 0, nil, 0, false
		}
		return c.lo(), c.hi(), nil, c.lo(), true
	})
}

// posAtLocal maps an editor-local cell to a byte offset, fold-aware:
// the cell's row is resolved through the visual-row mapping.
func (e *Editor) posAtLocal(local geom.Point) int {
	line := e.LineAtRow(local.Y)
	if line < 0 {
		return e.Buf.Len()
	}
	col := e.LeftCol + local.X
	if col < 0 {
		col = 0
	}
	ls, le := e.lineByIndex(line)
	return e.posAtCol(ls, le, col)
}

// localLineCol maps an editor-local cell to (buffer line, display
// column), clamped into the buffer.
func (e *Editor) localLineCol(local geom.Point) (int, int) {
	line := e.LineAtRow(local.Y)
	if line < 0 {
		line = e.prevVisibleLine(e.LineCount() - 1)
	}
	col := e.LeftCol + local.X
	if col < 0 {
		col = 0
	}
	return line, col
}

// mouseDown handles click, drag-select, Alt+click (toggle caret), and
// Alt+drag (column select). Like Splitter and window dragging, it
// captures the mouse with a synchronous loop pulling the event queue
// directly until EvMouseUp — events consumed here never re-enter
// positional routing. Without an event queue (headless tests, no
// program loop) it degrades to plain click semantics.
func (e *Editor) mouseDown(ev *drivers.Event) {
	local := e.MakeLocal(ev.Where)
	alt := ev.KeyShift&consts.KbAltShift != 0
	if e.Owner != nil {
		e.Owner.Focus(e.Self())
	}
	startLine, startCol := e.localLineCol(local)
	if !alt {
		e.CollapseCarets()
		e.MoveCursor(e.posAtLocal(local), false)
	}
	e.Draw()

	q := views.GetEventQueue()
	if q == nil {
		if alt {
			e.toggleCaretAt(e.posAtLocal(local))
			e.Draw()
		}
		e.ClearEvent(ev)
		return
	}
	pump, wait := views.GetPump(), views.GetWait()
	dragged := false
	for {
		if pump != nil {
			pump()
		}
		next, ok := q.Get()
		if !ok {
			if wait != nil {
				wait()
			}
			continue
		}
		switch next.What {
		case consts.EvMouseUp:
			// An Alt+click that never moved toggles a caret; an
			// Alt+drag already built its column selection.
			if alt && !dragged {
				e.toggleCaretAt(e.posAtLocal(local))
				e.Draw()
			}
			e.ClearEvent(ev)
			return
		case consts.EvMouseMove, consts.EvMouseDown:
			nl := e.MakeLocal(next.Where)
			// Auto-scroll one line per event when dragging past the
			// vertical edges, then clamp the row back into view.
			if nl.Y < 0 {
				e.Scroll(-1)
				nl.Y = 0
			} else if nl.Y >= e.Size.Y {
				e.Scroll(1)
				nl.Y = e.Size.Y - 1
			}
			if nl == local && !dragged {
				continue
			}
			dragged = true
			if alt {
				curLine, curCol := e.localLineCol(nl)
				e.ColumnSelect(startLine, curLine, startCol, curCol)
			} else {
				e.MoveCursor(e.posAtLocal(nl), true)
			}
			e.Draw()
		}
	}
}
