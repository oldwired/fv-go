// Package stddlg provides standard file / directory selection
// dialogs: FileOpen, FileSave, ChangeDir. Backed by os.ReadDir +
// path/filepath for portability.
//
// The Pascal version (StdDlg.pas) defines a richer family with
// preview panes, sort modes, file-info pages, and a separate
// directory tree dialog. The Go port keeps the public surface
// (one call returns a chosen path, or "" on cancel) and uses a
// simple two-pane layout: a list of files + an input line for the
// current name. A second list shows directories.
package stddlg

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Mode picks the dialog's intent.
type Mode int

const (
	ModeOpen      Mode = iota // pick an existing file to open
	ModeSave                  // pick a file path for saving (may not exist)
	ModeChangeDir             // pick a directory only
)

// Show runs the dialog. Returns the selected path and true on OK,
// "" and false on cancel. startDir is the directory to list initially;
// pattern is a glob used when populating the file list (e.g., "*.txt").
func Show(host *views.Group, mode Mode, title, startDir, pattern string) (string, bool) {
	if startDir == "" {
		startDir, _ = os.Getwd()
	}
	if pattern == "" {
		pattern = "*"
	}
	d := dialogs.NewDialog(geom.NewRect(0, 0, 60, 18), title)

	// Path input.
	pathInput := dialogs.NewInputLine(geom.NewRect(2, 2, 58, 3), 256)
	pathInput.SetText(filepath.Join(startDir, ""))
	pathInput.HistoryID = consts.HiFiles
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 1, 12, 2), "~P~ath:", pathInput))
	d.Insert(pathInput)

	// File list.
	fileScroll := views.NewScrollBar(geom.NewRect(34, 4, 35, 13))
	d.Insert(fileScroll)
	files := dialogs.NewStringListBox(geom.NewRect(2, 4, 34, 13), fileScroll, nil)
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 3, 14, 4), "~F~iles", files))
	d.Insert(files)

	// Dir list. Single click navigates immediately — there's no
	// "select-but-don't-enter" semantic for directories in this dialog.
	dirScroll := views.NewScrollBar(geom.NewRect(58, 4, 59, 13))
	d.Insert(dirScroll)
	dirs := dialogs.NewStringListBox(geom.NewRect(36, 4, 58, 13), dirScroll, nil)
	dirs.SingleClickSelects = true
	d.Insert(dialogs.NewLabel(geom.NewRect(36, 3, 48, 4), "~D~irs", dirs))
	d.Insert(dirs)

	// Repopulate based on the current input path. If pathInput holds a
	// directory, list it; if it holds a file path, list the parent.
	// `filepath.Clean` normalizes any trailing separator (otherwise
	// "/Users/foo/" would survive into cwd and filepath.Dir below
	// would just strip the slash without going up a level — that's
	// the ".." bug).
	cwd := filepath.Clean(startDir)
	repopulate := func() {
		path := pathInput.Text()
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			cwd = filepath.Clean(path)
		} else {
			cwd = filepath.Clean(filepath.Dir(path))
		}
		fns, dns := readDir(cwd, pattern, mode == ModeChangeDir)
		files.SetItems(fns)
		dirs.SetItems(dns)
	}

	// Buttons.
	okText := "~O~pen"
	if mode == ModeSave {
		okText = "~S~ave"
	} else if mode == ModeChangeDir {
		okText = "~C~hange"
	}
	d.Insert(dialogs.NewButton(geom.NewRect(38, 14, 50, 15), okText, consts.CmOK, dialogs.BfDefault))
	d.Insert(dialogs.NewButton(geom.NewRect(2, 14, 14, 15), "Cancel", consts.CmCancel, 0))

	repopulate()

	// Custom command broadcasts: list-item-selected. We translate
	// double-click on a file to OK, double-click on a directory to cd.
	// The dialog's ExecView modal loop already handles cmOK/cmCancel.
	host.Insert(d)
	defer host.Delete(d)
	host.Focus(d)

	q := views.GetEventQueue()
	if q == nil {
		return "", false
	}
	for {
		if pump := views.GetPump(); pump != nil {
			pump()
		}
		ev, ok := q.Get()
		if !ok {
			if wait := views.GetWait(); wait != nil {
				wait()
			}
			continue
		}
		// List interactions:
		//   Double-click / Enter on a file → commit + return.
		//   Double-click / Enter on a dir → navigate into it (".." up).
		// The cmListItemSelected broadcast fires from ListViewer on
		// double-click or Enter; single clicks just move focus.
		if ev.What == consts.EvBroadcast && ev.Command == consts.CmListItemSelected {
			if ev.InfoPtr == files.Self() && files.Focused < len(files.Items) {
				path := filepath.Join(cwd, files.Items[files.Focused])
				pathInput.SetText(path)
				if mode != ModeChangeDir {
					pathInput.Commit()
					return path, true
				}
			}
			if ev.InfoPtr == dirs.Self() && dirs.Focused < len(dirs.Items) {
				name := dirs.Items[dirs.Focused]
				var newDir string
				if name == ".." {
					// cwd is already filepath.Clean'd in repopulate, so
					// Dir reliably returns the parent (no trailing-slash
					// trip-up).
					newDir = filepath.Dir(cwd)
				} else {
					newDir = filepath.Join(cwd, name)
				}
				pathInput.SetText(newDir)
				repopulate()
			}
		}
		if ev.What == consts.EvCommand {
			switch ev.Command {
			case consts.CmOK:
				path := pathInput.Text()
				if mode == ModeChangeDir {
					if info, err := os.Stat(path); err == nil && info.IsDir() {
						pathInput.Commit()
						return path, true
					}
				} else {
					pathInput.Commit()
					return path, true
				}
			case consts.CmCancel:
				return "", false
			}
		}
		d.HandleEvent(&ev)
		views.MarkDirty()
	}
}

// readDir lists files and dirs in dir. Files are filtered by pattern.
// Returned slices are sorted case-insensitively. dirOnly suppresses
// the file list for ChangeDir mode.
func readDir(dir, pattern string, dirOnly bool) (filesOut, dirsOut []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []string{".."}
	}
	dirsOut = []string{".."}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			dirsOut = append(dirsOut, name)
			continue
		}
		if dirOnly {
			continue
		}
		match, _ := filepath.Match(pattern, name)
		if match {
			filesOut = append(filesOut, name)
		}
	}
	sort.Slice(dirsOut, func(i, j int) bool { return less(dirsOut[i], dirsOut[j]) })
	sort.Slice(filesOut, func(i, j int) bool { return less(filesOut[i], filesOut[j]) })
	return
}

func less(a, b string) bool {
	la, lb := []byte(a), []byte(b)
	for i := range la {
		la[i] = lower(la[i])
	}
	for i := range lb {
		lb[i] = lower(lb[i])
	}
	return string(la) < string(lb)
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

// Compile-time placeholder to silence unused-import warnings if the
// drivers import is ever stripped.
var _ = drivers.Event{}
