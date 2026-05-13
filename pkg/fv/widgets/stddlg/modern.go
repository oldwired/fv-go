package stddlg

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"
)

// ShowModern is the split-pane variant of Show. Layout (60×22 typical):
//
//	┌─ Title ──────────────────────────────────────────────────────┐
//	│ Name: [____________________________________________________] │
//	│ ┌──────────────┐  ┌──────────────────────────────────────┐  │
//	│ │ Directories  │  │ Files                                │  │
//	│ │ ▾ /          │  │ a.txt                                │  │
//	│ │   ▸ home     │  │ b.png                                │  │
//	│ │   ▸ etc      │  │ c.go                                 │  │
//	│ └──────────────┘  └──────────────────────────────────────┘  │
//	│ ┌─ Info ───────────────────────────────────────────────────┐ │
//	│ │ b.png   45.2 KB   2026-04-30 14:22                        │ │
//	│ └────────────────────────────────────────────────────────────┘ │
//	│              [ Open ]  [ Cancel ]                              │
//	└────────────────────────────────────────────────────────────────┘
//
// Tree on the left is lazy: only the visible nodes' children are read
// from disk. mode picks the OK button caption (Open / Save / Choose).
// pattern is a filepath.Match glob applied to the file list — pass "*"
// to show everything.
func ShowModern(host *views.Group, mode Mode, title, startDir, pattern string) (string, bool) {
	if startDir == "" {
		startDir, _ = os.Getwd()
	}
	if pattern == "" {
		pattern = "*"
	}

	d := dialogs.NewDialog(geom.NewRect(0, 0, 76, 24), title)
	w, h := d.Size.X, d.Size.Y

	// Name input across the top.
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 1, 8, 2), "~N~ame:", nil))
	nameIn := dialogs.NewInputLine(geom.NewRect(8, 1, w-2, 2), 1024)
	d.Insert(nameIn)
	nameIn.SetText(startDir + string(filepath.Separator))

	// Build the directory tree starting from a reasonable root (the
	// drive on Windows, "/" elsewhere) and pre-load the path leading
	// to startDir so the user lands somewhere familiar.
	rootPath := filepath.VolumeName(startDir)
	if rootPath == "" {
		rootPath = "/"
	} else {
		rootPath += string(filepath.Separator)
	}
	root := &treeview.Node{
		Label:       rootPath,
		Data:        rootPath,
		HasChildren: true,
	}
	tree := treeview.New(geom.NewRect(2, 3, 28, h-7), []*treeview.Node{root})
	tree.OnExpand = func(n *Node) { populateDirNode(n) }
	d.Insert(tree)

	// File list on the right, with its own scrollbar.
	fileScroll := views.NewScrollBar(geom.NewRect(w-3, 3, w-2, h-7))
	d.Insert(fileScroll)
	fileList := dialogs.NewStringListBox(geom.NewRect(28, 3, w-3, h-7), fileScroll, nil)
	d.Insert(fileList)

	// Info pane below.
	info := newInfoPane(geom.NewRect(2, h-7, w-2, h-4))
	d.Insert(info)

	// Buttons.
	okText := "~O~pen"
	if mode == ModeSave {
		okText = "~S~ave"
	} else if mode == ModeChangeDir {
		okText = "~C~hoose"
	}
	d.Insert(dialogs.NewButton(geom.NewRect(w-26, h-3, w-14, h-2), okText, consts.CmOK, dialogs.BfDefault))
	d.Insert(dialogs.NewButton(geom.NewRect(w-12, h-3, w-2, h-2), "Cancel", consts.CmCancel, 0))

	// Track current directory and the in-flight file list. setDir
	// re-reads dir and rewires the file list + info pane.
	current := struct{ dir string }{dir: ""}
	repopulate := func(dir string) {
		current.dir = dir
		names := readFiles(dir, pattern, mode == ModeChangeDir)
		fileList.SetItems(names)
		fileList.Focused = 0
		info.set("", 0, time.Time{})
		if len(names) > 0 {
			path := filepath.Join(dir, names[0])
			st, err := os.Stat(path)
			if err == nil {
				info.set(names[0], st.Size(), st.ModTime())
			}
		}
		nameIn.SetText(dir + string(filepath.Separator))
	}

	// Expand the path leading to startDir so the user sees their cwd
	// highlighted. Best-effort — silently does nothing on unreadable
	// directories.
	populateDirNode(root)
	root.Expanded = true
	expandToPath(tree, root, startDir)
	repopulate(startDir)

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
		if ev.What == consts.EvBroadcast && ev.Command == consts.CmListItemSelected {
			// Dir tree: navigate. File list: select + commit (open) or
			// just update info (save).
			if ev.InfoPtr == fileList.Self() {
				if fileList.Focused < len(fileList.Items) {
					name := fileList.Items[fileList.Focused]
					path := filepath.Join(current.dir, name)
					nameIn.SetText(path)
					if st, err := os.Stat(path); err == nil {
						info.set(name, st.Size(), st.ModTime())
					}
					if mode != ModeChangeDir {
						// Double-click / Enter commits.
						nameIn.Commit()
						return path, true
					}
				}
			}
		}
		if ev.What == consts.EvCommand {
			switch ev.Command {
			case consts.CmOK:
				path := nameIn.Text()
				if mode == ModeChangeDir {
					if st, err := os.Stat(path); err == nil && st.IsDir() {
						return path, true
					}
				} else {
					return path, true
				}
			case consts.CmCancel:
				return "", false
			}
		}
		d.HandleEvent(&ev)
		views.MarkDirty()

		// Selection-driven file-list refresh: if the user moved focus
		// on the tree, repopulate. We poll after dispatch since the
		// tree doesn't broadcast its focus changes.
		if cur := tree.CurrentNode(); cur != nil {
			if dir, ok := cur.Data.(string); ok && dir != current.dir {
				repopulate(dir)
			}
		}
		// Same for the file list: a single click moves Focused but
		// doesn't fire CmListItemSelected, so we sync nameIn with the
		// list's current selection. Without this, Open would submit
		// the directory path the repopulate() set instead of the
		// file the user clicked.
		if fileList.Focused >= 0 && fileList.Focused < len(fileList.Items) {
			name := fileList.Items[fileList.Focused]
			want := filepath.Join(current.dir, name)
			if nameIn.Text() != want {
				nameIn.SetText(want)
				if st, err := os.Stat(want); err == nil {
					info.set(name, st.Size(), st.ModTime())
				}
			}
		}
	}
}

// Node is a re-export so callers don't have to import treeview just
// to write an OnExpand callback.
type Node = treeview.Node

// populateDirNode reads node.Data (a directory path) and replaces
// its Children with one node per subdirectory. Each child is marked
// HasChildren so it renders as expandable even before its own
// contents are read — Toggle's OnExpand re-enters here for that child.
func populateDirNode(n *Node) {
	dir, ok := n.Data.(string)
	if !ok {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		n.HasChildren = false
		return
	}
	var subdirs []os.DirEntry
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			subdirs = append(subdirs, e)
		}
	}
	sort.Slice(subdirs, func(i, j int) bool {
		return less(subdirs[i].Name(), subdirs[j].Name())
	})
	children := make([]*Node, 0, len(subdirs))
	for _, e := range subdirs {
		path := filepath.Join(dir, e.Name())
		children = append(children, &Node{
			Label:       e.Name(),
			Data:        path,
			HasChildren: true, // probed lazily on the user's first expand
		})
	}
	n.Children = children
	n.HasChildren = len(children) > 0
}

// expandToPath walks the tree from root expanding nodes along the
// path, lazy-loading as it goes. The final node lands focused.
func expandToPath(t *treeview.TreeView, root *Node, target string) {
	target = filepath.Clean(target)
	cur := root
	curPath, _ := cur.Data.(string)
	for {
		curPath = filepath.Clean(curPath)
		if curPath == target {
			break
		}
		rel, err := filepath.Rel(curPath, target)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			break
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) == 0 {
			break
		}
		next := parts[0]
		if !cur.Expanded {
			if t.OnExpand != nil {
				t.OnExpand(cur)
			}
			cur.Expanded = true
		}
		var nextNode *Node
		for _, c := range cur.Children {
			if c.Label == next {
				nextNode = c
				break
			}
		}
		if nextNode == nil {
			break
		}
		cur = nextNode
		curPath = filepath.Join(curPath, next)
	}
	// Position focus at the deepest expanded node.
	rebuildAndFocus(t, cur)
}

// rebuildAndFocus calls a no-op event into the tree (it has no public
// "rebuild and focus") so we can position cursor at node n.
func rebuildAndFocus(t *treeview.TreeView, target *Node) {
	// Walk the tree's internal flattening by re-toggling root's
	// expanded state if needed; simplest is to use the public
	// SetRoots which rebuilds.
	t.SetRoots(t.Roots)
	// Now scan visible rows for our target. TreeView doesn't expose
	// the flat list, so we approximate: send Down events until
	// CurrentNode matches. Bounded by len(visible).
	for i := 0; i < 4096; i++ {
		if t.CurrentNode() == target {
			return
		}
		ev := drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbDown}
		prev := t.CurrentNode()
		t.HandleEvent(&ev)
		if t.CurrentNode() == prev {
			return // hit the end without finding it
		}
	}
}

// readFiles lists files matching pattern in dir (sorted, case-insensitive,
// hidden files excluded). When dirOnly is true, returns nothing — the
// caller in ModeChangeDir doesn't show a file list.
func readFiles(dir, pattern string, dirOnly bool) []string {
	if dirOnly {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || e.IsDir() {
			continue
		}
		if matched, _ := filepath.Match(pattern, name); !matched {
			continue
		}
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}

// infoPane is the file-metadata strip at the bottom of the dialog.
// Composes one line of "<name>   <human size>   <modtime>" inside a
// framed rectangle, redrawn whenever set() updates the fields.
type infoPane struct {
	views.Base
	name string
	size int64
	mod  time.Time
}

func newInfoPane(bounds geom.Rect) *infoPane {
	ip := &infoPane{Base: views.NewBase(bounds)}
	ip.SetSelf(ip)
	return ip
}

func (ip *infoPane) GetTypeID() string { return "infopane" }

func (ip *infoPane) set(name string, size int64, mod time.Time) {
	ip.name, ip.size, ip.mod = name, size, mod
	views.MarkDirty()
}

func (ip *infoPane) Draw() {
	w, h := ip.Size.X, ip.Size.Y
	pal := theme.Get()
	frame := pal.DialogBackground
	text := pal.DialogBackground
	for y := 0; y < h; y++ {
		buf := screen.MakeDrawBuffer(w)
		for x := 0; x < w; x++ {
			screen.DrawCell(buf, x, " ", frame)
		}
		// top border
		if y == 0 {
			screen.DrawCell(buf, 0, "┌", frame)
			for x := 1; x < w-1; x++ {
				screen.DrawCell(buf, x, "─", frame)
			}
			screen.DrawCell(buf, w-1, "┐", frame)
			screen.DrawStr(buf, 2, " Info ", frame)
		} else if y == h-1 {
			screen.DrawCell(buf, 0, "└", frame)
			for x := 1; x < w-1; x++ {
				screen.DrawCell(buf, x, "─", frame)
			}
			screen.DrawCell(buf, w-1, "┘", frame)
		} else {
			screen.DrawCell(buf, 0, "│", frame)
			screen.DrawCell(buf, w-1, "│", frame)
			if y == 1 && ip.name != "" {
				line := fmt.Sprintf(" %s   %s   %s",
					truncate(ip.name, w-40),
					humanSize(ip.size),
					ip.mod.Format("2006-01-02 15:04"))
				screen.DrawStr(buf, 2, line, text)
			}
		}
		ip.WriteLine(0, y, w, 1, buf)
	}
}

func truncate(s string, max int) string {
	if max < 4 {
		max = 4
	}
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	}
}
