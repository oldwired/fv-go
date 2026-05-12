# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`fv-go` is a Go port of **Free Vision**, a console-mode UI framework derived from Borland's Turbo Vision. The port is based on the modernized Delphi version (https://github.com/oldwired/fv-delphi-modern), not the original Free Pascal sources.

**Status**: Vertical slice is complete and working. Foundation, view tree, dialogs, the full Turbo-Vision widget toolkit (30+ widgets), heavy widgets (editor, hex editor, grid), syntax-highlighted editor with gutter, fuzzy finder, markdown view, log viewer, system gadgets, SIXEL graphics + ImageView/SixelCanvasView, and a working embedded VT/xterm terminal emulator (PTY on Unix, ConPTY on Windows). The `cmd/fvdemo` binary exercises every widget.

What's NOT done: Pascal's `Outline.pas` (superseded by `widgets/treeview`), `Statuses.pas` (covered by `widgets/spinner` + `widgets/taskprogress`), and the SIXEL "quality" path's BMP/TIFF/WebP loaders predate `golang.org/x/image` so we just register those decoders. Beyond that, the port matches the Pascal feature surface.

## Reference Implementation

The Delphi reference lives in `fv-delphi/`. **Read the relevant `.pas` before porting or modifying a unit:**

- `fv-delphi/src/` — Delphi/Pascal sources (one `.pas` file per unit; ~70 units total).
- `fv-delphi/CLAUDE.md` — Delphi-side conventions, OBJECT→CLASS conversion rules, known pitfalls.
- `fv-delphi/ARCHITECTURE.md` — Layer diagram, class hierarchy, interface contracts.
- `fv-delphi/PORTING.md` — Pascal-to-modern-Delphi playbook (much of it transfers conceptually to Go).
- `fv-delphi/README.md` — Feature inventory, SIXEL/clipboard/terminal-emulator details.

## Architecture

Bottom-up layering, mirroring the Delphi reference. Each layer depends only on the ones below it.

```
Application:  app.Application / app.Program / app.Desktop      (pkg/fv/app/)
Widgets:      Dialog, Button, InputLine, ListBox, …            (pkg/fv/dialogs/, pkg/fv/widgets/*)
Views:        View, Group, Window, Frame, ScrollBar, Scroller  (pkg/fv/views/)
Drivers:      Event queue, keyboard, mouse                     (pkg/fv/drivers/)
              Console output, VT/SGR, SIXEL                    (pkg/fv/term/, pkg/fv/sixel/)
Foundation:   Cell types, geom, unicode width, profile         (pkg/fv/types/, pkg/fv/geom/, pkg/fv/unicode/, pkg/fv/utf8/, pkg/fv/profile/)
```

The four cross-cutting interfaces from Pascal (`IFVDrawable`, `IFVEventHandler`, `IFVDataAware`, `ISerializable`) become Go interfaces satisfied structurally by `views.View`.

## Build & Test

Module path: `github.com/oldwired/fv-go`.

```bash
go build ./...
go test ./...
go test -run TestName ./pkg/...           # single test
go vet ./...

GOOS=linux   go build ./...               # cross-compile sanity
GOOS=windows go build ./...
```

`cmd/fvdemo` is the integration-test surface — its `Test → Widgets`
and `Test → Apps` menus visibly exercise every widget. **Don't run it
non-interactively as part of automation** — it expects a real TTY.

## Translation Notes (Delphi → Go)

These are the non-obvious mappings the upstream `PORTING.md` won't cover:

- **No inheritance.** Pascal's `TGroup : TView`, `TWindow : TGroup`, etc. are rewritten with embedding + interface satisfaction. The Turbo-Vision-style "override `Draw`/`HandleEvent`" pattern needs explicit virtual-dispatch wiring: every view stores a `self View` back-pointer that the framework calls into. **`SetSelf(v)` MUST be called in every constructor** — without it, your override is never called.
- **In-place initializers.** `views.InitGroup`, `views.InitWindow`, `dialogs.InitDialog` exist alongside the `NewX` constructors to avoid the struct-copy bug that orphans child `Owner` pointers. If you're embedding `Group` / `Window` / `Dialog` by value in a new type, initialize via `InitX` from inside your constructor — never via `*NewX(...)`.
- **Manual ref-counting is gone.** Delphi disables `_AddRef`/`_Release` on `TView` so groups own their children. In Go, parent groups own child views via slices; rely on GC.
- **`TFVStream` → `io.Reader`/`io.Writer`** for streaming. Persistence-with-type-tags (the `ISerializable` JSON path) maps cleanly to `encoding/json` + a type registry in `pkg/fv/serial`.
- **Unicode width** (`FVUnicodeWidth.pas`) is data, not logic — ported verbatim into `pkg/fv/unicode`. The width function distinguishes wide / zero-width / combining; emoji and CJK rendering depends on it.
- **Windows-only assumptions don't transfer.** `FVScreen.pas` (Windows Console API) and `FVClipboard.pas` (Win32 clipboard) are replaced by the hand-rolled VT/SGR writer in `pkg/fv/term` and OSC 52 clipboard in `pkg/fv/clipboard`. Cross-platform from day one.
- **`set of Char` and `ShortString`** — replace with `map[rune]struct{}` / `string`. The editor stores text as UTF-8 bytes already, so the editor port can keep `[]byte` buffers.

## The dirty / wake pipeline

The main loop's redraw is driven by a single `dirty` bool plus a `wake` channel. The contract is:

1. Anything that mutates view state (events, anim ticks, async data) calls `views.MarkDirty()`.
2. `MarkDirty` sets `p.dirty = true` and non-blockingly signals `p.wake`.
3. The main loop's `waitOne` selects on `backend.Events`, the anim timer, and `p.wake`. The wake channel exists so PTY output (a goroutine setting `dirty`) doesn't wait for the next keystroke to repaint.
4. `idle()` clears `dirty` BEFORE drawing, not after — if a goroutine sets `dirty` during the draw, the mark survives to the next idle pass. The previous "clear after" order had a race that manifested as "terminal echo appears one keystroke late."

When you add async data, follow the same pattern: mutate state under a lock, release, then `views.MarkDirty()`.

## The SIXEL z-order dance

SIXEL graphics persist in the terminal's pixel grid until overwritten. We resolve z-order against ordinary cell-buffer views via three primitives in `RootBackend`:

- `MarkClean(x, y)` — tells the diff "this cell hasn't changed since last frame," used to suppress emission of sentinel cells over SIXEL pixels.
- `Invalidate(x, y)` — forces a cell to re-emit on the next flush regardless of diff equality, used to force covering cells to re-paint on top of every fresh SIXEL emit.
- `WriteRaw(s)` — direct stdout write, bypasses the cell buffer.

`ImageView` and `SixelCanvasView` implement `views.PreFlusher`; `Program.idle()` walks the tree calling `PreFlush` between `Draw` and `Flush`. They emit a BG fill + SIXEL, then mark their sentinels clean and any covering cells dirty. `Group.Delete` invalidates the deleted view's rect so closing a SIXEL view clears its pixels.

## Conventions

- **Comments**: explain non-obvious *why* (subtle invariant, workaround for a known bug, deliberate trade-off). Don't restate what well-named identifiers already say.
- **No `T` prefix**. Pascal `TButton` → Go `dialogs.Button`. Package path is the namespace.
- **`GetTypeID()`** on every view. Required by the serialization registry. New widgets should add a unique string.
- **Constructor pattern**: `NewX(bounds, …) *X` for normal use; `InitX(&x.Base, bounds, …)` for embed-by-value cases.
- **Modal loops** (`MenuBox.runIn`, `FuzzyFinder.Run`, `ExecView`) MUST call `views.MarkDirty()` after each event so async / nav state changes get drawn.

## Build & runtime gotchas to remember

- The cell-pixel-size probe (`CSI 16t`) runs at backend init and may take up to 200ms on slow terminals. macOS Terminal doesn't respond — that's fine, defaults kick in.
- SIXEL detection is a heuristic via `TERM_PROGRAM` / `WT_SESSION` / `TERM`; users can force with `FV_SIXEL=1` / `FV_SIXEL=0`.
- Cell pixel size override: `FV_CELL_W=12 FV_CELL_H=24`.
- Terminal emulator on Windows requires Windows Terminal (or any console host that supports `EXTENDED_STARTUPINFO_PRESENT` + ConPTY). Legacy ConHost won't work.

## Out-of-scope

- A high-level `tea`/`bubbletea`-style declarative API. fv-go is the imperative TV-style framework; building a declarative layer on top is a separate project.
- Direct mapping to `tcell` / `termbox-go`. The terminal backend is hand-rolled to keep the dependency surface small and the VT pipeline explicit.

## License

Inherits the Free Vision / Free Pascal license — see `COPYING.FPC` and `COPYING.txt`. Preserve license headers when translating unit-by-unit.
