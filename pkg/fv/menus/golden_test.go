package menus_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/menus"
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

// TestGoldenStatusLineLeftRight exercises the new RightItems slot
// added by C2 to ensure right-justified rendering stays put.
func TestGoldenStatusLineLeftRight(t *testing.T) {
	h := term.NewHeadless(40, 1)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	def := &menus.StatusDef{
		LeftItems: []*menus.StatusItem{
			{Text: "~F1~ Help"},
			{Text: "~F10~ Menu"},
		},
		RightItems: []*menus.StatusItem{
			{Text: "READY"},
		},
	}
	sl := menus.NewStatusLine(geom.NewRect(0, 0, 40, 1), nil)
	sl.Defs = def
	sl.State |= consts.SfExposed | consts.SfVisible
	sl.Draw()
	_ = h.Flush()

	compareGolden(t, "statusline_lr", h.Snapshot())
}
