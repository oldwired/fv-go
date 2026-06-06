# Architecture

`fv-go` is a Go port of Free Vision, the modernized Delphi descendant of
Borland's Turbo Vision. The mental model is Turbo Vision's: an
`Application` owns a tree of `View`s, dispatches events top-down, and
draws bottom-up. The Go translation preserves that model while
adapting to the absence of inheritance and to a real async runtime.

## Layer diagram

```
Application:  app.Application / app.Program / app.Desktop      (pkg/fv/app/)
Widgets:      Dialog, Button, InputLine, ListBox, …            (pkg/fv/dialogs/, pkg/fv/widgets/*)
Views:        View, Group, Window, Frame, ScrollBar, Scroller  (pkg/fv/views/)
Drivers:      Event queue, keyboard, mouse                     (pkg/fv/drivers/)
              Console output, VT/SGR, SIXEL                    (pkg/fv/term/, pkg/fv/sixel/)
Foundation:   Cell types, geom, unicode width, profile         (pkg/fv/types/, pkg/fv/geom/, pkg/fv/unicode/, pkg/fv/utf8/, pkg/fv/profile/)
```

Each layer depends only on the layers below.

## View tree

Pascal `TGroup : TView`, `TWindow : TGroup`, etc. translate to
embedding plus a `self View` back-pointer on `views.Base`. Every
constructor calls `SetSelf(v)`; without it, virtual dispatch in
`Group.HandleEvent` / `Draw` falls through to no-op `Base` methods —
exactly the symptom of "my override is never called."

The invariant is enforced at test time by
`pkg/fv/internal/invarianttests/setself_test.go`.

In-place initializers (`InitGroup`, `InitWindow`, `InitDialog`) exist
alongside the `NewX` constructors. Use `InitX` when embedding by
value in a new type — copying a Group struct after children are
inserted would orphan their `Owner` pointers, a class of bug `*NewX`
trivially avoids.

## Event routing

`Program.Run` reads from `drivers.Queue`, then either intercepts the
event (Quit, Resize, the framework-internal `CmUserCallback`) or
dispatches to `p.HandleEvent` which calls into the view tree's
`Group.HandleEvent`. The Group hands the event to the focused child;
mouse hits walk the children by top-most-first.

`OfPreProcess` / `OfPostProcess` apply to keyboard events only: a
pre-pass over `OfPreProcess` children runs before the focused child,
and a post-pass over `OfPostProcess` children runs after — both for
`EvKeyboard` events only. Mouse dispatch is strictly positional;
there is no global pre/post hook by design (matches Pascal `TGroup`).
A view that needs to intercept mouse events globally can be inserted
as a full-parent-bounds child that leaves `ev.What` untouched for
events it doesn't want to consume.

Postprocess passes catch unconsumed `EvKeyDown` events and look for
status-line shortcut matches.

## Dirty / wake pipeline

The main loop's redraw is driven by a single `atomic.Bool` (`dirty`)
plus a `wake` channel. The contract:

1. Anything that mutates view state (events, anim ticks, async data)
   calls `views.MarkDirty()`.
2. `MarkDirty` sets `p.dirty.Store(true)` and non-blockingly signals
   `p.wake`.
3. The main loop's `waitOne` selects on `backend.Events`, the anim
   timer, and `p.wake`. The wake channel exists so PTY output (a
   goroutine setting `dirty`) doesn't wait for the next keystroke to
   repaint.
4. `idle()` clears `dirty` BEFORE drawing, not after. If a goroutine
   sets `dirty` during the draw, the mark survives to the next idle
   pass. The previous "clear after" order had a race that manifested
   as "terminal echo appears one keystroke late."

When you add async data, follow the same pattern: mutate state under
a lock, release, then `views.MarkDirty()`.

## CallSoon — UI-thread marshaling

Async callbacks from goroutines (Terminal's reader, anim tickers,
host workers) are enqueued on a dedicated `Program.callbacks` channel
via `Program.CallSoon(fn)`. The closures are drained and invoked on
the UI goroutine at the top of `idle()` — so they run not just from
the main `Run` loop but from every modal loop (`ExecView`,
`MenuBox.Run`, popupmenu / fuzzyfinder / stddlg) too, since those all
pump through `idle`. This means host code wired to `Terminal.OnTitle`
/ `OnCWDChange` / `OnActivity` / `OnExit` always runs on the UI
goroutine and can safely touch the view tree.

(A separate `EvCommand + CmUserCallback` event remains supported for
host code that wants to marshal a closure via `PostEvent` from a
background goroutine; `CallSoon` itself no longer uses that path.)

If you write a new async callback, follow the capture-under-lock-
then-schedule pattern:

```go
t.mu.Lock()
cb := t.OnSomething
state := t.something
t.mu.Unlock()
if cb != nil {
    views.CallSoon(func() { cb(state) })
}
```

## SIXEL z-order dance

SIXEL graphics persist in the terminal's pixel grid until overwritten.
We resolve z-order against ordinary cell-buffer views via three
primitives on `RootBackend`:

- `MarkClean(x, y)` — tells the diff "this cell hasn't changed since
  last frame," used to suppress emission of sentinel cells over SIXEL
  pixels.
- `Invalidate(x, y)` — forces a cell to re-emit on the next flush
  regardless of diff equality, used to force covering cells to
  re-paint on top of every fresh SIXEL emit.
- `WriteRaw(s)` — direct stdout write, bypasses the cell buffer.

`ImageView` and `SixelCanvasView` implement `views.PreFlusher`;
`Program.idle()` walks the tree calling `PreFlush` between `Draw`
and `Flush`. They emit a BG fill + SIXEL, then mark their sentinels
clean and any covering cells dirty. `Group.Delete` invalidates the
deleted view's rect so closing a SIXEL view clears its pixels.

## Editor: Buffer / view split

`widgets/editor` separates the text **model** from the **view**:

- `editor.Buffer` owns the bytes, the undo/redo stack (with
  `BeginGroup`/`EndGroup` transactions), `Modified`/`Filename`/encoding,
  and the monotonic change version. Every mutation funnels through one
  internal `splice`, which records undo and notifies listeners.
- `editor.Editor` is a view over a Buffer: caret, selection, secondary
  carets, scroll, folds, snippet session, decorations, colorer. `New`
  creates a private Buffer; `NewShared(…, buf)` attaches a second pane
  to an existing one (split panes: shared text + undo, independent
  view state). Call `Detach` when discarding a shared pane.

**Splice-listener contract.** After each splice the Buffer notifies
every attached editor with a `Splice{Start, OldLen, NewLen, StartLine,
LinesDelta, Origin}`. Listeners remap their byte-anchored state
(carets, bookmarks, fold spans, snippet stops, decorations) through
the `adjustPos`/`adjustSpan` helpers in `span.go`; `Origin` identifies
the acting editor, which manages its own carets and skips that part of
the remap. Version bumps (`bufferChanged`) fire once per user-visible
operation — a grouped multi-caret edit or a group undo is one
`OnChange`, which is what LSP `didChange` consumers want. Host-driven
edits go through `ReplaceRange` (one undo entry, carets remap) rather
than `SetText` (wipes undo, resets all panes).

## Translation notes (Delphi → Go)

- **No inheritance.** Embedding + structural interface satisfaction.
  The `SetSelf(v)` invariant glues virtual dispatch back together.
- **Manual ref-counting is gone.** Parent groups own children via
  slices; rely on GC.
- **`TFVStream` → `io.Reader`/`io.Writer`.** Persistence with type
  tags maps to `encoding/json` + a type registry in `pkg/fv/serial`.
- **Unicode width** is data, not logic — ported verbatim into
  `pkg/fv/unicode`. The width function distinguishes wide / zero-width
  / combining; emoji and CJK rendering depends on it.
- **Windows-only assumptions don't transfer.** `FVScreen.pas` (Windows
  Console API) and `FVClipboard.pas` (Win32 clipboard) are replaced by
  the hand-rolled VT/SGR writer in `pkg/fv/term` and OSC 52 clipboard
  in `pkg/fv/clipboard`. Cross-platform from day one.
- **`set of Char` / `ShortString`** → `map[rune]struct{}` / `string`.
  The editor stores text as UTF-8 bytes already.
