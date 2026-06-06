# CLAUDE.md

This file orients agent assistants working in the repo. The durable
engineering documentation lives in `docs/`:

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — layering, view tree,
  event routing, dirty/wake pipeline, `CallSoon` UI-thread marshaling,
  SIXEL z-order, Delphi→Go translation notes.
- [`docs/TERMINAL.md`](docs/TERMINAL.md) — PTY/ConPTY, VT parser, OSC
  handling, callbacks, bracketed paste, scrollback, Windows specifics.
- [`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md) — build / test / CI
  commands, conventions, the `SetSelf` invariant, in-place
  initializers, the capture-under-lock-then-schedule pattern,
  `ResetForTest()` helpers.

## Project overview

`fv-go` is a Go port of **Free Vision**, a console-mode UI framework
derived from Borland's Turbo Vision. The port is based on the
modernized Delphi version (https://github.com/oldwired/fv-delphi-modern),
not the original Free Pascal sources.

**Status**: Vertical slice complete and working. Foundation, view
tree, dialogs, the full Turbo-Vision widget toolkit (30+ widgets),
heavy widgets (editor, hex editor, grid), syntax-highlighted editor
with gutter, fuzzy finder, markdown view, log viewer, system gadgets,
SIXEL graphics + ImageView/SixelCanvasView, and a working embedded
VT/xterm terminal emulator (PTY on Unix, ConPTY on Windows). The
`cmd/fvdemo` binary exercises every widget.

What's NOT done: Pascal's `Outline.pas` (superseded by
`widgets/treeview`), `Statuses.pas` (covered by `widgets/spinner` +
`widgets/taskprogress`), and the SIXEL "quality" path's BMP/TIFF/WebP
loaders predate `golang.org/x/image` so we just register those
decoders. Beyond that, the port matches the Pascal feature surface —
plus IDE-grade editor extensions with no Pascal counterpart: a
Buffer/view split with shared-buffer split panes, `ReplaceRange` +
LSP position helpers, multi-cursor/column selection, code folding,
LSP snippet tab stops, transient decorations, a multi-line
`hoverpopup`, and TreeView context menus (see
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md), "Editor: Buffer /
view split"; the fvdemo "Editor Lab" exercises all of it).

## Agent guidance (non-code, non-architecture)

- **Default to writing no comments.** Let identifiers carry intent.
  Add a comment only when the *why* is non-obvious (a hidden
  constraint, a subtle invariant, a workaround for a specific bug).
- **Don't restate the task in a comment.** Comments that say "added
  for the X flow" or "used by Y" belong in PR descriptions, not in
  the source, and rot as the codebase evolves.
- **Reference implementation:** the original Delphi sources live upstream
  at https://github.com/oldwired/fv-delphi-modern. Browse the relevant
  `.pas` there before porting or modifying a unit.
- **Don't run `cmd/fvdemo` non-interactively** as part of automation
  — it expects a real TTY.

## License

Inherits the Free Vision / Free Pascal license — see
[`LICENSE`](LICENSE) (LGPL-2.1) and
[`FPC-EXCEPTION.txt`](FPC-EXCEPTION.txt) (linking exception).
Preserve license headers when translating unit-by-unit.
