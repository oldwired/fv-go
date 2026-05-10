// Package validators ports Validate.pas: input validators that
// InputLine consults before accepting user-typed text.
//
// Each validator implements Validator. The default implementation in
// Base validates anything; subtypes override IsValid / IsValidInput
// to add restrictions.
package validators

import (
	"strconv"
	"strings"
)

// Status codes (vsOk / vsSyntax in Pascal).
const (
	StatusOK     = 0
	StatusSyntax = 1
)

// Option flags.
const (
	OptFill     = 0x01
	OptTransfer = 0x02
	OptOnAppend = 0x04
)

// Validator is the per-field contract. InputLine calls IsValidInput
// after each keystroke and IsValid before accepting a value.
type Validator interface {
	// IsValid reports whether s is a complete, accepted value.
	IsValid(s string) bool

	// IsValidInput reports whether s could be extended into a valid
	// value. Used to reject keystrokes that would make the field
	// unrecoverable. The string can be modified (e.g., autofill);
	// suppressFill disables that.
	IsValidInput(s *string, suppressFill bool) bool
}

// Base is a noop validator: every input is valid.
type Base struct {
	Status  uint16
	Options uint16
}

func (Base) IsValid(string) bool             { return true }
func (Base) IsValidInput(*string, bool) bool { return true }

// PXPictureValidator validates against a Borland Paradox-style picture
// string: '#' = digit, '?' = letter, '&' = letter (uppercased), '@' =
// any char, '!' = any (uppercased), literal others. Square brackets
// mark optional groups, '*' repeats. We implement a useful subset:
// '#', '?', '&', '@', '!', literal characters; brackets and '*' are
// not supported (left for future work).
type PXPictureValidator struct {
	Base
	Pic      string
	AutoFill bool
}

// NewPXPictureValidator returns a picture validator. autoFill enables
// inserting literal characters into the input as the user types.
func NewPXPictureValidator(picture string, autoFill bool) *PXPictureValidator {
	return &PXPictureValidator{Pic: picture, AutoFill: autoFill}
}

// IsValid checks whether s satisfies the picture exactly.
func (v *PXPictureValidator) IsValid(s string) bool {
	return v.matches(s, true)
}

// IsValidInput rejects keystrokes that already break the picture.
func (v *PXPictureValidator) IsValidInput(s *string, suppressFill bool) bool {
	if v.AutoFill && !suppressFill {
		// Fill literal positions where the user hasn't typed yet.
		filled := v.fill(*s)
		*s = filled
	}
	return v.matches(*s, false)
}

func (v *PXPictureValidator) matches(s string, complete bool) bool {
	pi, si := 0, 0
	for pi < len(v.Pic) && si < len(s) {
		p := v.Pic[pi]
		c := s[si]
		switch p {
		case '#':
			if c < '0' || c > '9' {
				return false
			}
		case '?':
			if !isLetter(c) {
				return false
			}
		case '&':
			if !isLetter(c) {
				return false
			}
		case '@':
			// any char
		case '!':
			// any char, uppercased
		default:
			if p != c {
				return false
			}
		}
		pi++
		si++
	}
	if complete {
		return pi == len(v.Pic) && si == len(s)
	}
	return true
}

func (v *PXPictureValidator) fill(s string) string {
	if len(s) >= len(v.Pic) {
		return s
	}
	// If picture has a literal at the next position, append it.
	for len(s) < len(v.Pic) {
		p := v.Pic[len(s)]
		if isPicMeta(p) {
			break
		}
		s += string(p)
	}
	return s
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isPicMeta(c byte) bool {
	return c == '#' || c == '?' || c == '&' || c == '@' || c == '!'
}

// FilterValidator only accepts characters in ValidChars.
type FilterValidator struct {
	Base
	ValidChars string
}

// NewFilterValidator builds a validator that allows only the runes in
// chars.
func NewFilterValidator(chars string) *FilterValidator {
	return &FilterValidator{ValidChars: chars}
}

func (v *FilterValidator) IsValid(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune(v.ValidChars, r) {
			return false
		}
	}
	return true
}

func (v *FilterValidator) IsValidInput(s *string, suppressFill bool) bool {
	return v.IsValid(*s)
}

// RangeValidator accepts integers in [Min, Max].
type RangeValidator struct {
	FilterValidator
	Min, Max int64
}

// NewRangeValidator returns an integer-range validator.
func NewRangeValidator(min, max int64) *RangeValidator {
	return &RangeValidator{
		FilterValidator: *NewFilterValidator("0123456789-+"),
		Min:             min,
		Max:             max,
	}
}

func (v *RangeValidator) IsValid(s string) bool {
	if !v.FilterValidator.IsValid(s) {
		return false
	}
	if s == "" {
		return false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return false
	}
	return n >= v.Min && n <= v.Max
}

// LookupValidator accepts a string only if it appears in a fixed set.
type LookupValidator struct {
	Base
	Strings []string
}

// NewLookupValidator returns a validator that accepts only the given
// strings (case-sensitive).
func NewLookupValidator(strings []string) *LookupValidator {
	return &LookupValidator{Strings: strings}
}

func (v *LookupValidator) IsValid(s string) bool {
	for _, x := range v.Strings {
		if x == s {
			return true
		}
	}
	return false
}

func (v *LookupValidator) IsValidInput(s *string, suppressFill bool) bool {
	// Allow any prefix of any allowed value.
	for _, x := range v.Strings {
		if strings.HasPrefix(x, *s) {
			return true
		}
	}
	return false
}
