package views_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

var update = flag.Bool("update", false, "rewrite golden files instead of comparing")

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", path, err)
	}
	if string(want) != got {
		t.Errorf("snapshot mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", name, got, string(want))
	}
}

// TestGoldenWindow confirms the framed Window draws its border and
// title. SfShadow is off so the rendered region is purely the window.
func TestGoldenWindow(t *testing.T) {
	h := term.NewHeadless(20, 6)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	w := views.NewWindow(geom.NewRect(0, 0, 18, 5), "Demo", 1)
	w.State |= consts.SfExposed | consts.SfVisible
	w.State &^= consts.SfShadow
	w.Draw()
	_ = h.Flush()

	compareGolden(t, "window_basic", h.Snapshot())
}
