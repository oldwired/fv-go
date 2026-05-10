package drivers

// CommandSet is a 256-bit bitmask of command IDs in [0, 255].
//
// Mirrors Pascal `TCommandSet = set of Byte`. Used by views to declare
// which commands they currently handle (`EnableCommand` / `DisableCommand`).
// FV's command IDs above 255 (cmFileOpen=800 etc.) are not part of the
// per-view command set — they're broadcast events.
type CommandSet [32]byte

// Has reports whether cmd is in the set.
func (cs CommandSet) Has(cmd uint16) bool {
	if cmd > 255 {
		return false
	}
	return cs[cmd>>3]&(1<<(cmd&7)) != 0
}

// Set adds cmd.
func (cs *CommandSet) Set(cmd uint16) {
	if cmd > 255 {
		return
	}
	cs[cmd>>3] |= 1 << (cmd & 7)
}

// Clear removes cmd.
func (cs *CommandSet) Clear(cmd uint16) {
	if cmd > 255 {
		return
	}
	cs[cmd>>3] &^= 1 << (cmd & 7)
}

// Union returns cs ∪ other.
func (cs CommandSet) Union(other CommandSet) CommandSet {
	var out CommandSet
	for i := range cs {
		out[i] = cs[i] | other[i]
	}
	return out
}

// Intersect returns cs ∩ other.
func (cs CommandSet) Intersect(other CommandSet) CommandSet {
	var out CommandSet
	for i := range cs {
		out[i] = cs[i] & other[i]
	}
	return out
}

// Diff returns cs \ other.
func (cs CommandSet) Diff(other CommandSet) CommandSet {
	var out CommandSet
	for i := range cs {
		out[i] = cs[i] &^ other[i]
	}
	return out
}

// Equals reports value-equality.
func (cs CommandSet) Equals(other CommandSet) bool {
	return cs == other
}

// SetRange enables commands in [lo, hi].
func (cs *CommandSet) SetRange(lo, hi uint16) {
	if lo > 255 {
		return
	}
	if hi > 255 {
		hi = 255
	}
	for c := lo; c <= hi; c++ {
		cs.Set(c)
	}
}
