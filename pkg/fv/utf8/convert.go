package utf8

import (
	"unicode/utf16"
	stdutf8 "unicode/utf8"
)

// cp1252Map covers the high bytes (0x80..0x9F) of CP1252 that diverge
// from ISO-8859-1. Bytes outside this range are identical to Latin-1
// and pass through as the rune of the same numeric value.
var cp1252Map = [...]rune{
	0x20AC, 0xFFFD, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021, // 0x80
	0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0xFFFD, 0x017D, 0xFFFD, // 0x88
	0xFFFD, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014, // 0x90
	0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0xFFFD, 0x017E, 0x0178, // 0x98
}

// ANSIToUTF8 converts a CP1252 byte slice to UTF-8.
func ANSIToUTF8(data []byte) []byte {
	out := make([]byte, 0, len(data))
	var buf [4]byte
	for _, b := range data {
		var r rune
		switch {
		case b < 0x80:
			out = append(out, b)
			continue
		case b >= 0x80 && b < 0xA0:
			r = cp1252Map[b-0x80]
		default:
			r = rune(b)
		}
		n := stdutf8.EncodeRune(buf[:], r)
		out = append(out, buf[:n]...)
	}
	return out
}

// UTF16LEToUTF8 converts a UTF-16 little-endian byte slice to UTF-8.
// If skipBOM is true, a leading FF FE is removed.
func UTF16LEToUTF8(data []byte, skipBOM bool) []byte {
	return utf16ToUTF8(data, skipBOM, false)
}

// UTF16BEToUTF8 converts a UTF-16 big-endian byte slice to UTF-8.
func UTF16BEToUTF8(data []byte, skipBOM bool) []byte {
	return utf16ToUTF8(data, skipBOM, true)
}

func utf16ToUTF8(data []byte, skipBOM, bigEndian bool) []byte {
	if skipBOM && len(data) >= 2 {
		if (!bigEndian && data[0] == 0xFF && data[1] == 0xFE) ||
			(bigEndian && data[0] == 0xFE && data[1] == 0xFF) {
			data = data[2:]
		}
	}
	codeUnits := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		var u uint16
		if bigEndian {
			u = uint16(data[i])<<8 | uint16(data[i+1])
		} else {
			u = uint16(data[i]) | uint16(data[i+1])<<8
		}
		codeUnits = append(codeUnits, u)
	}
	runes := utf16.Decode(codeUnits)
	return []byte(string(runes))
}

// ConvertToUTF8 strips any BOM and returns UTF-8 bytes for the given
// encoding. Pure UTF-8 inputs are returned as-is (BOM removed).
func ConvertToUTF8(data []byte, enc FileEncoding) []byte {
	switch enc {
	case EncUTF8:
		return data
	case EncUTF8BOM:
		if len(data) >= 3 {
			return data[3:]
		}
		return data
	case EncUTF16LE:
		return UTF16LEToUTF8(data, true)
	case EncUTF16BE:
		return UTF16BEToUTF8(data, true)
	case EncANSI:
		return ANSIToUTF8(data)
	}
	return data
}
