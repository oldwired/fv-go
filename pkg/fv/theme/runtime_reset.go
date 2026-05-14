package theme

// ResetForTest restores the default palette and clears the change
// hook. Tests only.
func ResetForTest() {
	mu.Lock()
	active = Default
	onChg = nil
	mu.Unlock()
}
