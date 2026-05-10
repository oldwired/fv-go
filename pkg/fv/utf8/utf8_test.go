package utf8

import "testing"

func TestDetectEncoding(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want FileEncoding
	}{
		{"empty", nil, EncUnknown},
		{"ascii", []byte("hello"), EncUTF8},
		{"utf8", []byte("héllo"), EncUTF8},
		{"utf8bom", []byte{0xEF, 0xBB, 0xBF, 'a'}, EncUTF8BOM},
		{"utf16le", []byte{0xFF, 0xFE, 'a', 0x00}, EncUTF16LE},
		{"utf16be", []byte{0xFE, 0xFF, 0x00, 'a'}, EncUTF16BE},
		{"ansi", []byte{0xE9, 0xE0}, EncANSI}, // not valid UTF-8
	}
	for _, c := range cases {
		if got := DetectEncoding(c.in); got != c.want {
			t.Errorf("%s: DetectEncoding=%d want %d", c.name, got, c.want)
		}
	}
}

func TestStringDisplayWidth(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"", 0},
		{"hello", 5},
		{"héllo", 5},                 // é is precomposed, width 1
		{"éllo", 4},                 // e + combining acute is one 1-cell cluster, plus l,l,o = 4 cells
		{"日本語", 6},                   // CJK
		{"\U0001F600", 2},            // emoji 😀
		{"\U0001F468‍\U0001F469", 2}, // ZWJ family fragment - one wide cluster
		{"x️", 2},                    // VS16 promotes width 1 -> 2
	}
	for _, c := range cases {
		if got := StringDisplayWidth(c.s); got != c.want {
			t.Errorf("StringDisplayWidth(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}

func TestCStrDisplayWidth(t *testing.T) {
	if w := CStrDisplayWidth("~F~ile"); w != 4 {
		t.Errorf("CStr ~F~ile: got %d want 4", w)
	}
	if w := CStrDisplayWidth("File"); w != 4 {
		t.Errorf("CStr File: got %d want 4", w)
	}
}

func TestCopyDisplayCells(t *testing.T) {
	cases := []struct {
		name       string
		s          string
		start, max int
		want       string
	}{
		{"head", "hello", 0, 3, "hel"},
		{"middle", "hello world", 6, 5, "world"},
		{"narrow_emoji", "ab\U0001F600cd", 0, 3, "ab"}, // emoji at col 2 is wide; would straddle col 3
		{"emoji_fits", "ab\U0001F600cd", 0, 4, "ab\U0001F600"},
		{"empty_max", "hello", 0, 0, ""},
		{"start_negative", "hello", -3, 5, "hello"},
	}
	for _, c := range cases {
		if got := CopyDisplayCells(c.s, c.start, c.max); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestCharLen(t *testing.T) {
	if CharLen('A') != 1 {
		t.Error("ASCII")
	}
	if CharLen(0xC3) != 2 {
		t.Error("2-byte lead")
	}
	if CharLen(0xE3) != 3 {
		t.Error("3-byte lead")
	}
	if CharLen(0xF0) != 4 {
		t.Error("4-byte lead")
	}
	if CharLen(0xFF) != 0 {
		t.Error("invalid")
	}
	if !IsTrailByte(0x80) {
		t.Error("80 is trail")
	}
	if IsTrailByte(0x40) {
		t.Error("40 is not trail")
	}
}

func TestANSIToUTF8(t *testing.T) {
	in := []byte{'h', 'i', 0x80, 0xE9} // euro, é
	out := ANSIToUTF8(in)
	want := "hi€é"
	if string(out) != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestUTF16ToUTF8(t *testing.T) {
	// "hi" in UTF-16LE with BOM
	le := []byte{0xFF, 0xFE, 'h', 0, 'i', 0}
	if got := string(UTF16LEToUTF8(le, true)); got != "hi" {
		t.Errorf("LE: got %q", got)
	}
	be := []byte{0xFE, 0xFF, 0, 'h', 0, 'i'}
	if got := string(UTF16BEToUTF8(be, true)); got != "hi" {
		t.Errorf("BE: got %q", got)
	}
}
