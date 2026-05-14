# Contributing

## Build & test

Module path: `github.com/oldwired/fv-go`.

```bash
go build ./...
go test ./...
go test -race ./pkg/fv/...
go test -run TestName ./pkg/...    # single test
go vet ./...

GOOS=linux   go build ./...        # cross-compile sanity
GOOS=windows go build ./...
GOOS=darwin  go build ./...
```

`cmd/fvdemo` is the integration-test surface — its `Test → Widgets`
and `Test → Apps` menus visibly exercise every widget. Don't run it
non-interactively as part of automation — it expects a real TTY.

CI lives in `.github/workflows/ci.yml`. It runs build + vet + test on
ubuntu / macos / windows, and `go test -race ./pkg/fv/...` on Linux.

## Conventions

- **Comments**: explain non-obvious *why* (subtle invariant, workaround
  for a known bug, deliberate trade-off). Don't restate what well-named
  identifiers already say.
- **No `T` prefix.** Pascal `TButton` → Go `dialogs.Button`. The
  package path is the namespace.
- **`GetTypeID()` on every view.** Required by the serialization
  registry. New widgets should add a unique string.
- **Constructor pattern**: `NewX(bounds, …) *X` for normal use;
  `InitX(&x.Base, bounds, …)` for embed-by-value cases.
- **Modal loops** (`MenuBox.runIn`, `FuzzyFinder.Run`, `ExecView`)
  MUST call `views.MarkDirty()` after each event so async / nav state
  changes get drawn.

## The `SetSelf` invariant

Every view constructor must call `SetSelf(v)` before returning, so
virtual dispatch in `Group.HandleEvent` / `Draw` reaches your
overrides. Forgetting it is silent — the override just doesn't fire.

Enforced by `pkg/fv/internal/invarianttests/setself_test.go`. If you
add a new widget, append a case to that file.

## In-place initializers

If your widget embeds `Group` / `Window` / `Dialog` *by value* in a
new type, initialize via `InitGroup` / `InitWindow` / `InitDialog`
from inside the constructor — not via `*NewX(...)` and a copy. The
copy step orphans children's `Owner` pointers.

## Async data → MarkDirty

Anything that mutates view state from a goroutine (PTY readers,
animation tickers, network handlers) must call `views.MarkDirty()`
after the mutation so the main loop knows to repaint. If you also
need the callback to *run* on the UI goroutine (most do), use
`views.CallSoon(fn)` instead.

The capture-under-lock-then-schedule pattern:

```go
state.Lock()
cb := state.OnSomething
local := state.field
state.Unlock()
if cb != nil {
    views.CallSoon(func() { cb(local) })
}
```

Never call `views.CallSoon` while holding a state lock that the
scheduled callback might re-acquire.

## Runtime hooks

The framework wires several process-global hooks (event queue,
animation registry, theme, clipboard). Tests that need a clean
runtime should call the `ResetForTest()` helper in the respective
package:

```go
import "github.com/oldwired/fv-go/pkg/fv/views"

func TestSomething(t *testing.T) {
    views.ResetForTest()
    // ...
}
```

Available in: `views`, `anim`, `theme`, `clipboard`.

## Reference implementation

The Delphi reference lives in `fv-delphi/`. Read the relevant `.pas`
before porting or modifying a unit:

- `fv-delphi/src/` — Delphi/Pascal sources.
- `fv-delphi/CLAUDE.md` — Delphi-side conventions.
- `fv-delphi/ARCHITECTURE.md` — class hierarchy, interface contracts.
- `fv-delphi/PORTING.md` — Pascal-to-modern-Delphi playbook.

For the Go-side adaptation strategy, see [docs/ARCHITECTURE.md](ARCHITECTURE.md).
For terminal specifics see [docs/TERMINAL.md](TERMINAL.md).
