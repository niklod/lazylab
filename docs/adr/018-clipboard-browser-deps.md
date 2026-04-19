# ADR 018: clipboard and browser dependencies

## Status

Accepted — introduced alongside ADR 017 (Pipeline design parity) to
support `y copy log body` and `o open in browser` keybindings.

## Context

The Pipeline design parity pass (ADR 017) added two actions that
interact with the user's desktop environment:

- `y` — copy the sanitized job-trace body to the clipboard.
- `o` — open the job's GitLab web URL in the default browser.

The Go rewrite had neither capability. Implementations options:

1. Shell out. `os/exec.Command("open", url)` (darwin) /
   `xdg-open` (linux) / `rundll32 url.dll,FileProtocolHandler` (windows).
   For clipboard: `pbcopy` (darwin) / `xclip` (linux) / `clip`
   (windows).
2. Call a cross-platform library.
3. Emit an OSC52 sequence for clipboard; open URLs via shell.

## Decision

Pull two well-maintained libraries:

- `github.com/pkg/browser` — tiny wrapper around the same per-OS
  commands, handled with platform build-tags so the binary carries only
  what it needs.
- `github.com/atotto/clipboard` — thin layer over `pbcopy` / `xclip` /
  Windows API with a uniform `WriteAll` / `ReadAll` surface.

Both are wrapped behind project-local packages so the views layer
depends on a consumer-owned abstraction, not the third-party surface:

- `internal/pkg/browser` exports `Open(url)` and `SetOpenFunc(fn)` for
  tests.
- `internal/pkg/clipboard` exports a `Clipboard` interface (single
  `WriteAll(text) error` method), a `System()` factory, and a `Fake`
  implementation for tests.

## Consequences

- Two new module deps. Both are small, have been in maintenance mode
  for years, and have no transitive requirements of note.
- Tests never touch the real clipboard or spawn a real browser —
  `DetailView.SetClipboard(fake)` and `browser.SetOpenFunc(fn)` are the
  single injection points.
- `atotto/clipboard` returns an error on headless/remote-SSH hosts.
  The Pipeline tab surfaces that via the transient status slot
  (`"copy failed: <err>"`); callers decide how to handle it.

## Alternatives considered

- **Roll our own shellout.** ~25 LOC per capability, but every new OS
  quirk (Windows paths, WSL, WSL2, remote X11, Wayland) becomes ours
  to fix. The libraries already absorb that.
- **OSC52 escape for clipboard.** Works over SSH, but relies on
  terminal support that is unevenly shipped and often silently ignored
  when missing. Users would be confused by `y` that returns no error
  but doesn't actually copy. Revisit if atotto breaks for a common
  workflow.
- **No clipboard, just print the path to a temp file.** Rejected —
  wireframe specifies the clipboard workflow and it's the universal
  terminal-tool expectation.
