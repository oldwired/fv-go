package grid

import (
	"bytes"
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// TestColumnZeroValueIsVisible regression for the `Visible` →
// `Hidden` flip — passing a bare Column{} now means "visible by
// default", which is what callers always wanted.
func TestColumnZeroValueIsVisible(t *testing.T) {
	g := New(geom.NewRect(0, 0, 40, 10),
		[]Column{{Title: "a"}, {Title: "b"}}, nil, nil)
	if g.Columns[0].Hidden || g.Columns[1].Hidden {
		t.Error("zero-value Column should be visible (Hidden=false)")
	}
}

// TestColumnHiddenTrueRespected — the key behavioral payoff of the
// flip: callers can now construct initially-hidden columns.
func TestColumnHiddenTrueRespected(t *testing.T) {
	g := New(geom.NewRect(0, 0, 40, 10),
		[]Column{{Title: "a"}, {Title: "b", Hidden: true}}, nil, nil)
	if !g.Columns[1].Hidden {
		t.Error("Column{Hidden: true} was forced visible by New()")
	}
	// colVisible is the predicate the renderer + hit-test use.
	if g.colVisible(1) {
		t.Error("colVisible should report false for Hidden column")
	}
	if !g.colVisible(0) {
		t.Error("colVisible should report true for visible column")
	}
}

// TestContainsFoldUnicode — `Müller` matches `mü`. The previous ASCII-
// only lowerASCII was a silent footgun on European data.
func TestContainsFoldUnicode(t *testing.T) {
	cases := []struct {
		s, sub string
		want   bool
	}{
		{"Müller", "mü", true},
		{"ÄSI", "äs", true},
		{"strasse", "STRASSE", true},
		{"foo", "bar", false},
		{"foo", "", true},
	}
	for _, c := range cases {
		got := containsFold(c.s, c.sub)
		if got != c.want {
			t.Errorf("containsFold(%q, %q) = %v, want %v", c.s, c.sub, got, c.want)
		}
	}
}

// TestSaveCSVEOFInQuoteSurfacesError — under the old hand-rolled
// parser, a paste with an unclosed quote silently succeeded. With
// encoding/csv it must error.
func TestLoadCSVEOFInQuoteSurfacesError(t *testing.T) {
	g := New(geom.NewRect(0, 0, 40, 10),
		[]Column{{Title: "x"}}, nil, nil)
	// Open a quote and never close it.
	malformed := []byte(`"unfinished`)
	err := g.LoadCSV(bytes.NewReader(malformed), DefaultCSVOptions())
	if err == nil {
		t.Error("LoadCSV should report an error for unterminated quoted field")
	}
}

// TestDefaultCSVOptionsHeader confirms DefaultCSVOptions() flips
// IncludeHeader on (the previous CSVOptions{} zero-value silently
// dropped headers because the comment claimed "default true").
func TestDefaultCSVOptionsHeader(t *testing.T) {
	g := New(geom.NewRect(0, 0, 40, 10),
		[]Column{{Title: "a"}, {Title: "b"}}, nil, nil)
	g.rows = [][]string{{"1", "2"}}
	var buf bytes.Buffer
	if err := g.SaveCSV(&buf, DefaultCSVOptions()); err != nil {
		t.Fatalf("SaveCSV: %v", err)
	}
	if !strings.Contains(buf.String(), "a,b") {
		t.Errorf("DefaultCSVOptions should include header; got %q", buf.String())
	}
}
