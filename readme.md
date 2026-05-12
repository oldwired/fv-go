# fv-go — Free Vision for Go

A Go port of Free Vision, the Turbo-Vision-derived console UI framework
originally from Free Pascal. Based on the modernized Delphi version at
[oldwired/fv-delphi-modern](https://github.com/oldwired/fv-delphi-modern),
not the original Free Pascal sources — the Delphi tree is included
verbatim under `fv-delphi/` as a porting reference.

Status: full vertical slice up to and including an embedded VT terminal
emulator. macOS / Linux first-class; Windows builds clean
(ConPTY-backed terminal, otherwise feature-parity).

## What's in the box

- **Foundation** — types, term backend (hand-rolled VT100/SGR, alt
  screen, bracketed paste, SGR-1006 mouse, focus events, cell-pixel
  probe), drivers (FV event queue), screen primitives.
- **Views** — `View`, `Group`, `Window` (drag/resize/zoom/close),
  `Frame`, `ScrollBar`, `Scroller`, `ListViewer`, `Background`,
  `SplitGroup`.
- **App** — `Program` / `Application` / `Desktop`, idle loop with
  dirty-tracking, anim ticker, pre-flush hook for SIXEL z-order,
  wake-channel so async data (PTY output) doesn't wait for the next
  keystroke.
- **Menus** — `MenuBar`, `MenuBox` (nested submenus, hotkeys, Alt+letter,
  Left/Right cycling between top-level menus), `StatusLine`.
- **Dialogs** — `Dialog`, `Button`, `InputLine`, `Cluster` (radio /
  check), `ListBox` / `StringListBox`, `Label`, `StaticText`,
  `ParamText`, `History`, `CheckListBox`, `InputLong`.
- **Standard dialogs** — `msgbox`, classic single-pane `stddlg.Show`
  (file/save/changeDir), modern split-pane `stddlg.ShowModern` with
  lazy-loaded directory tree + file-info pane.
- **Validators** — picture / range / filter / lookup, the full TV set.
- **Widget toolkit** — 30+ widgets: ProgressBar, Sparkline, BarChart,
  VUMeter, ToggleSwitch, LEDDigits, ColorTxt, BlinkIndicator, Marquee,
  ToolBar, Tabs, Accordion, Notification, Tooltip, PopupMenu, TimedDlg,
  ComboBox, TreeView (lazy-load via `OnExpand`), AsciiTab, SpinnerView,
  TaskProgress, Breadcrumb, Calendar, ColorSel.
- **Heavy** — Editor (with optional `EditorGutter` for line numbers /
  breakpoints / bookmarks; pluggable syntax highlighting via
  `widgets/syntax`), HexEdit, Grid, MarkdownView, LogViewer,
  FuzzyFinder.
- **System gadgets** — CPUMeter, CPUCoreView, RAMView, DiskUsageView,
  BatteryView, NetworkView, ProcessView, UptimeView. All
  bring-your-own-data via a `Sampler` callback so the framework stays
  portable.
- **Graphics** — `pkg/fv/sixel` realtime (216-cube) + quality
  (median-cut + Floyd-Steinberg) encoders, cell-pixel-size probe via
  `CSI 16t`, `ImageView` (PNG/JPEG/GIF/BMP/TIFF/WebP) and
  `SixelCanvasView` (programmable pixel canvas with line/rect/fill
  primitives). Half-block fallback on terminals without SIXEL.
- **Terminal emulator** — `widgets/terminal`. PTY-backed shell via
  `creack/pty` on Unix and ConPTY on Windows. VT100 + xterm parser
  covering plain text + UTF-8, the C0 set, CSI cursor/erase/SGR/scroll,
  DEC private modes (autowrap, cursor vis, alt screen), OSC 0/1/2
  titles. 2000-line scrollback with `/`-search; drag-to-select +
  clipboard copy; mouse forwarding when the inner program asks for it.

## Quickstart

```go
package main

import (
    "github.com/oldwired/fv-go/pkg/fv/app"
    "github.com/oldwired/fv-go/pkg/fv/consts"
    "github.com/oldwired/fv-go/pkg/fv/dialogs"
    "github.com/oldwired/fv-go/pkg/fv/drivers"
    "github.com/oldwired/fv-go/pkg/fv/geom"
    "github.com/oldwired/fv-go/pkg/fv/menus"
    "github.com/oldwired/fv-go/pkg/fv/views"
)

func main() {
    a, err := app.NewApplication()
    if err != nil {
        panic(err)
    }
    defer a.Done()

    cols := a.BaseView().Size.X
    a.SetMenuBar(menus.NewMenuBar(geom.NewRect(0, 0, cols, 1),
        menus.NewMenu(
            &menus.Item{Name: "~F~ile", Sub: menus.NewMenu(
                &menus.Item{Name: "~Q~uit", Command: consts.CmQuitApp},
            )},
        )))

    a.OnCommand = func(cmd uint16, ev *drivers.Event) bool {
        // Custom commands land here.
        return false
    }

    // Open a window with a button. NewWindow's third arg is the
    // number badge in the frame title — 0 means no badge.
    win := views.NewWindow(geom.NewRect(5, 3, 40, 10), "Hello", 0)
    win.Insert(dialogs.NewButton(geom.NewRect(2, 2, 16, 3), "~C~lick me",
        consts.CmOK, dialogs.BfDefault))
    a.Desktop.InsertWindow(win)

    a.Run()
}
```

See `docs/QUICKSTART.md` for a deeper tour and `cmd/fvdemo/` for a
fully-loaded demo exercising every widget.

## Running the demo

```bash
go run ./cmd/fvdemo
```

The demo opens with a menu bar; explore via `Test → Widgets` /
`Test → Apps` for the widget showcases. On a SIXEL-capable terminal
(iTerm2, WezTerm, kitty, Windows Terminal ≥ 1.22), `Apps → Image View
(Open...)` and `Apps → Sixel Canvas` render true pixel graphics.

## Architecture

```
Application:  app.Application / app.Program / app.Desktop      (app/)
Widgets:      Dialog, Button, InputLine, ListBox, …            (dialogs/, widgets/*)
Views:        View, Group, Window, Frame, ScrollBar, Scroller  (views/)
Drivers:      Event queue, keyboard, mouse                     (drivers/)
              Console output, VT/SGR, SIXEL                    (term/, sixel/)
Foundation:   Cell types, geom, unicode width, profile         (types/, geom/, unicode/, utf8/, profile/)
```

The layering matches the Delphi reference: each layer only depends on
the ones below it. The four cross-cutting interfaces from Pascal
(`IFVDrawable`, `IFVEventHandler`, `IFVDataAware`, `ISerializable`)
become Go interfaces satisfied structurally by `views.View`.

## Pascal → Go translation notes

These are the non-obvious mappings the original `PORTING.md` won't give
you:

- **No inheritance.** Pascal's `TGroup : TView`, `TWindow : TGroup`,
  etc. are rewritten with embedding + interface satisfaction. Virtual
  dispatch goes through a `self View` back-pointer set by `SetSelf`
  during construction.
- **In-place initialization.** `InitGroup`, `InitWindow`, `InitDialog`
  exist alongside the `NewX` constructors to avoid the struct-copy bug
  that orphans child `Owner` pointers — initialize via these whenever
  you embed a `Group` / `Window` / `Dialog` by value.
- **Manual ref-counting is gone.** Parents own children via slices;
  the GC handles cleanup. No `_AddRef`/`_Release` plumbing.
- **`TFVStream` → `io.Reader`/`io.Writer`** for streaming;
  persistence-with-type-tags maps cleanly to `encoding/json` + a type
  registry (`pkg/fv/serial`).
- **Unicode width** (`FVUnicodeWidth.pas`) is data, not logic — ported
  verbatim into `pkg/fv/unicode`. Emoji and CJK rendering depends on
  the wide / zero-width / combining classification.
- **Windows-only assumptions don't transfer.** `FVScreen.pas` (Windows
  Console API) and `FVClipboard.pas` (Win32 clipboard) are replaced by
  the hand-rolled VT/SGR writer in `pkg/fv/term` and OSC 52 clipboard
  in `pkg/fv/clipboard`. Cross-platform from day one.

## License

Inherits the Free Vision / Free Pascal license — see `COPYING.FPC`
and `COPYING.txt`. Original-author copyright headers are preserved
inside each ported unit.
