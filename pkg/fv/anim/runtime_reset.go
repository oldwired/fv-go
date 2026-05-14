package anim

// ResetForTest clears the animation registry. Tests only.
func ResetForTest() {
	mu.Lock()
	entries = nil
	mu.Unlock()
}
