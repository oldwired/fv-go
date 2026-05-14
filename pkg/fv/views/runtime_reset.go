package views

// ResetForTest resets package-level runtime hooks to their zero state.
// It is intended for tests only — production callers should not invoke
// it. Exported (not under a build tag) so cross-package tests in
// dialogs/, menus/, widgets/* can call it without dragging in the
// internal-state of `views`.
//
// globalQueue is set to nil (not a fresh queue) deliberately. A test
// running without an installed Program should fail loudly when it
// reaches code that posts events, rather than silently queue them
// into a zombie buffer that no dispatch loop will ever drain.
func ResetForTest() {
	globalQueue = nil
	rootBackend = nil
	pumpFn = nil
	waitFn = nil
	dirtyFn = nil
	callSoonFn = nil
}
