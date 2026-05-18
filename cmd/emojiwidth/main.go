// emojiwidth is a one-shot diagnostic that asks your terminal how
// many cells it actually advances the cursor for a handful of
// emoji glyphs. Run when fv-go renders with stray gaps or overflow
// on emoji-heavy rows — the report tells you which codepoints
// disagree between fv-go's UAX#11 width tables and your terminal's
// real rendering.
//
// Usage:
//
//	go run ./cmd/emojiwidth
//
// Must be run in an interactive terminal — it uses CSI 6n (Device
// Status Report) to ask the terminal where the cursor is before and
// after printing each glyph.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

func cursorCol(in *bufio.Reader) int {
	// Write the DSR query directly to /dev/tty so we bypass any
	// buffering between Go and the terminal. fmt.Print + Sync is
	// not always reliable on macOS ttys.
	if _, err := os.Stdout.WriteString("\x1b[6n"); err != nil {
		return -1
	}
	_ = os.Stdout.Sync() // best-effort; tty Sync may legitimately fail

	// Read until 'R'. Tolerate intervening bytes that aren't part of
	// the response (defensive against terminal-driven noise).
	var sb strings.Builder
	for {
		b, err := in.ReadByte()
		if err != nil {
			return -1
		}
		sb.WriteByte(b)
		if b == 'R' {
			break
		}
		// Safety: bail out after 64 bytes if 'R' never arrives.
		if sb.Len() > 64 {
			return -1
		}
	}
	s := sb.String()
	i := strings.Index(s, ";")
	if i < 0 {
		return -1
	}
	var col int
	if _, err := fmt.Sscanf(s[i+1:], "%d", &col); err != nil {
		return -1
	}
	return col
}

func main() {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Println("stdin is not a tty — run this directly in an interactive terminal")
		os.Exit(2)
	}
	st, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Println("make-raw failed:", err)
		os.Exit(2)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), st) }()

	in := bufio.NewReader(os.Stdin)

	cases := []struct {
		name  string
		glyph string
	}{
		{"✨            U+2728   sparkles (BMP, dual-presentation)", "✨"},
		{"✨️           U+2728+VS16              forced emoji", "✨️"},
		{"🚀            U+1F680  rocket", "🚀"},
		{"💫            U+1F4AB  dizzy", "💫"},
		{"🍎            U+1F34E  red apple", "🍎"},
		{"🏳️‍🌈           rainbow ZWJ cluster", "🏳️‍🌈"},
		{"👨‍👩‍👧‍👦         family ZWJ cluster", "👨‍👩‍👧‍👦"},
		{"日            U+65E5  CJK", "日"},
		{"em-dash       U+2014", "—"},
		{"↕  U+2195   up-down arrow (zoom icon in title bar)", "↕"},
		{"[↕]  zoom icon as drawn by Frame", "[↕]"},
		{"■  U+25A0   close-button glyph", "■"},
		{"[■]  close icon as drawn by Frame", "[■]"},
		{"◢  U+25E2   resize handle", "◢"},
		{"FULL EMOJI ROW", "🍎🍊🍋  👨‍👩‍👧‍👦  🏳️‍🌈  ✨🚀💫"},
		{"FULL CJK ROW", "你好世界 — 日本語 — 한국어"},
		{"DEMO StaticText emoji line (60 cells expected)",
			"Emoji:    🍎🍊🍋  👨‍👩‍👧‍👦  🏳️‍🌈  ✨🚀💫                            "},
		{"DEMO StaticText CJK   line (60 cells expected)",
			"CJK:      你好世界 — 日本語 — 한국어                        "},
		{"é (NFC)      U+00E9   precomposed Latin-1", "é"},
		{"e + combining U+0065 U+0301 (NFD)", "é"},
	}

	fmt.Print("\r\n  --- emoji-width probe ---\r\n")
	for _, c := range cases {
		fmt.Print("\r\n")
		c0 := cursorCol(in)
		fmt.Print(c.glyph)
		c1 := cursorCol(in)
		// Pad to a consistent column so the labels line up.
		needPad := 6 - (c1 - c0)
		if needPad < 0 {
			needPad = 0
		}
		fmt.Printf("%s  → %d cell(s)   %s",
			strings.Repeat(" ", needPad), c1-c0, c.name)
	}
	fmt.Print("\r\n\r\n  Paste this output to me; the cell-count column is what I need.\r\n")
}
