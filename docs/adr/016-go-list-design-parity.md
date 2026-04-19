# ADR 016: Repos & MRs list design parity

## Status

Accepted — implemented as a follow-up to ADR 015 (theme palette) once the
list panes were measured against `design/project/wireframes/layout.js` and
found to drift in icons, colours, alignment, and empty-state copy.

## Context

ADR 015 introduced the design palette but the Repositories and Merge
Requests list views still rendered with placeholder formatting:

- Repos: `☆`/`★` plain (uncoloured), no relative timestamps, no header
  meta line.
- MRs: state shown as a single uppercase letter (`O`/`M`/`C`/`?`), no
  draft detection, fields ordered `!IID letter author title`, filter
  banner emitted as `[state=X owner=Y]` inline.
- Both panes set `SelFgColor = gocui.ColorBlack` against the accent
  background — wireframe specifies `#fff`.
- Loading / empty / error states were bare ASCII strings, missing the
  dim instructional copy from `design/project/wireframes/states.js`.

The wireframes specify a different shape: pane-number prefix
(`[1] Repositories`, `[2] Merge Requests`), filtered/total count,
right-aligned dim metadata column (timestamp / `@author`), coloured glyph
per MR state with a dedicated draft glyph, and instructional empty states.

## Decision

### Render meta inside the pane buffer, not in `pv.Title`

`gocui.View.Title` is a single string with no split-justification — there
is no native way to right-align meta beside the title. Putting the meta
into the buffer's first row matches how the wireframe's `panel-head`
visually composes inside the dark panel, keeps `layout.go` decoupled from
view-level state, and lets us colour the count / filter chips with the
same SGR escapes the rest of the row uses.

The first buffer row becomes a pane-scoped header:

```
[1] Repositories · 18/42
[2] Merge Requests · state:opened · owner:mine · 6/12
```

The header eats one line of the pane; cursor placement offsets by `+1`
(repos) and stays at `+1` (mrs, where the existing banner already did
this).

### Draft detection by title prefix

GitLab's API exposes drafts via title convention. `MergeRequest.IsDraft()`
matches case-insensitively against `Draft:`, `[Draft]`, `[WIP]`, `WIP:`
after trimming leading whitespace. The method lives next to `IsOpen` /
`IsMerged` / `IsClosed` so renderers reach for one consistent vocabulary.

Draft overrides the underlying state for the rendered glyph: a draft
opened MR gets the draft `◐` in the draft tone, never the `●` ok dot.

### `FgDim` colour vs `Dim` SGR attribute

`theme.Dim` is `\x1b[2m` — an SGR **attribute** (half-intensity default
foreground), not a colour. The wireframe's `--term-dim` is `#7a7970`,
a specific colour. They behave differently on real terminals (half-bright
white vs a discrete neutral grey).

Added `theme.FgDim` for the colour, alongside the existing palette
constants. `theme.Dim` is kept for callers that want the SGR attribute
specifically. New "dim" usage everywhere in the list views uses `FgDim`.

### `theme.ColorSelectionFg`

Added a single `gocui.NewRGBColor(255, 255, 255)` symbol for the
selection-row foreground so the value lives next to `ColorAccent`. Both
list views set `SelFgColor = theme.ColorSelectionFg`, matching the
wireframe's `.sel { color: #fff }`.

### `formatRepoRow` / `formatMRRow`

A small `internal/tui/views/row_format.go` exposes two helpers that take
the pane width, an icon (already colour-wrapped), a left-side payload,
and a right-side metadata token. They left-pad the payload (truncating
with `…` via `rivo/uniseg` to respect grapheme clusters) so the right
column ends exactly at `paneWidth`. `paneWidth ≤ 0` falls back to an
unaligned form so headless tests render readable rows.

`visibleWidth` strips SGR escapes before measuring; otherwise the bytes
of `\x1b[38;2;217;119;87m` are counted as visible glyphs and column math
underpads the row.

## Alternatives considered

- **Embed meta in the gocui frame title** by computing right-aligned
  padding against `pv.Size()`. Rejected: title rendering is gocui's, the
  frame is one cell tall but the title is drawn into the border row, and
  any padding scheme has to fight the focus-highlight repaint. Buffer
  rows are ours to compose.
- **Return three pieces from `formatRepoRow` / `formatMRRow`** (left,
  pad-count, right) and let the caller assemble. Rejected — call sites
  would all do the same `+ pad + ` join, and the helper is the natural
  home for the truncation rule.
- **Detect draft via a server-side flag** (some GitLab endpoints expose
  `work_in_progress`). Rejected for now — the field isn't in the existing
  `MergeRequest` model and the title-prefix convention is what GitLab
  itself uses to surface the draft chip in their web UI.

## Consequences

- Any future list pane should follow the same contract: pane-number
  prefix in the buffer header, `theme.FgDim` for meta, `theme.Wrap` to
  colour cells, `formatRow*` helpers for column alignment, instructional
  empty-state copy.
- Tests that assert on specific SGR sequences must match the truecolor
  RGB *prefix* (e.g. `\x1b[38;2;217;119;87`) rather than the full
  `\x1b[…m` byte form, because gocui's pane buffer re-emits SGR with a
  trailing `;m` (see ADR 015 consequences).
- `MergeRequest.IsDraft()` is the single place to evolve draft detection
  if GitLab adds an explicit flag to the response payload.
