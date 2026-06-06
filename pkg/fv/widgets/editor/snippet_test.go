package editor

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
)

func TestParseSnippet(t *testing.T) {
	resolve := func(name string) (string, bool) {
		if name == "TM_FILENAME" {
			return "main.go", true
		}
		return "", false
	}
	cases := []struct {
		name     string
		body     string
		expanded string
		stops    map[int][]int // index → instance offsets
		wantErr  bool
	}{
		{name: "plain", body: "hello", expanded: "hello"},
		{name: "bare stop", body: "f($1)", expanded: "f()", stops: map[int][]int{1: {2}}},
		{name: "braced stop", body: "f(${1})", expanded: "f()", stops: map[int][]int{1: {2}}},
		{name: "default", body: "f(${1:x})", expanded: "f(x)", stops: map[int][]int{1: {2}}},
		{name: "two stops", body: "$1+$2", expanded: "+", stops: map[int][]int{1: {0}, 2: {1}}},
		{name: "terminal", body: "if $1 {$0}", expanded: "if  {}", stops: map[int][]int{1: {3}, 0: {5}}},
		{name: "mirrors", body: "$1 = $1", expanded: " = ", stops: map[int][]int{1: {0, 3}}},
		{name: "mirror seed", body: "${1:v} := $1", expanded: "v := v", stops: map[int][]int{1: {0, 5}}},
		{name: "var resolved", body: "// $TM_FILENAME", expanded: "// main.go"},
		{name: "var braced default", body: "${UNKNOWN:fallback}", expanded: "fallback"},
		{name: "var unresolved empty", body: "[$UNKNOWN]", expanded: "[]"},
		{name: "escaped dollar", body: "\\$1", expanded: "$1"},
		{name: "escaped backslash", body: "a\\\\b", expanded: "a\\b"},
		{name: "escaped brace in default", body: "${1:a\\}b}", expanded: "a}b", stops: map[int][]int{1: {0}}},
		{name: "dollar before space", body: "5$ x", expanded: "5$ x"},
		{name: "dollar at end", body: "x$", expanded: "x$"},
		{name: "default with colon", body: "${1:a:b}", expanded: "a:b", stops: map[int][]int{1: {0}}},
		{name: "dollar in default literal", body: "${1:$x}", expanded: "$x", stops: map[int][]int{1: {0}}},
		{name: "unterminated brace", body: "${1:oops", wantErr: true},
		{name: "bad name", body: "${1bad}", wantErr: true},
		{name: "empty braces", body: "${}", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			expanded, stops, err := parseSnippet(c.body, resolve)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got %q", expanded)
				}
				return
			}
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if expanded != c.expanded {
				t.Errorf("expanded = %q, want %q", expanded, c.expanded)
			}
			got := map[int][]int{}
			for _, s := range stops {
				got[s.index] = s.offsets
			}
			for idx, offs := range c.stops {
				g := got[idx]
				if len(g) != len(offs) {
					t.Fatalf("stop %d instances = %v, want %v", idx, g, offs)
				}
				for i := range offs {
					if g[i] != offs[i] {
						t.Errorf("stop %d offsets = %v, want %v", idx, g, offs)
					}
				}
			}
			if len(got) != len(c.stops) {
				t.Errorf("stop set = %v, want %v", got, c.stops)
			}
		})
	}
}

func TestParseSnippetNavigationOrder(t *testing.T) {
	_, stops, err := parseSnippet("$0 then $2 then $1", nil)
	if err != nil {
		t.Fatal(err)
	}
	order := []int{}
	for _, s := range stops {
		order = append(order, s.index)
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 0 {
		t.Errorf("navigation order = %v, want [1 2 0]", order)
	}
}

func key(code uint16) *drivers.Event {
	return &drivers.Event{What: consts.EvKeyDown, KeyCode: code}
}

func TestSnippetSessionBasics(t *testing.T) {
	e := newTestEditor()
	e.SetText("func main() {\n\t\n}\n")
	e.MoveCursor(15, false) // inside the body
	if err := e.InsertSnippet("for ${1:i} := 0; $1 < ${2:n}; $1++ {$0}"); err != nil {
		t.Fatal(err)
	}
	if !e.SnippetActive() {
		t.Fatal("session must be active")
	}
	if got := e.Text(); got != "func main() {\n\tfor i := 0; i < n; i++ {}\n}\n" {
		t.Fatalf("expanded text: %q", got)
	}
	// Stop 1 selected with mirrors: 1 primary + 2 extras.
	if len(e.Carets()) != 3 {
		t.Fatalf("carets = %d, want 3 (mirrors)", len(e.Carets()))
	}
	// Typing replaces all three instances.
	e.Insert("idx")
	if got := e.Text(); got != "func main() {\n\tfor idx := 0; idx < n; idx++ {}\n}\n" {
		t.Fatalf("after mirror typing: %q", got)
	}
	// Tab → stop 2 ("n" selected).
	e.HandleEvent(key(consts.KbTab))
	if !e.HasSelection() {
		t.Fatal("stop 2 default must be selected")
	}
	lo, hi := e.selRange()
	if string(e.Buf.data[lo:hi]) != "n" {
		t.Errorf("stop 2 selection = %q, want n", string(e.Buf.data[lo:hi]))
	}
	e.Insert("count")
	// Tab → $0, session ends.
	e.HandleEvent(key(consts.KbTab))
	if e.SnippetActive() {
		t.Error("session must end at $0")
	}
	if got := e.Text(); got != "func main() {\n\tfor idx := 0; idx < count; idx++ {}\n}\n" {
		t.Fatalf("final text: %q", got)
	}
}

func TestSnippetSingleUndo(t *testing.T) {
	e := newTestEditor()
	e.SetText("abc")
	e.MoveCursor(3, false)
	_ = e.InsertSnippet("X${1:one}Y${2:two}Z")
	e.Undo()
	if got := e.Text(); got != "abc" {
		t.Fatalf("snippet insertion must be one undo entry: %q", got)
	}
	if e.SnippetActive() {
		t.Error("undoing the insertion must end the session")
	}
}

func TestSnippetShiftTabBack(t *testing.T) {
	e := newTestEditor()
	_ = e.InsertSnippet("${1:a}-${2:b}")
	e.HandleEvent(key(consts.KbTab))
	lo, hi := e.selRange()
	if string(e.Buf.data[lo:hi]) != "b" {
		t.Fatalf("after Tab selection = %q, want b", string(e.Buf.data[lo:hi]))
	}
	e.HandleEvent(key(consts.KbShiftTab))
	lo, hi = e.selRange()
	if string(e.Buf.data[lo:hi]) != "a" {
		t.Errorf("after Shift-Tab selection = %q, want a", string(e.Buf.data[lo:hi]))
	}
}

func TestSnippetShiftTabWithoutSessionNotConsumed(t *testing.T) {
	e := newTestEditor()
	e.SetText("x")
	ev := key(consts.KbShiftTab)
	e.HandleEvent(ev)
	if ev.What == consts.EvNothing {
		t.Error("Shift-Tab without a session must stay with group focus-nav")
	}
}

func TestSnippetEscEndsSessionKeepsText(t *testing.T) {
	e := newTestEditor()
	_ = e.InsertSnippet("${1:keep}")
	ev := key(consts.KbEsc)
	e.HandleEvent(ev)
	if e.SnippetActive() {
		t.Error("Esc must end the session")
	}
	if ev.What != consts.EvNothing {
		t.Error("Esc that ended a session must be consumed")
	}
	if got := e.Text(); got != "keep" {
		t.Errorf("text after Esc = %q, want keep", got)
	}
	// Tab now inserts a literal tab again.
	e.MoveCursor(4, false)
	e.HandleEvent(key(consts.KbTab))
	if got := e.Text(); got != "keep\t" {
		t.Errorf("literal tab after session end: %q", got)
	}
}

func TestSnippetClickOutsideEndsSession(t *testing.T) {
	e := newTestEditor()
	e.SetText("header\n")
	e.MoveCursor(7, false)
	_ = e.InsertSnippet("${1:x}")
	e.MoveCursor(0, false)
	if e.SnippetActive() {
		t.Error("caret leaving bounds must end the session")
	}
}

func TestSnippetForeignEditEndsSession(t *testing.T) {
	a, b := newSharedPair("base\n")
	b.MoveCursor(5, false)
	_ = b.InsertSnippet("${1:x}")
	if !b.SnippetActive() {
		t.Fatal("session should be active")
	}
	a.MoveCursor(0, false)
	a.Insert("!")
	if b.SnippetActive() {
		t.Error("a foreign splice must end the session")
	}
}

func TestSnippetNoStopsIsPlainInsert(t *testing.T) {
	e := newTestEditor()
	_ = e.InsertSnippet("plain text")
	if e.SnippetActive() {
		t.Error("no stops → no session")
	}
	if got := e.Text(); got != "plain text" {
		t.Errorf("text = %q", got)
	}
	if e.Cursor != len("plain text") {
		t.Errorf("cursor = %d, want end", e.Cursor)
	}
}

func TestSnippetReplacesSelection(t *testing.T) {
	e := newTestEditor()
	e.SetText("old text")
	e.SelAnchor = 0
	e.Cursor = 3
	_ = e.InsertSnippet("new${1}")
	if got := e.Text(); got != "new text" {
		t.Errorf("text = %q, want %q", got, "new text")
	}
}

func TestSnippetReinsertCancelsOldSession(t *testing.T) {
	e := newTestEditor()
	_ = e.InsertSnippet("${1:first} $2")
	// Re-inserting replaces the selected placeholder (completion
	// accepted mid-snippet) and supersedes the old session.
	_ = e.InsertSnippet("${1:second}")
	if !e.SnippetActive() {
		t.Fatal("new session must be active")
	}
	if got := e.Text(); got != "second " {
		t.Fatalf("text = %q, want %q", got, "second ")
	}
	// Typing edits only the new session's placeholder.
	e.Insert("X")
	if got := e.Text(); got != "X " {
		t.Errorf("text = %q, want %q", got, "X ")
	}
	// Tab past the last stop ends the new session, not the stale one.
	e.HandleEvent(key(consts.KbTab))
	if e.SnippetActive() {
		t.Error("session must end after the only stop")
	}
}

func TestSnippetAtEOF(t *testing.T) {
	e := newTestEditor()
	e.SetText("end")
	e.MoveCursor(3, false)
	if err := e.InsertSnippet("${1:tail}$0"); err != nil {
		t.Fatal(err)
	}
	e.HandleEvent(key(consts.KbTab))
	if e.SnippetActive() {
		t.Error("$0 reached — session must end")
	}
	if e.Cursor != 7 {
		t.Errorf("cursor = %d, want 7 ($0 position)", e.Cursor)
	}
}

func TestSnippetMalformedInsertsNothing(t *testing.T) {
	e := newTestEditor()
	e.SetText("safe")
	if err := e.InsertSnippet("${1:broken"); err == nil {
		t.Fatal("want parse error")
	}
	if got := e.Text(); got != "safe" {
		t.Errorf("buffer mutated on parse error: %q", got)
	}
}

func TestSnippetReadOnlyNoOp(t *testing.T) {
	e := newTestEditor()
	e.SetText("ro")
	e.ReadOnly = true
	if err := e.InsertSnippet("${1:x}"); err != nil {
		t.Fatal(err)
	}
	if got := e.Text(); got != "ro" || e.SnippetActive() {
		t.Errorf("ReadOnly editor mutated: %q", got)
	}
}

func TestSnippetMirrorGrowsOnMultiCharTyping(t *testing.T) {
	e := newTestEditor()
	_ = e.InsertSnippet("$1 := $1")
	// Empty stops: carets only, no selection.
	e.Insert("longIdentifier")
	if got := e.Text(); got != "longIdentifier := longIdentifier" {
		t.Fatalf("mirror growth: %q", got)
	}
	// Backspace trims both.
	e.Backspace()
	if got := e.Text(); got != "longIdentifie := longIdentifie" {
		t.Fatalf("mirror backspace: %q", got)
	}
}
