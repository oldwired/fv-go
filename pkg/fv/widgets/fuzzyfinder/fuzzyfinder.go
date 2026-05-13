// Package fuzzyfinder provides FuzzyFinder — an interactive
// "type to filter" picker that scores items by character-subsequence
// match. Returns the chosen item's index from Run, or -1 on cancel.
//
// Ported from FuzzyFinder.pas. The Pascal version uses its own
// scoring function; ours is the standard "consecutive matches score
// higher, anchored matches score higher" heuristic.
package fuzzyfinder

import (
	"sort"
	"strings"
	"unicode"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// FuzzyFinder is a popup with an inline filter input + scrollable
// candidate list.
type FuzzyFinder struct {
	views.Base

	Items []string

	query   string
	current int // selected index in matches
	matches []match
	closed  bool
	chosen  int

	FrameColor uint16
	TextColor  uint16
	HitColor   uint16
	SelColor   uint16
}

type match struct {
	idx   int
	score int
	hits  []int // byte indices in Items[idx] that matched
}

// New constructs a FuzzyFinder with the given items.
func New(bounds geom.Rect, items []string) *FuzzyFinder {
	f := &FuzzyFinder{
		Base:       views.NewBase(bounds),
		Items:      items,
		FrameColor: theme.Get().FuzzyFinderFrame,
		TextColor:  theme.Get().PopupMenuNormal,
		HitColor:   theme.Get().PopupMenuHot,
		SelColor:   theme.Get().FuzzyFinderSelected,
		chosen:     -1,
	}
	f.SetSelf(f)
	f.State |= consts.SfVisible | consts.SfExposed | consts.SfCursorVis
	f.recalc()
	return f
}

// GetTypeID for serial registry.
func (f *FuzzyFinder) GetTypeID() string { return "fuzzyfinder" }

// Run inserts the picker into host and runs a modal-style loop.
// Returns the chosen item's index in the original Items slice, or -1.
func (f *FuzzyFinder) Run(host *views.Group) int {
	host.Insert(f)
	defer host.Delete(f)
	q := views.GetEventQueue()
	if q == nil {
		return -1
	}
	for !f.closed {
		if pump := views.GetPump(); pump != nil {
			pump()
		}
		ev, ok := q.Get()
		if !ok {
			if wait := views.GetWait(); wait != nil {
				wait()
			}
			continue
		}
		f.HandleEvent(&ev)
		views.MarkDirty()
	}
	return f.chosen
}

// HandleEvent processes one input event.
func (f *FuzzyFinder) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvMouseDown {
		local := f.MakeLocal(ev.Where)
		if local.Y >= 2 && local.Y-2 < len(f.matches) &&
			local.X > 0 && local.X < f.Size.X-1 {
			f.current = local.Y - 2
			f.commit()
			f.ClearEvent(ev)
			return
		}
		// click outside → cancel
		f.cancel()
		f.ClearEvent(ev)
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbEsc:
		f.cancel()
	case consts.KbEnter:
		f.commit()
	case consts.KbUp:
		if f.current > 0 {
			f.current--
		}
	case consts.KbDown:
		if f.current+1 < len(f.matches) {
			f.current++
		}
	case consts.KbBack:
		if len(f.query) > 0 {
			f.query = f.query[:len(f.query)-1]
			f.recalc()
		}
	default:
		if ev.UnicodeChar >= ' ' {
			f.query += string(ev.UnicodeChar)
			f.recalc()
		} else {
			return
		}
	}
	f.Draw()
	f.ClearEvent(ev)
}

func (f *FuzzyFinder) cancel() {
	f.chosen = -1
	f.closed = true
}

func (f *FuzzyFinder) commit() {
	if f.current < 0 || f.current >= len(f.matches) {
		f.chosen = -1
	} else {
		f.chosen = f.matches[f.current].idx
	}
	f.closed = true
}

// recalc re-scores items against query.
func (f *FuzzyFinder) recalc() {
	f.matches = f.matches[:0]
	q := strings.ToLower(f.query)
	for i, it := range f.Items {
		score, hits, ok := scoreFuzzy(strings.ToLower(it), q)
		if !ok {
			continue
		}
		f.matches = append(f.matches, match{idx: i, score: score, hits: hits})
	}
	sort.SliceStable(f.matches, func(i, j int) bool {
		return f.matches[i].score > f.matches[j].score
	})
	if f.current >= len(f.matches) {
		f.current = len(f.matches) - 1
	}
	if f.current < 0 {
		f.current = 0
	}
}

// scoreFuzzy returns (score, hit-positions, matched). All chars of
// query must appear in target in order. Anchored / consecutive
// matches score higher.
func scoreFuzzy(target, query string) (int, []int, bool) {
	if query == "" {
		return 0, nil, true
	}
	hits := make([]int, 0, len(query))
	score := 0
	prev := -2
	qi := 0
	for i := 0; i < len(target) && qi < len(query); i++ {
		if target[i] == query[qi] {
			hits = append(hits, i)
			s := 1
			if i == 0 || !unicode.IsLetter(rune(target[i-1])) {
				s += 5 // anchored to a word boundary
			}
			if i == prev+1 {
				s += 5 // consecutive match
			}
			score += s
			prev = i
			qi++
		}
	}
	if qi < len(query) {
		return 0, nil, false
	}
	return score, hits, true
}

// Draw paints the picker.
func (f *FuzzyFinder) Draw() {
	w, h := f.Size.X, f.Size.Y
	// Frame.
	top := screen.MakeDrawBuffer(w)
	bot := screen.MakeDrawBuffer(w)
	screen.DrawCell(top, 0, "┌", f.FrameColor)
	screen.DrawCell(bot, 0, "└", f.FrameColor)
	for i := 1; i < w-1; i++ {
		screen.DrawCell(top, i, "─", f.FrameColor)
		screen.DrawCell(bot, i, "─", f.FrameColor)
	}
	screen.DrawCell(top, w-1, "┐", f.FrameColor)
	screen.DrawCell(bot, w-1, "┘", f.FrameColor)
	f.WriteLine(0, 0, w, 1, top)
	f.WriteLine(0, h-1, w, 1, bot)

	// Query row.
	q := screen.MakeDrawBuffer(w)
	screen.DrawCell(q, 0, "│", f.FrameColor)
	for i := 1; i < w-1; i++ {
		screen.DrawCell(q, i, " ", f.TextColor)
	}
	screen.DrawStr(q, 2, "› "+f.query, f.TextColor)
	screen.DrawCell(q, w-1, "│", f.FrameColor)
	f.WriteLine(0, 1, w, 1, q)

	// Matches.
	for r := 0; r+2 < h-1; r++ {
		row := screen.MakeDrawBuffer(w)
		c := f.TextColor
		if r == f.current {
			c = f.SelColor
		}
		screen.DrawCell(row, 0, "│", f.FrameColor)
		for i := 1; i < w-1; i++ {
			screen.DrawCell(row, i, " ", c)
		}
		screen.DrawCell(row, w-1, "│", f.FrameColor)
		if r < len(f.matches) {
			m := f.matches[r]
			text := f.Items[m.idx]
			screen.DrawStr(row, 2, text, c)
			// Highlight matched chars.
			for _, hi := range m.hits {
				if 2+hi < w-1 {
					ch := string(text[hi])
					row[2+hi] = types.DrawCell{Ch: ch, Attr: f.HitColor}
				}
			}
		}
		f.WriteLine(0, 2+r, w, 1, row)
	}
	f.Cursor = geom.Point{X: 4 + len(f.query), Y: 1}
}
