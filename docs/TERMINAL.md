# Terminal widget

`pkg/fv/widgets/terminal` is a full-featured embedded VT/xterm
emulator: PTY on Unix, ConPTY on Windows. The widget owns:

- A `parser` driving a `buffer` (cells + scrollback) — `vt.go`.
- A `pty` handle with platform-specific spawn paths
  (`pty_unix.go`, `pty_windows.go`).
- Mouse / scroll / search / selection / copy-mode UI on top.

## Lifecycle

1. `terminal.New(bounds)` constructs the widget.
2. Caller wires callbacks (`OnTitle`, `OnCWDChange`, `OnActivity`,
   `OnExit`, `OnBell`, `OnFeed`) **before** calling `Start`.
3. `Start(name, args, env)` spawns the child, launches the reader
   and waiter goroutines, and begins parsing.
4. `Stop` is called by `Application.Done` (via the `Stoppable`
   interface walk) and tears down the child + reader.

`OnPanic` in `Program` calls `Application.Done` on its way out, so
PTY children get SIGHUP even on a crash.

## Callbacks fire on the UI goroutine

The reader and waiter goroutines do not invoke host callbacks
directly. They use the capture-under-lock-then-schedule pattern:
read `t.OnTitle` (etc.) under `t.mu`, release the lock, then
`views.CallSoon(cb)`. The main loop dispatches `cb` on the UI
goroutine. Host code wired to these callbacks can safely touch
view state.

`OnFeed` is the one exception — it runs synchronously between PTY
read and parser feed because hosts use it for asciicast-style
byte-level recording. Keep `OnFeed` cheap.

## VT parser

`vt.go` is a state machine consuming bytes from the PTY:

- C0 control codes, CR/LF/BS, tabs.
- CSI (`ESC [ … final`) — cursor movement, SGR, mode toggles,
  scroll regions, mouse mode (1000/1002/1006), bracketed paste
  (2004), focus events (1004).
- DCS (`ESC P … ST`) — SIXEL parsing is here.
- OSC (`ESC ] … BEL or ST`) — title (0/1/2), CWD (7), and hyperlinks
  (8). All payloads pass through `sanitizeOSCString` before reaching
  callbacks: C0 stripped, length capped, UTF-8 truncation rune-safe.
- ESC-prefix Alt sequences and SS3 function keys.

Malformed sequences fall back to "discard until next ESC" rather
than aborting — the philosophy of terminal emulators is "be liberal
in what you accept."

## Bracketed paste

When the host terminal is in bracketed-paste mode (`ESC[?2004h`),
the reader collects bytes between `ESC[200~` and `ESC[201~` into a
single paste payload. There is a 4 MiB cap (`maxPasteBytes`); a
runaway paste hits the cap, emits an `EventPaste` with
`Truncated=true`, and the truncation flag is carried into
`drivers.Event.InfoByte` as `consts.PasteTruncated`. Hosts that
care can observe it; everyone else keeps reading `InfoPtr.(string)`.

## Scrollback

The buffer keeps a configurable number of off-screen rows (default
2000; set `Terminal.ScrollbackLines` before `Start`). Scrollback
content is searchable, selectable, and copyable. Selection feeds
into OSC 52 clipboard (`clipboard.SetText`) which is rate-limited
by `clipboard.Policy` (100 KB default, configurable, can be
disabled).

## Windows specifics

Spawning uses `CreateProcessW` + `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE`.
The child is placed in a `JobObject` with `KILL_ON_JOB_CLOSE` so
abrupt parent termination (crash, debugger Ctrl-C) cleans up the
descendant tree.

The earlier env-block code joined `KEY=VALUE` entries with NULs and
called `syscall.UTF16FromString`, which silently truncated at the
first NUL — so only the first env var made it through. The fix
returns `[]uint16` directly via `utf16.Encode`. See
`pty_windows.go:utf16EnvBlock`.

Command-line quoting uses the full `CommandLineToArgvW` inverse, so
trailing-backslash paths like `C:\Program Files\Foo\` round-trip
correctly. See `pty_windows.go:quoteWindowsArg`.
