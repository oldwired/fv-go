package editor

import "sort"

// Transient decorations: host-supplied highlight overlays distinct
// from the syntax Colorer — LSP document-highlight, run-to-cursor,
// coverage shading. Namespaced so independent producers replace their
// own set without clobbering each other. Spans auto-adjust across
// edits (so overlays don't drift between host refreshes) and a span
// whose text is deleted drops out. Paint priority per cell:
// normal < syntax < decoration < selection.

// Decoration is one highlighted byte range [Start, End) with a full
// attribute override.
type Decoration struct {
	Start, End int
	Attr       uint16
}

// SetDecorations replaces the decoration set for key. Ranges are
// clamped to the buffer and snapped to rune starts; empty or
// degenerate ranges are dropped. Decorations are per-Editor — apply
// to both panes of a split explicitly when wanted.
func (e *Editor) SetDecorations(key string, decs []Decoration) {
	clean := make([]Decoration, 0, len(decs))
	for _, d := range decs {
		d.Start = e.Buf.clampToRuneStart(d.Start)
		d.End = e.Buf.clampToRuneStart(d.End)
		if d.End > d.Start {
			clean = append(clean, d)
		}
	}
	sort.Slice(clean, func(a, b int) bool { return clean[a].Start < clean[b].Start })
	if e.decorations == nil {
		e.decorations = map[string][]Decoration{}
	}
	if len(clean) == 0 {
		delete(e.decorations, key)
	} else {
		e.decorations[key] = clean
	}
	e.decoValid = false
}

// ClearDecorations removes the decoration set for key.
func (e *Editor) ClearDecorations(key string) {
	delete(e.decorations, key)
	e.decoValid = false
}

// Decorations returns the current set for key.
func (e *Editor) Decorations(key string) []Decoration {
	return append([]Decoration(nil), e.decorations[key]...)
}

// mergedDecorations flattens every namespace into one sorted,
// non-overlapping list for the draw walk. Namespaces overlay in
// sorted key order — the lexicographically later key wins where they
// overlap — so the outcome is deterministic regardless of call order.
func (e *Editor) mergedDecorations() []Decoration {
	if e.decoValid {
		return e.decoCache
	}
	keys := make([]string, 0, len(e.decorations))
	for k := range e.decorations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var merged []Decoration
	for _, k := range keys {
		for _, d := range e.decorations[k] {
			merged = overlayDecoration(merged, d)
		}
	}
	e.decoCache = merged
	e.decoValid = true
	return merged
}

// overlayDecoration inserts d into a sorted non-overlapping list,
// trimming away whatever it covers.
func overlayDecoration(list []Decoration, d Decoration) []Decoration {
	out := make([]Decoration, 0, len(list)+2)
	inserted := false
	for _, x := range list {
		switch {
		case x.End <= d.Start:
			out = append(out, x)
		case x.Start >= d.End:
			if !inserted {
				out = append(out, d)
				inserted = true
			}
			out = append(out, x)
		default:
			if x.Start < d.Start {
				out = append(out, Decoration{Start: x.Start, End: d.Start, Attr: x.Attr})
			}
			if !inserted {
				out = append(out, d)
				inserted = true
			}
			if x.End > d.End {
				out = append(out, Decoration{Start: d.End, End: x.End, Attr: x.Attr})
			}
		}
	}
	if !inserted {
		out = append(out, d)
	}
	return out
}

// adjustDecorationsForSplice keeps overlays attached across edits;
// ranges whose text was removed drop out.
func (e *Editor) adjustDecorationsForSplice(sp Splice) {
	if len(e.decorations) == 0 {
		return
	}
	for k, decs := range e.decorations {
		kept := decs[:0]
		for _, d := range decs {
			ns, ok := adjustSpan(span{Start: d.Start, End: d.End}, sp, false)
			if !ok || ns.Start == ns.End {
				continue
			}
			kept = append(kept, Decoration{Start: ns.Start, End: ns.End, Attr: d.Attr})
		}
		if len(kept) == 0 {
			delete(e.decorations, k)
		} else {
			e.decorations[k] = kept
		}
	}
	e.decoValid = false
}
