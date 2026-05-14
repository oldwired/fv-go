package terminal

import (
	"strings"
	"testing"
)

// TestSanitizeOSCStringStripsC0 verifies that NUL, BEL, CR, LF, ESC,
// and DEL are removed but TAB survives.
func TestSanitizeOSCStringStripsC0(t *testing.T) {
	in := "OK\x00\x07\r\n\x1b\x7fdone\there"
	got := sanitizeOSCString(in, 1024)
	if strings.ContainsAny(got, "\x00\x07\r\n\x1b\x7f") {
		t.Errorf("C0 codes leaked through: %q", got)
	}
	if !strings.Contains(got, "done\there") {
		t.Errorf("TAB was stripped: %q", got)
	}
}

// TestSanitizeOSCStringRuneSafeTruncation: cutting a multi-byte rune
// at the byte boundary must not produce invalid UTF-8.
func TestSanitizeOSCStringRuneSafeTruncation(t *testing.T) {
	// "ä" is 0xC3 0xA4. Cap at 1 byte and we'd otherwise emit half a rune.
	got := sanitizeOSCString("äbc", 1)
	if got != "" {
		t.Errorf("rune-safe truncation should drop the half-rune; got %q", got)
	}
	// 2 bytes is exactly the size of "ä".
	got = sanitizeOSCString("äbc", 2)
	if got != "ä" {
		t.Errorf("expected just 'ä' for 2-byte cap; got %q", got)
	}
}

// TestSanitizeOSCStringTitleCap matches xterm's 512-byte convention.
func TestSanitizeOSCStringTitleCap(t *testing.T) {
	in := strings.Repeat("x", 1000)
	got := sanitizeOSCString(in, 512)
	if len(got) > 512 {
		t.Errorf("title exceeded 512-byte cap: got len=%d", len(got))
	}
}
