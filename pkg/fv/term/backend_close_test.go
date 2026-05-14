//go:build unix

package term

import (
	"errors"
	"testing"
)

// TestPosixBackendCloseIdempotent confirms a double Close() doesn't
// panic, even though the underlying read loop and channels haven't
// been wired through Init (these tests use the struct directly).
func TestPosixBackendCloseIdempotent(t *testing.T) {
	b := &posixBackend{}
	// First call is a no-op (no channels initialized).
	if err := b.Close(); err != nil {
		t.Fatalf("first Close returned err: %v", err)
	}
	// Second call must not re-close anything or panic.
	if err := b.Close(); err != nil {
		t.Fatalf("second Close returned err: %v", err)
	}
}

// stubFailingWriter returns err from WriteString — used to check
// Flush's commit-after-success contract.
type stubFailingWriter struct{ err error }

func (s *stubFailingWriter) WriteString(string) (int, error) { return 0, s.err }

// TestFlushDoesNotCommitOnWriteError verifies that a failing write
// leaves the cell buffer dirty so the next Flush retries the same
// span. Earlier the front buffer was committed unconditionally,
// hiding the failed write.
func TestFlushDoesNotCommitOnWriteError(t *testing.T) {
	b := &posixBackend{}
	b.buf = newCellBuf(4, 1)
	b.enc = newSGREncoder(0)
	b.writer = &stubFailingWriter{err: errors.New("boom")}
	// Mark some dirty content so dirty() returns non-empty.
	b.buf.Clear(0)

	err := b.Flush()
	if err == nil {
		t.Fatal("Flush should propagate the writer error")
	}
	// After a failed write the buffer must still report dirt for the
	// next Flush — i.e., commit() did not run.
	if len(b.buf.dirty()) == 0 {
		t.Error("dirty cells were committed after failed write")
	}
}
