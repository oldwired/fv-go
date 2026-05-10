package term

import (
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/profile"
	"github.com/oldwired/fv-go/pkg/fv/types"
)

func TestSGRCGABlue(t *testing.T) {
	// CGA index 1 should map to ANSI blue (BG 44), not red (41).
	enc := newSGREncoder(profile.Legacy)
	var sb strings.Builder
	enc.transition(&sb, sgrState{fg: 7, bg: 1})
	got := sb.String()
	if !strings.Contains(got, "44") {
		t.Errorf("CGA bg=1 should map to SGR 44 (blue); got %q", got)
	}
	if strings.Contains(got, ";41") || strings.HasSuffix(got, "41m") {
		t.Errorf("CGA bg=1 should NOT emit SGR 41 (red); got %q", got)
	}
}

func TestSGRCGAFGRedBlue(t *testing.T) {
	// CGA fg=4 is red → ANSI 31. fg=1 is blue → ANSI 34.
	enc := newSGREncoder(profile.Legacy)
	var sb strings.Builder
	enc.transition(&sb, sgrState{fg: 4, bg: 0})
	got := sb.String()
	if !strings.Contains(got, "31") {
		t.Errorf("CGA fg=4 should emit SGR 31 (red); got %q", got)
	}
}

func TestCellBufDiffMinimal(t *testing.T) {
	b := newCellBuf(4, 1)
	// First flush should mark all 4 cells dirty (prev is zero-valued).
	first := b.dirty()
	if len(first) != 1 {
		t.Fatalf("first frame should have one span; got %d", len(first))
	}
	b.commit()
	// No changes → empty.
	if next := b.dirty(); len(next) != 0 {
		t.Fatalf("idempotent flush should produce no spans; got %d", len(next))
	}
	// Touch one cell.
	b.Set(2, 0, types.DrawCell{Ch: "x", Attr: types.MakeAttr(15, 0)})
	chunks := b.dirty()
	if len(chunks) != 1 {
		t.Fatalf("one-cell change should be one span; got %d", len(chunks))
	}
	if chunks[0].x != 2 || chunks[0].text != "x" {
		t.Errorf("dirty span: %+v", chunks[0])
	}
}
