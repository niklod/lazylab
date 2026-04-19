# ADR 017: Pipeline tab design parity

## Status

Accepted — follow-up to ADR 014 (Pipeline tab MVP) once the tab was
measured against `design/project/wireframes/pipeline.js` and found to
drift in icons, colours, chrome, and interaction surface.

## Context

Phase G4 shipped the Pipeline tab with functional parity (stages list +
inline job log) but not visual parity:

- Status icons were wrong: running as `▶` warn (design wants `●` ok),
  skipped dim (design: warn), manual info (design: dim), canceled dim
  `✗` (design: err `⊘`).
- Job indent 2 spaces (design: 3).
- Overall summary line, refresh meta, keybind strip — all missing.
- Log pane composed the job header inside the scrollable body instead
  of the pane chrome, putting a non-content line at origin 0 that every
  scroll gesture dragged around.
- Selection `SelFgColor` set to `gocui.ColorBlack`; design palette
  specifies white (`theme.ColorSelectionFg`).
- No auto-refresh — running pipelines went stale until the user bounced
  tabs; finished pipelines were never re-read.
- No action keys — users couldn't retry jobs, open in browser, copy log
  body, or pause polling.

The wireframe specifies a richer interface: right-aligned meta on each
pane (`Pipeline · #91422 · 4m 12s   ↻ auto 5s · updated 2s ago`), an
overall-summary line beneath the stages list (`Overall ● failed ·
triggered by @mira.k · commit a3f7b2e · 14m ago`), a keybind strip at
the bottom of each pane, and a footer line on the log pane (`(end of
log · 1342 lines · press y to copy · Esc to close)`).

## Decision

### Icons and colours mapped to theme tokens

`pipelineJobStatusIcon` rewritten to reference `theme.Fg*` directly so
the call site reads design palette roles, not ANSI codes. Full mapping
in `internal/tui/views/pipeline_stages.go:pipelineJobStatusIcon`.

Canceled renders as `⊘` in `FgErr` — the wireframe specifies err colour
but we keep the `⊘` glyph (rather than reusing `✗`) so the eye can
distinguish a user-cancelled job from a hard-failed one even though
both share the err tone.

### Overall summary line is re-computed on each render, not cached

The "Overall ● failed · triggered by … · 14m ago" fragment has a
`Relative(CreatedAt, time.Now())` component that must tick forward. We
inject `pipelineStagesNow = time.Now` as a package-level indirection so
tests freeze the clock without having to thread a clock through every
renderer, and rebuild the line during each `Render` call rather than
storing it in `rows`. `TriggeredBy *models.User` is new on
`models.Pipeline`; `toDomainPipeline` maps from `gogitlab.Pipeline.User`.

### Pane meta lives on the first body line

gocui's `View.Title` has no right-alignment — same constraint ADR 016
hit. The stages and log panes render the chrome as their first line:

```
 Pipeline · #91422 · 4m 12s                 ↻ auto 5s · updated 2s ago
```

`DetailView.applyPipelineChromeLocked(now)` recomputes the title/meta
pair under `d.mu` and pushes it into the child widget via `SetChrome`.
`renderChromeLine(title, meta, innerW)` handles the left/right layout
with a graceful narrow-pane fallback (two-space join) so a resize to
near-zero never produces a mangled buffer.

### Auto-refresh: two goroutines, one cancel

One controller per Pipeline-tab session. The data ticker fires at 5s
while the pipeline is in-flight, 30s once terminal — the `applyPipelineRefresh`
hook uses a non-blocking `select` on `dataReset` so interval switches
propagate without restarting the timer. The UI ticker fires every 1s
and only issues `g.Update(noop)` so the `updated Ns ago` fragment ticks
forward between network calls. Both goroutines share one
`context.Context`; `stopPipelineRefreshLocked` cancels it on tab leave,
MR change, or app shutdown. `ToggleAutoRefresh` flips a flag rather
than teardown/restart the goroutines — re-enable is instant.

### Stale-while-revalidate refresh

Background refresh must not blank the pane while waiting for the next
fetch. `invalidateAndRefetchPipeline` calls `Invalidate(mr_pipeline,
pid, iid)` followed by `GetMRPipelineDetail`, then applies the result
through `applyPipelineRefresh` — which updates the widget in place
without going through `ShowLoading`. A failed background refresh is
swallowed (no user-facing error) so a transient network blip doesn't
clobber stale data the user is still reading.

### Action keys: r / R / o / a / y

- `r` retry — uses `models.PipelineStatus.IsRetryable()` to skip the
  RPC when GitLab would 409. Cache is invalidated on success so the
  next background tick picks up the re-queued state.
- `R` manual refresh — same invalidate + refetch path as the data
  ticker, but triggered by the user.
- `o` open in browser — `internal/pkg/browser` wraps `pkg/browser` with
  a swappable hook so tests can assert the URL without spawning a
  browser.
- `a` toggle auto-refresh.
- `y` copy log body — `internal/pkg/clipboard` fronts `atotto/clipboard`
  with a consumer-owned interface (`Clipboard`) and a `Fake` backend
  for tests. Copy strips SGR so the clipboard gets plain text.

### Log pane splits chrome from body

Before: `SetJob` prepended `renderJobLogHeader(job)` into the body so
every j/k scroll dragged the header around. After: the header lives on
the pane chrome (`Log · ✗ e2e-smoke`), the meta line lives on the chrome
right (`stage test · 2m 03s · job #77431   ↻ paused — job finished`),
the body is just the sanitized trace, and `renderJobLogFooter(N)`
appears below it.

## Consequences

Positive:

- Visual and interactive parity with the design bundle.
- Auto-refresh eliminates the "stale pipeline" footgun without
  flickering the pane.
- Retry / copy / browser actions cover the common workflows users
  previously had to switch to the GitLab web UI for.
- Chrome pattern (`SetChrome`) is reusable — Diff and Overview tabs
  can adopt the same title/meta split when their design parity passes
  come around.

Trade-offs:

- +2 goroutines per active Pipeline tab. Cancellation covered by unit
  test (`TestStopPipelineRefreshCancelsGoroutines`); the UI-tick cost
  is one `g.Update` per second, well below the render budget.
- `pkg/browser` and `atotto/clipboard` are new runtime deps. See ADR
  018.
- `models.Pipeline.TriggeredBy` changes the on-disk cache layout —
  `mr_pipeline` namespace was added to `InvalidateMR` and the cache
  bust ran once.

## Alternatives considered

- **Render chrome inside `pv.Title`.** Rejected — gocui has no Subtitle
  and no right-alignment; would need a monkey-patch. Body-first-line
  matches ADR 016.
- **Single ticker that handles both data and UI ticks.** Rejected —
  would couple the 1s redraw cadence to the 5s/30s network cadence,
  meaning either too-frequent fetches or stuttery clock updates.
- **OSC52 for clipboard.** Rejected — less reliable cross-terminal than
  atotto. Revisit if users report OSC52 would be preferable.
- **Keep canceled glyph `✗`.** Rejected — would collide visually with
  `failed`. `⊘` in err colour retains both signal and differentiation.
