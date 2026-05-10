// Package streams provides FV-style stream wrappers around Go's standard
// io interfaces.
//
// The Pascal TFVStream / TDosStream / TBufStream / TMemoryStream
// hierarchy maps to: just use io.Reader / io.Writer / io.Seeker. This
// package exists for naming continuity (so a port of, say, Editors.pas
// can refer to "a Stream") and to centralize a few helpers like
// LoadFile / SaveFile that the Pascal code expects to find here.
package streams

import (
	"bytes"
	"io"
	"os"
)

// LoadFile reads the entire file at path into a byte slice.
func LoadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// SaveFile writes data to path with mode 0644, creating or truncating
// as needed.
func SaveFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

// MemoryStream is a tiny io.ReadWriteSeeker over an in-memory byte slice.
// Mirrors TMemoryStream's role as a scratch buffer.
type MemoryStream struct {
	buf []byte
	pos int64
}

// NewMemoryStream returns an empty MemoryStream.
func NewMemoryStream() *MemoryStream { return &MemoryStream{} }

// Bytes returns the underlying buffer (do not modify).
func (m *MemoryStream) Bytes() []byte { return m.buf }

// Size returns the buffer length.
func (m *MemoryStream) Size() int64 { return int64(len(m.buf)) }

func (m *MemoryStream) Read(p []byte) (int, error) {
	if m.pos >= int64(len(m.buf)) {
		return 0, io.EOF
	}
	n := copy(p, m.buf[m.pos:])
	m.pos += int64(n)
	return n, nil
}

func (m *MemoryStream) Write(p []byte) (int, error) {
	end := m.pos + int64(len(p))
	if end > int64(len(m.buf)) {
		grown := make([]byte, end)
		copy(grown, m.buf)
		m.buf = grown
	}
	n := copy(m.buf[m.pos:], p)
	m.pos += int64(n)
	return n, nil
}

func (m *MemoryStream) Seek(offset int64, whence int) (int64, error) {
	var pos int64
	switch whence {
	case io.SeekStart:
		pos = offset
	case io.SeekCurrent:
		pos = m.pos + offset
	case io.SeekEnd:
		pos = int64(len(m.buf)) + offset
	default:
		return 0, os.ErrInvalid
	}
	if pos < 0 {
		return 0, os.ErrInvalid
	}
	m.pos = pos
	return pos, nil
}

// Truncate shrinks the buffer to length n. Position is clamped.
func (m *MemoryStream) Truncate(n int64) {
	if n < 0 {
		n = 0
	}
	if int64(len(m.buf)) > n {
		m.buf = m.buf[:n]
	}
	if m.pos > n {
		m.pos = n
	}
}

// Reset rewinds and discards the buffer.
func (m *MemoryStream) Reset() {
	m.buf = nil
	m.pos = 0
}

// ReadAll consumes all remaining bytes from r and returns them.
// Convenience wrapper kept here so callers can stay inside the
// streams namespace.
func ReadAll(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	return buf.Bytes(), err
}
