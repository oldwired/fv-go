package sixel

import (
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

// IsSupported reports whether the host terminal renders SIXEL.
//
// Detection options, in priority order:
//
//  1. The FV_SIXEL env var: "1"/"true"/"yes" forces on, "0"/"false"/
//     "no" forces off. This is the explicit user override and takes
//     precedence over heuristics.
//  2. TERM_PROGRAM heuristic: iTerm.app, WezTerm, mintty, mlterm — all
//     known to render SIXEL.
//  3. WT_SESSION env var: Windows Terminal (≥ v1.22 supports SIXEL).
//     The variable is set by Windows Terminal itself; legacy ConHost
//     leaves it unset.
//  4. TERM contains "mlterm".
//  5. Default: false. We refuse to render SIXEL on terminals where we
//     can't verify support, because emitting a DCS to a non-SIXEL term
//     leaks raw bytes onto the screen.
//
// (A proper protocol probe — DA1 / "CSI c" with parsing for "4" in the
// response — is the right long-term answer and would require reader-
// side support for routing replies back here. Deferred.)
func IsSupported() bool {
	if v := os.Getenv("FV_SIXEL"); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm", "mintty":
		return true
	}
	if os.Getenv("WT_SESSION") != "" {
		return true
	}
	if strings.Contains(os.Getenv("TERM"), "mlterm") {
		return true
	}
	return false
}

// cellPxW/cellPxH hold the detected cell pixel size. They're atomic
// because the term backend may update them from its signal goroutine
// (on a font-size-changing resize) while render code reads them.
var (
	cellPxW atomic.Int32
	cellPxH atomic.Int32
)

func init() {
	cellPxW.Store(9)
	cellPxH.Store(18)
}

// SetCellSize records the terminal's actual character-cell pixel
// dimensions. Called by the term backend when it learns the cell size
// from TIOCGWINSZ pixel fields or a CSI 16t reply. Env-var overrides
// (FV_CELL_W / FV_CELL_H) in CellSize still take precedence.
func SetCellSize(w, h int) {
	if w >= 4 {
		cellPxW.Store(int32(w))
	}
	if h >= 4 {
		cellPxH.Store(int32(h))
	}
}

// CellSize returns the pixel dimensions of one terminal cell.
//
// Source priority:
//
//  1. FV_CELL_W and FV_CELL_H env vars (both must be present and ≥4).
//  2. Whatever the term backend learned from CSI 16t at startup.
//  3. Default 9×18 — a workable compromise for most fonts at common
//     sizes when neither override nor probe gave us anything.
func CellSize() (w, h int) {
	w, h = int(cellPxW.Load()), int(cellPxH.Load())
	if envW, errW := strconv.Atoi(os.Getenv("FV_CELL_W")); errW == nil && envW >= 4 {
		w = envW
	}
	if envH, errH := strconv.Atoi(os.Getenv("FV_CELL_H")); errH == nil && envH >= 4 {
		h = envH
	}
	return
}
