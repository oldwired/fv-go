package term

import (
	"bytes"
	"io"
	"testing"
)

// chunkReader yields bytes incrementally across many Read calls.
// Each Read returns at most len(p) bytes from the front of data.
type chunkReader struct{ data []byte }

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.data)
	c.data = c.data[n:]
	return n, nil
}

// TestPasteCapTruncatesAtLimit feeds a paste that opens ESC[200~,
// fills well past maxPasteBytes, and never closes — the reader must
// emit a Truncated paste event rather than allocating unbounded
// memory.
func TestPasteCapTruncatesAtLimit(t *testing.T) {
	// Construct: ESC[200~ + (maxPasteBytes + 1MiB) of 'A' bytes.
	var b bytes.Buffer
	b.WriteString("\x1b[200~")
	b.Write(bytes.Repeat([]byte{'A'}, maxPasteBytes+(1<<20)))

	r := &reader{
		in:  &chunkReader{data: b.Bytes()},
		buf: make([]byte, 1<<16),
	}

	var sawTruncated bool
	for !sawTruncated {
		evs, err := r.Next()
		if err != nil && err != io.EOF {
			t.Fatalf("reader err: %v", err)
		}
		for _, ev := range evs {
			if ev.Kind == EventPaste {
				if ev.Truncated {
					sawTruncated = true
					if len(ev.Paste) > maxPasteBytes {
						t.Errorf("paste payload exceeded cap: got %d bytes", len(ev.Paste))
					}
				}
			}
		}
		if err == io.EOF && !sawTruncated {
			t.Fatal("EOF reached without a Truncated paste event")
		}
	}
}

// TestPasteCapDiscardsResidualThenResumes regression: after the cap
// fires and Truncated=true is emitted, the reader used to set
// r.paste=false but leave residual bytes in r.scan. Those residual
// bytes then re-entered the keystroke parser, allowing a malicious or
// pathological huge paste to inject synthetic key events past the
// truncation boundary. The fix keeps paste-mode armed in
// "capped/discard" state until the actual ESC[201~ close sequence
// arrives, then resumes normal parsing.
//
// To exercise the residual-discard path, the test makes the overflow
// large enough (4 MiB past the cap) that the closing ESC[201~ lands
// many Read chunks AFTER the cap fires — otherwise the close-found
// branch would handle the paste normally before the cap was reached.
func TestPasteCapDiscardsResidualThenResumes(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("\x1b[200~")
	// Overflow well past the cap so the close arrives multiple
	// Read-buffer fills later, guaranteeing the cap fires first.
	b.Write(bytes.Repeat([]byte{'A'}, maxPasteBytes+(4<<20)))
	b.WriteString("\x1b[201~")
	b.WriteByte('x')

	r := &reader{
		in:  &chunkReader{data: b.Bytes()},
		buf: make([]byte, 1<<16),
	}

	var pasteEvents int
	var sawTruncated bool
	var sawX bool
	for {
		evs, err := r.Next()
		for _, ev := range evs {
			if ev.Kind == EventPaste {
				pasteEvents++
				if ev.Truncated {
					sawTruncated = true
				}
			}
			if ev.Kind == EventKey && ev.Rune == 'x' {
				sawX = true
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reader err: %v", err)
		}
	}
	if !sawTruncated {
		t.Fatal("missing Truncated paste event")
	}
	if pasteEvents != 1 {
		t.Errorf("expected exactly one EventPaste (the truncated one); got %d", pasteEvents)
	}
	if !sawX {
		t.Error("trailing 'x' keystroke did not arrive — reader stuck in paste-discard mode past ESC[201~")
	}
}

// TestPasteCapAllowsNormalPaste regression: short pastes must still
// fire a normal EventPaste with Truncated=false.
func TestPasteCapAllowsNormalPaste(t *testing.T) {
	data := []byte("\x1b[200~hello world\x1b[201~")
	r := &reader{in: &chunkReader{data: data}, buf: make([]byte, 4096)}
	evs, _ := r.Next()
	var got *Event
	for i := range evs {
		if evs[i].Kind == EventPaste {
			got = &evs[i]
		}
	}
	if got == nil {
		t.Fatal("no paste event")
	}
	if got.Truncated {
		t.Error("short paste falsely flagged as truncated")
	}
	if got.Paste != "hello world" {
		t.Errorf("paste payload: got %q", got.Paste)
	}
}
