# ADR 015: Centralised design-palette theme (`internal/tui/theme`)

## Status

Accepted — implemented alongside Phase G4 polish, after `design/` handoff landed.

## Context

Until this change the Go TUI scattered colour choices across views: raw
`\x1b[31m..\x1b[36m` 16-colour ANSI escapes inside `detail.go`, hard-coded
`gocui.ColorGreen` for panel-frame focus and selection backgrounds in every
list view, and duplicated status→colour switches in `pipeline_stages.go`
and the Overview-tab status rows. The `design/` wireframes landed with a
concrete palette (ok / warn / err / info / accent / merged / draft / dim)
and explicit hex values — none of which the code used.

Three pain points:

- **No single source of truth.** Changing "accent" meant touching ~7 files.
- **Wrong palette.** The 16-colour basic ANSI (`\x1b[32m`) did not match
  the design swatches; any reviewer comparing the running UI against the
  wireframe saw a colour mismatch.
- **Semantic drift.** `ansiGreen` was used for "success", "approved", and
  "added diff line" with no shared vocabulary, so anyone editing the UI
  had to inspect adjacent context to know what the colour meant.

## Decision

Introduce a single package `internal/tui/theme` that exposes the design
palette in two flavours:

1. **ANSI SGR strings** (`FgOK`, `FgWarn`, `FgErr`, `FgInfo`, `FgAccent`,
   `FgMerged`, `FgDraft`) for inline text colouring inside pane buffers.
   All values are 24-bit truecolor escapes (`\x1b[38;2;R;G;Bm`), the RGB
   triples taken verbatim from `design/project/wireframes/_helpers.js`.
2. **`gocui.Attribute` values** (`ColorAccent`) for tcell-side colouring
   (panel frames, selection background) via `gocui.NewRGBColor`.

The `gocui.Gui` is already built with `OutputMode: gocui.OutputTrue`
(`internal/tui/app.go:30`), which means `gocui` parses `\x1b[38;2;…m` out
of the cell buffer and forwards it through tcell. tcell itself downgrades
24-bit values to 256/16-colour when the terminal advertises anything less,
so no manual fallback is required on our side.

Two small helpers live alongside the palette:

- `Wrap(color, text)` — `color + text + Reset`.
- `Dot(color)` — coloured filled-circle glyph (`●`) used by the Overview
  "State" and "Pipeline" rows.

A sibling `theme/timeago.go` exposes `Relative(t, now time.Time) string`
("14 minutes ago", "3 days ago", "just now", …). The now parameter is
explicit so tests can freeze the clock.

To keep the migration mechanical, `internal/tui/views/ansi.go` holds a
compatibility shim: the old `ansiRed/Green/Yellow/Cyan/Reset/Bold/Dim`
identifiers are now package-level `var`s that re-export the theme tokens.
This let `diff_content.go`, `pipeline_log.go`, and `pipeline_stages.go`
pick up the new palette without touching their source — the bytes emitted
go through the design colours, the call sites read like before. New code
should reference the theme package directly; the shim is a bridge, not a
pattern to extend.

### What changed downstream

- `internal/tui/layout.go` — `highlightFocused` paints the focused frame
  and title in `theme.ColorAccent` instead of `gocui.ColorGreen`.
- `mrs.go`, `repos.go`, `diff_tree.go`, `pipeline_stages.go` — all use
  `theme.ColorAccent` for `SelBgColor` (cursor highlight).
- `detail.go` — deleted the in-file ANSI constants, rewrote `renderTabBar`
  and `renderOverview` to the wireframe spec (12-char key column, subtitle
  line, colored state dot, pipeline row, relative "Updated" time, dashed
  rule + description block).
- Overview-tab pipeline row — prefetched in `SetMR` the same way the diff
  is prefetched, so the row populates without waiting for the user to open
  the Pipeline tab. `cache.Do` in the GitLab client dedupes when the tab
  is later opened.

## Alternatives considered

- **Ship raw 16-colour ANSI and pretend the wireframe is aspirational.**
  Rejected — the wireframe colours are the accepted spec; any future
  design review would be unable to validate against the running UI.
- **Emit 256-colour escapes instead of truecolor.** Strictly smaller
  escape sequences, but forces manual quantisation from hex. tcell already
  does this for us when the terminal cannot render truecolor. Not worth
  the ceremony.
- **Put the palette in a YAML config so users can theme it.** Out of scope
  and premature — no user has asked. The theme package is a neutral
  starting point if that day ever comes.
- **Migrate every ANSI call site explicitly, skip the compat shim.** Would
  have turned a palette change into a diff across five view files plus
  their tests. The shim keeps the blast radius localised to `theme/`,
  `detail.go`, and the frame/selection hot spots.

## Consequences

- Any new colour consumer MUST add it through `theme` — no inline escapes
  in views.
- Tests that assert on specific SGR sequences must account for `gocui`
  re-serialising SGR with a trailing `;` before `m` in `pane.Buffer()`
  output. The recommended pattern is `strings.TrimSuffix(seq, "m")` on
  the expected prefix; `sgrPrefix` in `detail_test.go` shows the idiom.
- The compat shim (`views/ansi.go`) is a one-directional bridge. When a
  file next needs editing for a substantive reason, prefer swapping its
  `ansiXxx` references for `theme.FgXxx` and deleting the shim entry.
