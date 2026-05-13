package dialogs_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

var update = flag.Bool("update", false, "rewrite golden files instead of comparing")

// compareGolden reads or writes testdata/<name>.golden depending on the
// -update flag. Tests that exercise a new widget should run with
// `go test -run TestGolden -update` once, eyeball the captured output,
// then commit the file.
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

// TestGoldenButton confirms the default-button render is stable.
// Uses the headless backend so this runs in any CI environment.
func TestGoldenButton(t *testing.T) {
	h := term.NewHeadless(16, 3)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	btn := dialogs.NewButton(geom.NewRect(0, 0, 12, 2), "~O~k", 0, dialogs.BfDefault)
	btn.State |= consts.SfExposed | consts.SfVisible
	btn.Draw()
	_ = h.Flush()

	compareGolden(t, "button_default", h.Snapshot())
}
