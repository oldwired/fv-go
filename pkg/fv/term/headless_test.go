package term

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/types"
)

func TestHeadlessSnapshotRoundtrip(t *testing.T) {
	h := NewHeadless(10, 3)
	h.Clear(types.MakeAttr(7, 0))
	h.SetCell(0, 0, types.DrawCell{Ch: "H", Attr: types.MakeAttr(7, 0)})
	h.SetCell(1, 0, types.DrawCell{Ch: "i", Attr: types.MakeAttr(7, 0)})
	h.SetCell(0, 1, types.DrawCell{Ch: "x", Attr: types.MakeAttr(7, 0)})
	got := h.Snapshot()
	// 3 rows: "Hi", "x", "" — rows are joined with '\n' separators,
	// no trailing newline at EOF.
	want := "Hi\nx\n"
	if got != want {
		t.Fatalf("snapshot mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestHeadlessEventsChannel(t *testing.T) {
	h := NewHeadless(10, 3)
	h.PushEvent(Event{Kind: EventKey, Rune: 'a'})
	select {
	case ev := <-h.Events():
		if ev.Rune != 'a' {
			t.Fatalf("got rune %q, want 'a'", ev.Rune)
		}
	default:
		t.Fatal("event channel empty")
	}
}

func TestHeadlessWriteRawCapture(t *testing.T) {
	h := NewHeadless(10, 3)
	_ = h.WriteRaw("\x1b]52;c;abc\x07")
	if got := h.Writes(); got != "\x1b]52;c;abc\x07" {
		t.Fatalf("WriteRaw not captured, got %q", got)
	}
	h.Reset()
	if got := h.Writes(); got != "" {
		t.Fatalf("Reset didn't clear, got %q", got)
	}
}

func TestHeadlessCursor(t *testing.T) {
	h := NewHeadless(10, 3)
	h.SetCursor(3, 2)
	h.ShowCursor(true)
	x, y, vis := h.Cursor()
	if x != 3 || y != 2 || !vis {
		t.Fatalf("cursor state mismatch: x=%d y=%d vis=%v", x, y, vis)
	}
}
