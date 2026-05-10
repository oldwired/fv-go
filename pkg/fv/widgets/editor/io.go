package editor

import "os"

// readFile / writeFile are thin wrappers so tests can fake them.
func readFile(path string) ([]byte, error)  { return os.ReadFile(path) }
func writeFile(path string, b []byte) error { return os.WriteFile(path, b, 0644) }
