package clipboard_test

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/clipboard"
)

// The package keeps an in-memory text buffer that mirrors the OS
// clipboard when an OSC 52 writer is wired. Without a writer it's a
// usable scratch buffer for tests and any host that doesn't want to
// touch the OS clipboard.
func ExampleSetText() {
	clipboard.SetText("a snippet of code")
	fmt.Println(clipboard.GetText())
	// Output: a snippet of code
}
