package clipboard

// ResetForTest clears the buffer, writer, and policy. Tests only.
func ResetForTest() {
	mu.Lock()
	buf = ""
	writer = nil
	policy = defaultPolicy
	mu.Unlock()
}
