// Package history maintains per-ID history lists, used by InputLine
// (recall past values), ComboBox, file dialogs, and any widget that
// wants a "previously-entered values" dropdown.
//
// IDs are bytes: pick a constant in `consts` for each kind of input
// (e.g., HiFiles, HiDirectories) and pass it to InputLine.HistoryID.
// On commit the most recent value is pushed to the front of that ID's
// list, deduped, and trimmed to MaxItems.
package history

import "sync"

// MaxItems caps each ID's list. Older entries are dropped.
var MaxItems = 32

var (
	mu    sync.RWMutex
	store = map[byte][]string{}
)

// Add prepends s to id's list, deduping any existing copy and
// trimming to MaxItems. No-op for an empty string.
func Add(id byte, s string) {
	if s == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	list := store[id]
	for i, v := range list {
		if v == s {
			list = append(list[:i], list[i+1:]...)
			break
		}
	}
	list = append([]string{s}, list...)
	if len(list) > MaxItems {
		list = list[:MaxItems]
	}
	store[id] = list
}

// Get returns a copy of id's list, oldest entries last (so the
// most-recently-added value is at index 0).
func Get(id byte) []string {
	mu.RLock()
	defer mu.RUnlock()
	src := store[id]
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// Clear drops the list for id.
func Clear(id byte) {
	mu.Lock()
	delete(store, id)
	mu.Unlock()
}
