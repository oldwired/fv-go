package terminal

import "testing"

// TestParserPlainText verifies that ASCII bytes land at the cursor
// and the cursor advances one cell per glyph.
func TestParserPlainText(t *testing.T) {
	b := newBuffer(20, 5)
	p := newParser(b)
	p.Feed([]byte("hi"))
	if got := b.cells[0][0].Ch; got != 'h' {
		t.Errorf("cell[0][0] = %q, want 'h'", got)
	}
	if got := b.cells[0][1].Ch; got != 'i' {
		t.Errorf("cell[0][1] = %q, want 'i'", got)
	}
	if b.cursorR != 0 || b.cursorC != 2 {
		t.Errorf("cursor = (%d,%d), want (0,2)", b.cursorR, b.cursorC)
	}
}

// TestParserCRLF verifies that "\r\n" returns to col 0 and advances
// to the next row.
func TestParserCRLF(t *testing.T) {
	b := newBuffer(20, 5)
	p := newParser(b)
	p.Feed([]byte("ab\r\ncd"))
	if b.cursorR != 1 || b.cursorC != 2 {
		t.Errorf("cursor = (%d,%d), want (1,2)", b.cursorR, b.cursorC)
	}
	if b.cells[1][0].Ch != 'c' {
		t.Errorf("cells[1][0] = %q, want 'c'", b.cells[1][0].Ch)
	}
}

// TestParserCursorPosition verifies CSI Pn;Pn H positions the cursor.
func TestParserCursorPosition(t *testing.T) {
	b := newBuffer(20, 10)
	p := newParser(b)
	p.Feed([]byte("\x1b[3;5HX"))
	// CSI 3;5H = row 3, col 5 (1-based) → row 2, col 4.
	if b.cursorR != 2 || b.cursorC != 5 {
		t.Errorf("cursor after H+X = (%d,%d), want (2,5)", b.cursorR, b.cursorC)
	}
	if b.cells[2][4].Ch != 'X' {
		t.Errorf("cells[2][4] = %q, want 'X'", b.cells[2][4].Ch)
	}
}

// TestParserSGRColors verifies that 31;42 gives the cell red FG on
// green BG (in CGA-order: red=4, green=2).
func TestParserSGRColors(t *testing.T) {
	b := newBuffer(20, 5)
	p := newParser(b)
	p.Feed([]byte("\x1b[31;42mZ"))
	c := b.cells[0][0]
	if c.Ch != 'Z' {
		t.Errorf("cell.Ch = %q, want 'Z'", c.Ch)
	}
	// ANSI 31 (red) → CGA 4. ANSI 42 (green) → CGA 2.
	if c.FG != 4 {
		t.Errorf("FG = %d, want 4", c.FG)
	}
	if c.BG != 2 {
		t.Errorf("BG = %d, want 2", c.BG)
	}
}

// TestParserEraseLine verifies CSI K erases from cursor to EOL.
func TestParserEraseLine(t *testing.T) {
	b := newBuffer(10, 3)
	p := newParser(b)
	p.Feed([]byte("abcdefghij"))
	p.Feed([]byte("\x1b[5G\x1b[K"))
	// Cursor at column 5 (1-based) → col 4 (0-based).
	// Cells 4..9 should be blanks.
	for x := 4; x < 10; x++ {
		if b.cells[0][x].Ch != ' ' {
			t.Errorf("cells[0][%d] = %q, want space", x, b.cells[0][x].Ch)
		}
	}
	// Cells 0..3 still hold the originals.
	for x := 0; x < 4; x++ {
		if b.cells[0][x].Ch != rune('a'+x) {
			t.Errorf("cells[0][%d] = %q, want %q", x, b.cells[0][x].Ch, rune('a'+x))
		}
	}
}

// TestParserAltScreen verifies that DECSET 1049 swaps to the alt
// screen and DECRST 1049 restores original cells.
func TestParserAltScreen(t *testing.T) {
	b := newBuffer(10, 3)
	p := newParser(b)
	p.Feed([]byte("primary"))
	p.Feed([]byte("\x1b[?1049h"))
	if !b.altActive {
		t.Fatal("alt screen not active after DECSET 1049")
	}
	// On alt: write something different. We expect original cells preserved.
	p.Feed([]byte("\x1b[1;1Halt"))
	p.Feed([]byte("\x1b[?1049l"))
	if b.altActive {
		t.Fatal("alt screen still active after DECRST 1049")
	}
	// Original "primary" should still be on screen 0..6.
	for x, want := range "primary" {
		if b.cells[0][x].Ch != want {
			t.Errorf("after restore cells[0][%d] = %q, want %q", x, b.cells[0][x].Ch, want)
		}
	}
}

// TestParserOSCTitle verifies "ESC ] 0 ; <s> BEL" surfaces via OnTitle.
func TestParserOSCTitle(t *testing.T) {
	b := newBuffer(10, 3)
	p := newParser(b)
	got := ""
	p.OnTitle = func(s string) { got = s }
	p.Feed([]byte("\x1b]0;hello\x07"))
	if got != "hello" {
		t.Errorf("OnTitle got %q, want %q", got, "hello")
	}
}
