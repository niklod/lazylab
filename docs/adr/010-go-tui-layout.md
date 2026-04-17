# 010: Go TUI Layout, Keybindings, and Headless Testing

## Decision

Phase G2 task 1 lands `internal/tui/` with three files carrying discrete concerns:

1. `layout.go` — pure rectangle math plus `layout(g)` that `SetManagerFunc`
   feeds to gocui every tick. Three panes mirror the Python split:
   - `repos`  (top-left, 40% × 50%)
   - `mrs`    (bottom-left, 40% × 50%)
   - `detail` (right, 60% × 100%)
2. `keys.go` — a single `bindings` table; `Bind(g)` iterates it once before
   `MainLoop`. No per-view gating required: verified against upstream source
   that `jesseduffield/gocui.SetKeybinding` does not validate `viewname`
   against the view table at registration time; match is checked on dispatch.
3. `focus.go` — pure `cycle(order, current, delta)` plus two handlers
   (`focusNext`, `focusPrev`) that call `SetCurrentView`. Isolating the cycle
   arithmetic keeps focus tests independent of a `*gocui.Gui`.

`tui.Run(ctx, *AppContext)` checks `term.IsTerminal(stdout)` up-front and
returns `ErrRequiresTTY` (sentinel) when false. `cli.Run` wraps and surfaces
it, giving subprocess tests a deterministic non-zero exit path with a matchable
error string.

Integration tests use `gocui.NewGui(NewGuiOpts{Headless: true, Width, Height})`
— the fork's simulation-screen mode. This avoids a PTY dependency while still
driving real gocui machinery (view creation, `SetCurrentView`, focus
transitions).

## Context

Python's `LazyLabMainScreen` (`lazylab/ui/screens/primary.py`) uses Textual's
flexbox-style docking to build a `SelectionsPane` (left 40%, repos over mrs)
beside a `SelectionDetailsPane` (right 60%). `MainViewPane` owns the section
focus cycle on `h`/`l`, and the `SelectionDetailsContainer` owns tab nav on
`[`/`]`. Keybindings are distributed across widgets via Textual's `BINDINGS`
attribute.

gocui has no flexbox; layout is imperative rectangle math per tick. That
works in our favour — the layout is small enough to hold in one `paneRects`
function and test at the arithmetic level.

The harder question was testing. gocui does not ship a documented PTY harness.
Three candidates were considered:

- **PTY subprocess** via `creack/pty`: realistic but adds a dep and couples
  tests to a running binary.
- **Debug env var + stderr grep**: production hack that exists only for tests.
- **`NewGuiOpts{Headless: true}` in-process**: real gocui against a tcell
  simulation screen; no new dep, no production hack.

Option 3 wins. It substantively satisfies the DoD ("e2e drives keys and
asserts focused view name") by driving `focusNext`/`focusPrev` and reading
`g.CurrentView().Name()` in the same process.

## Consequences

**Positive:**
- Per-view handler stubs are registered today and swap to real behaviour as
  widgets arrive in G2-task-2 (repos) and G4 (detail tabs) — no binding-table
  churn.
- No new test deps; the headless tcell simulation already ships with gocui.
- Binding table is a single slice in one file; reviews of "what keys are
  bound where" never need to cross files.
- `errors.Is(err, gocui.ErrUnknownView)` does not work — `go-errors/errors
  v1.0.2` (gocui's transitive dep) does not implement `Unwrap()`. We use
  `goerrors.Is()` from `github.com/go-errors/errors` instead.

**Known follow-ups (do not block this PR):**
- Global `q`, `h`, `l` vs search-mode: gocui already guards global bindings
  against editable views (see `gui.go:1553` — global bindings only fire when
  the focused view is not editable or the key is a control key). When
  G2-task-2 wires `/` to an editable search input, this should Just Work.
  Verify in G2-task-2; do not pre-emptively add guard logic here.
- `j`, `k`, `g`, `G`, `/` handlers are no-ops in G2-task-1. Real behaviour
  lands when the repos/mrs tables (G2-task-2) and detail tabs (G4) replace
  the empty panes.
- No global `CacheRefreshed` event (ADR 009 stands) — per-view polling is
  the prescribed path if a view needs live refresh in later phases.

## Alternatives considered

- **Per-view binding registration gated on `ErrUnknownView` inside layout()**:
  unnecessary given upstream's lazy matching. Added complexity for no benefit.
- **`lazygit`-style integration test runner**: overkill for a 3-pane smoke
  scenario. Revisit if the test matrix grows past a handful of scenarios.
- **PTY subprocess e2e**: revisit if/when we need to test full TTY rendering
  (colors, cursor, actual terminal state). The current headless path does
  not cover rendering fidelity — it only covers state transitions.
