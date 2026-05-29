package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Case-insensitive Find must map the match offset onto the ORIGINAL
// bytes. bytes.ToLower folds U+212A (KELVIN SIGN, 3 bytes) to ASCII "k"
// (1 byte), changing length and shifting the offset; the ASCII fold is
// length-preserving so the offset stays aligned.
func TestFindCaseInsensitiveUnicodeAlignment(t *testing.T) {
	e := newTestEditor()
	kelvin := "K" // 3 UTF-8 bytes
	e.SetText(kelvin + "abcXYZ")

	if !e.Find("xyz", false) {
		t.Fatal("Find did not match XYZ case-insensitively")
	}
	wantAnchor := len(kelvin) + len("abc") // 6
	if e.SelAnchor != wantAnchor {
		t.Errorf("SelAnchor = %d, want %d (offset must map onto original bytes)", e.SelAnchor, wantAnchor)
	}
	if e.Cursor != wantAnchor+len("XYZ") {
		t.Errorf("Cursor = %d, want %d", e.Cursor, wantAnchor+len("XYZ"))
	}
}

// ReplaceAll shared the same mis-slice bug via strings.ToLower.
func TestReplaceAllCaseInsensitiveUnicodeAlignment(t *testing.T) {
	e := newTestEditor()
	kelvin := "K"
	e.SetText(kelvin + "XYZ" + kelvin)

	n := e.ReplaceAll("xyz", "q", false)
	if n != 1 {
		t.Fatalf("ReplaceAll count = %d, want 1", n)
	}
	want := kelvin + "q" + kelvin
	if string(e.Data) != want {
		t.Errorf("ReplaceAll = %q, want %q (mis-sliced on multibyte rune)", string(e.Data), want)
	}
}

// PgUp must leave the caret on screen. The old manual `Top -= Size.Y`
// (on top of adjustScroll's own page shift) scrolled a second page and
// pushed the caret off the top.
func TestPageUpKeepsCaretVisible(t *testing.T) {
	e := newTestEditor() // 40x10 viewport
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&sb, "line%02d\n", i)
	}
	e.SetText(sb.String())

	// Park the caret deep in the buffer (line 40).
	e.MoveCursor(e.posAtVisible(40, 0), false)

	ev := drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbPgUp}
	e.HandleEvent(&ev)

	cl := e.lineNumber(e.Cursor)
	if cl < e.Top || cl >= e.Top+e.Size.Y {
		t.Errorf("after PgUp caret line %d outside visible window [%d,%d)", cl, e.Top, e.Top+e.Size.Y)
	}
}

// Deep-scroll Draw must render the same lines after the incremental
// line-walk refactor (no per-row rescan from byte 0).
func TestDrawDeepScrollRendersCorrectLines(t *testing.T) {
	h := term.NewHeadless(40, 12)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	e := newTestEditor() // 40x10
	e.State |= consts.SfExposed | consts.SfVisible
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&sb, "L%d\n", i)
	}
	e.SetText(sb.String())
	e.Top = 90

	e.Draw()
	_ = h.Flush()

	rows := strings.Split(h.Snapshot(), "\n")
	if len(rows) < 10 {
		t.Fatalf("snapshot has %d rows, want >= 10", len(rows))
	}
	if rows[0] != "L90" {
		t.Errorf("row 0 = %q, want L90 (incremental line walk drifted)", rows[0])
	}
	if rows[9] != "L99" {
		t.Errorf("row 9 = %q, want L99", rows[9])
	}
}
