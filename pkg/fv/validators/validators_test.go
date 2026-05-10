package validators

import "testing"

func TestPXPicture(t *testing.T) {
	v := NewPXPictureValidator("###-##", false)
	if !v.IsValid("123-45") {
		t.Error("123-45 should be valid")
	}
	if v.IsValid("12-345") {
		t.Error("12-345 should not match")
	}
	if v.IsValid("abc-de") {
		t.Error("letters should not match digits")
	}
	if v.IsValid("123-456") {
		t.Error("too long should fail")
	}
	if v.IsValid("123-4") {
		t.Error("too short should fail in IsValid")
	}
	// Partial input should be valid for IsValidInput.
	s := "12"
	if !v.IsValidInput(&s, true) {
		t.Error("partial 12 should be valid input")
	}
}

func TestFilter(t *testing.T) {
	v := NewFilterValidator("abc")
	if !v.IsValid("aabbcc") {
		t.Error("abc-only should pass")
	}
	if v.IsValid("aabbcd") {
		t.Error("d should be rejected")
	}
}

func TestRange(t *testing.T) {
	v := NewRangeValidator(1, 100)
	if !v.IsValid("50") {
		t.Error("50 should be valid")
	}
	if v.IsValid("0") {
		t.Error("0 should be too small")
	}
	if v.IsValid("101") {
		t.Error("101 should be too large")
	}
	if v.IsValid("abc") {
		t.Error("abc should be invalid")
	}
}

func TestLookup(t *testing.T) {
	v := NewLookupValidator([]string{"red", "green", "blue"})
	if !v.IsValid("red") {
		t.Error("red should be valid")
	}
	if v.IsValid("yellow") {
		t.Error("yellow should be invalid")
	}
	prefix := "gre"
	if !v.IsValidInput(&prefix, true) {
		t.Error("prefix gre should be valid input")
	}
	bad := "xyz"
	if v.IsValidInput(&bad, true) {
		t.Error("xyz prefix should be invalid input")
	}
}
