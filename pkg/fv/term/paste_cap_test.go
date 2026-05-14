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
