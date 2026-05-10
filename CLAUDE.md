# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`fv-go` is a Go port of **Free Vision**, a console-mode UI framework derived from Borland's Turbo Vision. The port is based on the modernized Delphi version (https://github.com/oldwired/fv-delphi-modern), not the original Free Pascal sources.

**Status**: Pre-source-code. No Go code has been written yet. The repository currently contains only the Delphi reference implementation under `fv-delphi/` and licensing files. New work means establishing the initial Go module layout and translating units from Delphi.

## Reference Implementation

The complete Delphi reference is in `fv-delphi/`. **Read these before porting any unit:**

- `fv-delphi/src/` — Delphi/Pascal sources to translate (one `.pas` file per unit; ~70 units total)
- `fv-delphi/CLAUDE.md` — Delphi-side conventions, OBJECT→CLASS conversion rules, and known pitfalls
- `fv-delphi/ARCHITECTURE.md` — Layer diagram, class hierarchy, interface contracts
- `fv-delphi/PORTING.md` — Pascal-to-modern-Delphi porting playbook (much of it transfers conceptually to Go)
- `fv-delphi/README.md` — Feature inventory, SIXEL/clipboard/terminal-emulator details

## Architecture (carry over from Delphi)

The Delphi version uses a strict bottom-up layering. The Go port should preserve this layering — it makes the dependency graph acyclic and matches how Turbo Vision is conventionally taught.

```
Application:  TApplication / TProgram / TDesktop          (App.pas)
Widgets:      TDialog, TButton, TInputLine, TListBox …    (Dialogs.pas, Menus.pas, Editors.pas, …)
Views:        TView, TGroup, TWindow, TFrame              (Views.pas)
Drivers:      Event queue, keyboard, mouse                (Drivers.pas)
              Console output, VT/SGR, Sixel               (FVScreen.pas)
Foundation:   Streams, Unicode width, UTF-8, profile      (Objects.pas, FVUTF8.pas, FVUnicodeWidth.pas, FVProfile.pas)
```

Four cross-cutting interfaces (in Delphi's `FVInterfaces.pas`) — drawing, event handling, data binding, JSON serialization — are implemented by `TView` and propagate to all widgets. In Go these become small interfaces (`Drawable`, `EventHandler`, `DataAware`, `Serializable`) that view types satisfy structurally.

## Translation Notes (Delphi → Go)

These are the non-obvious mappings that the Delphi `PORTING.md` will not give you:

- **No inheritance.** Delphi's `TGroup : TView`, `TWindow : TGroup`, etc. must be rewritten with embedding + interface satisfaction. The Turbo-Vision-style "override `Draw`/`HandleEvent`" pattern needs explicit virtual-dispatch wiring (a method table on the view, or an interface field the parent calls into).
- **Manual ref-counting is gone.** Delphi disables `_AddRef`/`_Release` on `TView` so groups own their children. In Go, parent groups own child views via slices; rely on GC.
- **`TFVStream` → `io.Reader`/`io.Writer`** for the streaming layer, but persistence-with-type-tags (the `ISerializable` JSON path) maps cleanly to `encoding/json` + a type registry.
- **Unicode width** (`FVUnicodeWidth.pas`) is data, not logic — port the tables verbatim. The width function must distinguish wide / zero-width / combining; emoji and CJK rendering depends on it.
- **Windows-only assumptions in the Delphi version do not transfer.** `FVScreen.pas` uses the Windows Console API and `FVClipboard.pas` uses Win32 clipboard. The Go port should target a portable terminal backend (e.g. `tcell` or a hand-rolled VT/SGR writer that already exists conceptually in the Delphi `FVProfile`/`FVScreen` split). Decide this before porting `FVScreen`.
- **`ConPTY`-based `Terminal.pas`** is Windows-specific and should be deferred or made platform-gated.
- **`set of Char` and `ShortString`** — replace with `map[rune]struct{}` / `string`. Note Delphi's editor stores text as UTF-8 bytes already, so the editor port can keep `[]byte` buffers.

## Build & Test

No Go module exists yet. The first non-trivial change should `go mod init` (likely `github.com/<owner>/fv-go` — confirm with the user) and add a minimal foundation-layer package before anything depends on it.

Once a module exists, standard Go tooling applies:

```bash
go build ./...
go test ./...
go test -run TestName ./path/to/pkg   # single test
go vet ./...
```

There is no equivalent of the Delphi `FVTest.exe` interactive harness yet; building one (a Go program that exercises each ported widget) is the natural integration-test surface, mirroring the Delphi side.

## License

Inherits the Free Vision / Free Pascal license — see `COPYING.FPC` and `COPYING.txt`. Preserve license headers when translating unit-by-unit.
