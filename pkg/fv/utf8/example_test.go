package utf8_test

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/utf8"
)

// Sniff the encoding of a few byte slices. DetectEncoding inspects
// the leading bytes for a BOM, then falls back to UTF-8 validation,
// then ANSI/CP1252.
func ExampleDetectEncoding() {
	plain := []byte("hello, world")
	bom := []byte{0xEF, 0xBB, 0xBF, 'h', 'i'}
	utf16le := []byte{0xFF, 0xFE, 'h', 0, 'i', 0}

	fmt.Println(utf8.DetectEncoding(plain) == utf8.EncUTF8)
	fmt.Println(utf8.DetectEncoding(bom) == utf8.EncUTF8BOM)
	fmt.Println(utf8.DetectEncoding(utf16le) == utf8.EncUTF16LE)
	// Output:
	// true
	// true
	// true
}
