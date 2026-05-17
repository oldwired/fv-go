# fv-go Quickstart

A 10-minute tour of the framework. Assumes you've seen Turbo Vision
once, or are willing to skim
[the Wikipedia page](https://en.wikipedia.org/wiki/Turbo_Vision).

## Mental model

An fv-go application is a tree of **views**. The root is a
`*app.Program` (the embedded variant `*app.Application` adds backend
lifecycle). Standard structure:

```
Program (Group)
├── MenuBar
├── StatusLine
└── Desktop (Group)
    └── Window (Group)
        ├── Frame
        ├── ScrollBar
        └── content view (e.g. an Editor, Terminal, ImageView)
```

Each view has `Origin`, `Size`, `State` (visible / focused / cursor /
disabled / …), `Options` (selectable / pre-process / post-process /
…), and an `Owner` pointer to its parent. Drawing is cell-based: every
view's `Draw()` method writes into a per-view buffer that's eventually
flushed to the terminal in one diffed batch.

Events flow:

```
terminal → reader → backend.Events → main loop's queue → dispatched
```

Dispatch order inside a `Group`:

1. **Pre-process** children get first crack at keys (this is how
   `MenuBar` catches F10 / Alt+letter before anything else).
2. **Focused** child gets the event.
3. **Post-process** children get a chance.
4. **Mouse** events are routed by position (back-to-front).

A view consumes an event by calling `ClearEvent(ev)` (which sets
`ev.What = EvNothing`); siblings stop seeing it.

## Hello, fv-go

```go
package main

import (
    "github.com/oldwired/fv-go/pkg/fv/app"
    "github.com/oldwired/fv-go/pkg/fv/consts"
    "github.com/oldwired/fv-go/pkg/fv/dialogs"
    "github.com/oldwired/fv-go/pkg/fv/drivers"
    "github.com/oldwired/fv-go/pkg/fv/geom"
    "github.com/oldwired/fv-go/pkg/fv/menus"
)

const cmGreet uint16 = 1000

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
                &menus.Item{Name: "~G~reet…", Command: cmGreet},
                menus.Separator(),
                &menus.Item{Name: "~Q~uit", Command: consts.CmQuitApp},
            )},
        )))

    a.OnCommand = func(cmd uint16, ev *drivers.Event) bool {
        if cmd == cmGreet {
            d := dialogs.NewDialog(geom.NewRect(0, 0, 30, 7), "Hi")
            d.Insert(dialogs.NewStaticText(geom.NewRect(2, 2, 28, 3), "Hello, world"))
            d.Insert(dialogs.NewButton(geom.NewRect(10, 4, 20, 5), "O~K~", consts.CmOK, dialogs.BfDefault))
            a.Desktop.ExecView(d)
            return true
        }
        return false
    }

    a.Run()
}
```

Things to notice:

- `~F~ile` — tilde marks the hotkey letter. `Alt+F` opens the menu.
- `menus.Separator()` — a non-selectable horizontal rule.
- `consts.CmQuitApp` — built-in commands live in `consts`. Custom
  commands start at any uint16 above the built-in range; 1000+ is
  conventional.
- `a.OnCommand` — the program-wide command handler. Returning `true`
  marks the command consumed. Most apps dispatch via a `switch cmd`.
- `dialogs.NewDialog` / `Desktop.ExecView` — modal dialog. ExecView
  runs its own event loop until the dialog ends (CmOK / CmCancel etc.).

## Coordinate systems

Rectangles are half-open: `geom.NewRect(x1, y1, x2, y2)` covers cells
`[x1, x2) × [y1, y2)`. A 30-wide dialog is `(0, 0, 30, …)`, not
`(0, 0, 29, …)`. Sizes use `Size.X` / `Size.Y` (= `x2-x1`, `y2-y1`).

Origins are local to the parent — a button at `(10, 4)` inside a
dialog whose origin is `(5, 3)` lives at screen cells `(15, 7)`.

## Building a custom view

Embed `views.Base`, implement `Draw()`, optionally `HandleEvent()`:

```go
type Clock struct {
    views.Base
    Time time.Time
}

func NewClock(bounds geom.Rect) *Clock {
    c := &Clock{Base: views.NewBase(bounds)}
    c.SetSelf(c) // crucial — wires the back-pointer for virtual dispatch
    return c
}

func (c *Clock) GetTypeID() string { return "clock" }

func (c *Clock) Draw() {
    buf := screen.MakeDrawBuffer(c.Size.X)
    text := c.Time.Format("15:04:05")
    for x := 0; x < c.Size.X; x++ {
        screen.DrawCell(buf, x, " ", types.MakeAttr(0x0F, 0x01))
    }
    screen.DrawStr(buf, 0, text, types.MakeAttr(0x0F, 0x01))
    c.WriteLine(0, 0, c.Size.X, 1, buf)
}

func (c *Clock) Tick(now time.Time) (redraw bool) {
    c.Time = now
    return true
}
```

The Clock above expects a 1-row bound — extend the `WriteLine` loop
over `c.Size.Y` if you want to fill a taller rect.

`SetSelf(c)` is the most important line — without it, polymorphic
dispatch (Group calling your `Draw`/`HandleEvent`) goes through the
embedded `Base`'s method, not yours. You'll see "the view doesn't
draw anything" and waste an hour.

For animation, register with the `anim` package:

```go
anim.Register(c, time.Second)
```

`anim.Pulse()` runs in `Program.idle()` and calls your view's `Tick`
method when due. Return `true` to mark the program dirty so the view
repaints.

## Modal dialogs

`Desktop.ExecView(dialog)` is the workhorse. The dialog gets inserted
into the desktop, focus moves to it, and ExecView's loop pumps events
until the dialog calls `EndModal(cmd)` (which the OK/Cancel buttons
do automatically). The return value is the command that ended the
loop — `CmOK` / `CmCancel` / `CmYes` / `CmNo`.

For modeless windows (the kind you can stack and switch between), use
`Desktop.InsertWindow(win)` instead. `Desktop.InsertWindowPassive(win)`
is the same but does NOT take keyboard focus — use it for decorative
overlays (mascots, status widgets, watchlists) that should sit on top
of the wallpaper without interrupting whichever window the user is
typing in.

## Standard file dialog

```go
path, ok := stddlg.ShowModern(&a.Desktop.Group,
    stddlg.ModeOpen, "Open File", "" /* cwd */, "*" /* pattern */)
if !ok {
    return // cancelled
}
```

`ShowModern` is the split-pane variant (directory tree + file list +
info pane). The single-pane `Show` is also available if you prefer.

## Patterns from the demo

The demo (`cmd/fvdemo/`) is the practical reference:

- `main.go` — application bootstrap, menu construction.
- `widgets.go` — one `showXxx` function per widget category.
- `extensions.go` — the heavyweights (Editor, Grid, HexEdit, Terminal,
  ImageView, etc.).
- `apps.go` — the "Open File" path and a small editor application.

Patterns to lift:

- **Per-feature command IDs** declared in a `const (… iota)` block
  starting above 200.
- **`dispatchWidget` / `dispatchApp` / `dispatchExtension`** —
  category-split command dispatch, each returning `bool` so the main
  `OnCommand` can chain them.
- **`OnTitle` / `OnExit` callbacks** on views to plumb async events
  back into the UI (e.g., the Terminal updating its window's caption
  from the inner shell's OSC sequences).

## What's where

| Need to do this                       | Look at                                  |
|---------------------------------------|------------------------------------------|
| Draw cells                            | `pkg/fv/screen` + `pkg/fv/types`         |
| Build a menu                          | `pkg/fv/menus`                           |
| Get text input                        | `pkg/fv/dialogs.InputLine`               |
| Show a modal yes/no                   | `pkg/fv/msgbox`                          |
| Pick a file                           | `pkg/fv/widgets/stddlg.ShowModern`       |
| Display an image                      | `pkg/fv/widgets/imageview`               |
| Run a shell-in-a-window               | `pkg/fv/widgets/terminal`                |
| Show a tree                           | `pkg/fv/widgets/treeview`                |
| Animate something                     | `pkg/fv/anim` + a `Tick` method          |
| Copy / paste                          | `pkg/fv/clipboard`                       |
| Probe terminal capabilities           | `pkg/fv/profile` + `pkg/fv/sixel.IsSupported` |

## Common gotchas

- **`SetSelf(self)` after construction**. Without it, your `Draw`,
  `HandleEvent`, and `GetTypeID` aren't called by the framework.
- **In-place vs by-value init**. If you embed a `Group` / `Window` /
  `Dialog` by value in your own type, use `views.InitGroup` /
  `InitWindow` / `dialogs.InitDialog` from inside your constructor;
  the `NewX` constructors return *new* values whose `Owner` field
  isn't safe to copy.
- **`ClearEvent` consumes the event**. Set `ev.What = EvNothing`
  yourself if you want to suppress an event without claiming it from
  this view.
- **Don't call `Draw` directly**. The framework owns the redraw
  pipeline — `MarkDirty()` schedules a redraw; the next idle pass
  walks the tree.
- **Goroutine work needs `views.MarkDirty()` to wake the UI**. The
  main loop has a `wake` channel; `MarkDirty` signals it. Async data
  (terminal output, downloaded JSON, etc.) that mutates view state
  must call `MarkDirty` for the screen to refresh.

## Where to go from here

- Read `cmd/fvdemo/main.go` end-to-end (200 lines, dense but
  illustrative).
- For Pascal porters: the source-of-truth reference is upstream at
  [oldwired/fv-delphi-modern](https://github.com/oldwired/fv-delphi-modern).
  Most Go files have a `// Ported from <Foo>.pas` comment at the top
  pointing to the matching unit.
- For widget authors: pick a small existing widget like
  `widgets/spinner` and use it as a template.
