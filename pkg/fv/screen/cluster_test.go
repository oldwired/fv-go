package screen

import "testing"

// These tests exercise DrawStr's cluster-aware emission via uniseg.
// Each multi-codepoint cluster should land in one cellbuf cell, with
// a continuation cell (Ch="") if it's 2 cells wide. Subsequent
// characters land at the correct cell index — no drift.

// TestDrawStrRegionalFlag: 🇩🇪 is two regional indicators that form
// one wide cluster. Cell 0 = full flag string, cell 1 = continuation,
// cell 2 = the 'X' that follows.
func TestDrawStrRegionalFlag(t *testing.T) {
	buf := MakeDrawBuffer(10)
	flag := "\U0001F1E9\U0001F1EA"
	DrawStr(buf, 0, flag+"X", 0)

	if buf[0].Ch != flag {
		t.Errorf("buf[0].Ch = %q, want full flag %q", buf[0].Ch, flag)
	}
	if buf[1].Ch != "" {
		t.Errorf("buf[1].Ch = %q, want continuation (\"\")", buf[1].Ch)
	}
	if buf[2].Ch != "X" {
		t.Errorf("buf[2].Ch = %q, want X (no drift past wide cluster)", buf[2].Ch)
	}
}

// TestDrawStrSkinToneCluster: 👋🏼 — wave with skin tone modifier.
// One cluster, 2 cells.
func TestDrawStrSkinToneCluster(t *testing.T) {
	buf := MakeDrawBuffer(10)
	wave := "\U0001F44B\U0001F3FC"
	DrawStr(buf, 0, wave+"Z", 0)

	if buf[0].Ch != wave {
		t.Errorf("buf[0].Ch = %q, want full skin-tone cluster %q", buf[0].Ch, wave)
	}
	if buf[1].Ch != "" {
		t.Errorf("buf[1].Ch = %q, want continuation", buf[1].Ch)
	}
	if buf[2].Ch != "Z" {
		t.Errorf("buf[2].Ch = %q, want Z", buf[2].Ch)
	}
}

// TestDrawStrFamilyThenAscii: the 4-person family ZWJ cluster
// renders in cells 0..1; the trailing 'Y' lands at cell 2.
func TestDrawStrFamilyThenAscii(t *testing.T) {
	buf := MakeDrawBuffer(10)
	family := "\U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466"
	DrawStr(buf, 0, family+"Y", 0)

	if buf[0].Ch != family {
		t.Errorf("buf[0].Ch = %q, want full family %q", buf[0].Ch, family)
	}
	if buf[1].Ch != "" {
		t.Errorf("buf[1].Ch = %q, want continuation", buf[1].Ch)
	}
	if buf[2].Ch != "Y" {
		t.Errorf("buf[2].Ch = %q, want Y", buf[2].Ch)
	}
}

// TestDrawStrRainbowFlag: 🏳️\u200D🌈 is white-flag + VS16 + ZWJ + rainbow.
// One cluster, 2 cells.
func TestDrawStrRainbowFlag(t *testing.T) {
	buf := MakeDrawBuffer(10)
	rainbow := "\U0001F3F3️\u200D\U0001F308"
	DrawStr(buf, 0, rainbow+"!", 0)

	if buf[0].Ch != rainbow {
		t.Errorf("buf[0].Ch = %q, want full rainbow %q", buf[0].Ch, rainbow)
	}
	if buf[1].Ch != "" {
		t.Errorf("buf[1].Ch = %q, want continuation", buf[1].Ch)
	}
	if buf[2].Ch != "!" {
		t.Errorf("buf[2].Ch = %q, want '!'", buf[2].Ch)
	}
}

// TestDrawCStrHotkeyAroundCluster: '~' must split into normal/hot
// segments at byte boundaries; a multi-byte cluster within a
// segment is rendered intact.
func TestDrawCStrHotkeyAroundCluster(t *testing.T) {
	buf := MakeDrawBuffer(10)
	// "~F~lag 🇩🇪" — F is hotkey, lag is normal, then a flag.
	DrawCStr(buf, 0, "~F~lag \U0001F1E9\U0001F1EA", 0x07, 0x0F)
	// buf[0] = F (hot attr), buf[1-3] = lag, buf[4]=space, buf[5]=flag, buf[6]=continuation.
	if buf[0].Ch != "F" || buf[0].Attr != 0x0F {
		t.Errorf("buf[0] = %+v, want F with hot attr 0x0F", buf[0])
	}
	if buf[1].Ch != "l" || buf[1].Attr != 0x07 {
		t.Errorf("buf[1] = %+v, want l with normal attr", buf[1])
	}
	if buf[5].Ch != "\U0001F1E9\U0001F1EA" {
		t.Errorf("buf[5].Ch = %q, want flag cluster", buf[5].Ch)
	}
	if buf[6].Ch != "" {
		t.Errorf("buf[6].Ch = %q, want continuation", buf[6].Ch)
	}
}
