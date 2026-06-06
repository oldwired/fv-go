package editor

// span is a half-open byte range [Start, End) into a Buffer.
type span struct{ Start, End int }

// bias controls which side a position sticks to when text is inserted
// exactly at it.
type bias uint8

const (
	biasLeft  bias = iota // insertion at the position leaves it in place
	biasRight             // insertion at the position pushes it right
)

// adjustPos maps a byte position across a splice. A position strictly
// inside the replaced range clamps to the range's start (biasLeft) or
// to the end of the replacement (biasRight).
func adjustPos(p int, sp Splice, b bias) int {
	end := sp.Start + sp.OldLen
	switch {
	case p < sp.Start:
		return p
	case p > end:
		return p + sp.NewLen - sp.OldLen
	case p == sp.Start:
		if sp.OldLen == 0 && b == biasRight {
			return p + sp.NewLen
		}
		return sp.Start
	case p == end:
		return sp.Start + sp.NewLen
	default:
		if b == biasRight {
			return sp.Start + sp.NewLen
		}
		return sp.Start
	}
}

// adjustCaret maps a caret (or bookmark) across a splice: an
// insertion exactly at the position pushes it right (so a caret
// parked where another pane types stays after the typed text), while
// a position inside a replaced range clamps to the range start.
func adjustCaret(p int, sp Splice) int {
	if p > sp.Start && p < sp.Start+sp.OldLen {
		return sp.Start
	}
	return adjustPos(p, sp, biasRight)
}

// adjustSpan maps a span across a splice. grow=true keeps insertions
// at either edge inside the span (snippet placeholders: typing at the
// edge of a placeholder extends it); grow=false leaves edge insertions
// outside (decorations, fold anchors).
//
// ok=false reports that the splice destroyed the span: a previously
// non-empty span collapsed to nothing. The one exception is a
// grow-span whose exact range was replaced — a placeholder that was
// select-all-deleted is still a live tab stop, so it survives
// collapsed.
func adjustSpan(s span, sp Splice, grow bool) (span, bool) {
	sb, eb := biasRight, biasLeft
	if grow {
		sb, eb = biasLeft, biasRight
	}
	ns := span{Start: adjustPos(s.Start, sp, sb), End: adjustPos(s.End, sp, eb)}
	if ns.End < ns.Start {
		ns.End = ns.Start
	}
	if ns.Start != ns.End || s.Start == s.End {
		return ns, true
	}
	if grow && sp.Start == s.Start && sp.Start+sp.OldLen == s.End {
		return ns, true
	}
	return ns, false
}
