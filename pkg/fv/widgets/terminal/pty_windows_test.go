//go:build windows

package terminal

import (
	"strings"
	"testing"
)

// TestUtf16EnvBlockSurvivesMultipleEntries is the regression for the
// project-review's headline ConPTY bug: the earlier implementation
// joined entries with NULs into a single string and called
// syscall.UTF16FromString, which silently stopped at the first NUL —
// so only the first env var was actually passed to the child.
func TestUtf16EnvBlockSurvivesMultipleEntries(t *testing.T) {
	got, err := utf16EnvBlock([]string{"FOO=bar", "BAZ=qux", "NAME=Müller"})
	if err != nil {
		t.Fatalf("utf16EnvBlock: %v", err)
	}
	// Decode back to a flat string with NULs as field separators.
	var sb strings.Builder
	for _, u := range got {
		if u == 0 {
			sb.WriteByte('|')
			continue
		}
		sb.WriteRune(rune(u))
	}
	out := sb.String()
	for _, want := range []string{"FOO=bar", "BAZ=qux", "NAME=Müller"} {
		if !strings.Contains(out, want) {
			t.Errorf("env block missing entry %q; got %q", want, out)
		}
	}
	// Final double-NUL terminator.
	if !strings.HasSuffix(out, "||") {
		t.Errorf("env block must end with double-NUL terminator; got %q", out)
	}
}

// TestUtf16EnvBlockRejectsEmbeddedNUL guards the explicit error.
func TestUtf16EnvBlockRejectsEmbeddedNUL(t *testing.T) {
	if _, err := utf16EnvBlock([]string{"BAD=val\x00ue"}); err == nil {
		t.Error("expected error for entry containing NUL byte")
	}
}

// TestQuoteWindowsArgRoundTrips checks the cases that the previous
// naive quoter mishandled — most importantly trailing backslashes
// before the closing quote (paths like "C:\Program Files\Foo\").
func TestQuoteWindowsArgRoundTrips(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Plain.
		{`abc`, `abc`},
		// Spaces force wrapping.
		{`hello world`, `"hello world"`},
		// Trailing backslash in a quoted arg: must double the backslash
		// so it isn't eaten by the closing quote.
		{`C:\Program Files\Foo\`, `"C:\Program Files\Foo\\"`},
		// Embedded double-quote needs escaping.
		{`say "hi"`, `"say \"hi\""`},
		// Backslash-quote sequence.
		{`a\"b`, `"a\\\"b"`},
		// Empty argument: must still produce a placeholder pair so
		// argv[] gets the slot.
		{``, `""`},
	}
	for _, c := range cases {
		got := quoteWindowsArg(c.in)
		if got != c.want {
			t.Errorf("quoteWindowsArg(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}
