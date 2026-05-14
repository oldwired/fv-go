//go:build windows

// Package conpty isolates one Win32 attribute call that needs a
// uintptr → unsafe.Pointer conversion (PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE
// per Microsoft's official ConPTY sample). Carved out so CI can run
// `go vet -unsafeptr=false` on this package only; every other package
// keeps the full default analyzer set.
//
// In C, HPCON is a typedef for void* and the attribute API takes the
// handle value directly as lpValue (PVOID). The Go translation of
// `windows.Handle` is a uintptr, so the conversion is unavoidable on
// the Go side — and `go vet`'s unsafeptr check flags every such
// conversion, with no semantic awareness of "this uintptr is a real
// kernel-issued pointer." Hence the per-package suppression at the
// CI level rather than a per-line trick in source.
package conpty

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// SetPseudoConsoleAttribute attaches hPC to attrList as the
// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE value, so the about-to-be-spawned
// child process inherits the pseudoconsole.
//
// Documented Win32 contract (and Microsoft's ConPTY sample):
// the kernel reads lpValue as the HPCON handle itself, not as a
// pointer to handle storage; sizeof(HPCON) bytes are stored verbatim.
// See https://learn.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session
func SetPseudoConsoleAttribute(
	attrList *windows.ProcThreadAttributeListContainer,
	hPC windows.Handle,
) error {
	return attrList.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		unsafe.Pointer(hPC),
		unsafe.Sizeof(hPC),
	)
}
