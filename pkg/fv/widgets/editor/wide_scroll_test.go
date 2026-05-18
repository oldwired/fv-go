package editor

import (
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// TestWideGlyphScrollClearsCleanly regression for the corruption the
// user saw when scrolling text with wide chars (CJK / emoji): without
// continuation cells in the cellbuf, the diff between a row with
// wide chars and a row of spaces only updated the leading cell of
// each wide glyph. The trailing half stayed on screen as residue.
//
// Setup: render a row that contains 日本語, flush it, then scroll so
// that row becomes empty, flush again. The resulting snapshot must
// not contain any of those wide chars.
func TestWideGlyphScrollClearsCleanly(t *testing.T) {
	h := term.NewHeadless(40, 5)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	e := New(geom.NewRect(0, 0, 40, 5), nil, nil)
	e.State |= consts.SfVisible | consts.SfExposed
	// A few lines of buffer; the wide chars sit on the FIRST line so
	// scrolling down will move them off the top.
	e.SetText(strings.Repeat("ascii padding line\n", 6) + "日本語\n")
	// Place wide chars on line 0 by re-ordering.
	e.SetText("日本語\n" + strings.Repeat("plain ascii line\n", 6))

	e.Draw()
	if err := h.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Sanity: the snapshot should contain the wide chars now.
	snap := h.Snapshot()
	if !strings.Contains(snap, "日") {
		t.Fatalf("pre-scroll snapshot missing 日:\n%s", snap)
	}

	// Scroll down so the wide-char line moves off-screen.
	e.Scroll(3)
	e.Draw()
	if err := h.Flush(); err != nil {
		t.Fatalf("flush 2: %v", err)
	}

	snap = h.Snapshot()
	for _, r := range []string{"日", "本", "語"} {
		if strings.Contains(snap, r) {
			t.Errorf("post-scroll snapshot still contains %q (wide-glyph residue):\n%s", r, snap)
		}
	}
}
