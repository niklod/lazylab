# ADR 013: Go Diff tab — sub-pane strategy, bind-per-view, API choice

## Status

Accepted — implemented in Phase G4 "Diff tab" sub-task.

## Context

The Python UI renders the MR Diff tab as a Textual `Horizontal` with `DiffFileTree` (30% width) + `DiffContentView` (70%). Parity on `go-rewrite` required three decisions that are not obvious from the code alone:

1. **How does the single `detail` gocui pane host two sub-panels + a tab bar?**
2. **Where do the `[`/`]` tab-cycle keys live when focus has moved to a sub-pane?**
3. **Which GitLab client endpoint feeds the tree — `/changes` (deprecated) or `/diffs`?**

## Decision

### 1. Sub-pane strategy: ephemeral child views inside the `detail` rect

The `detail` pane keeps its outer frame + title. When `DetailView.currentTab == DetailTabDiff`, the layout mounts two frameless child views inside the inner rect (row 0 = tab bar, rows 1..end = sub-panes): `detail_diff_tree` (~30% wide) and `detail_diff_content` (~70% wide), separated by a one-column visual gutter (no drawn border — the detail frame is the only outer edge). `manageDiffSubpanes` mirrors the existing `manageSearchPane` ephemeral-pane pattern: self-heals on every layout tick, deleted when the tab switches away.

Rejected alternatives:

- **Everything in one gocui view.** Single cursor, single scroll origin, no way to separate tree navigation from diff scrolling.
- **Split the detail pane at the layout level** (three top-level panes in the right column). Breaks the existing 3-pane frame contract + complicates focus-order for non-Diff tabs where the split shouldn't exist.
- **Nested `Frame: true` on both sub-panes.** Visually muddy — gocui doesn't de-duplicate adjacent frames.

### 2. `[`/`]` bound per-view across the detail family, not globally

gocui dispatches keybindings by the currently-focused view name. If `[`/`]` were bound only on `ViewDetail`, the user would get stuck in the Diff tab the moment focus moved to `detail_diff_tree` (or from there to `detail_diff_content`) — no way to cycle back to Overview. Each cycle key is therefore bound on **three** views: `ViewDetail`, `ViewDetailDiffTree`, `ViewDetailDiffContent`.

Rejected alternative: global `[`/`]`. Would fire during MRs-pane search/filter (where `/` + typed input must take precedence) and during the repos-pane search input. Keeping the bindings scoped to the detail family preserves that isolation; the cost (three bindings per key) is acceptable for four total registrations (`[` × 3, `]` × 3).

Focus shift after a tab switch is deferred via `DetailView.pendingFocus` — the cycle handler only mutates tab state; the layout picks up the token after `manageDiffSubpanes` mounts the target sub-pane and calls `g.SetCurrentView`. This avoids an `ErrUnknownView` when the sub-pane is being mounted the same tick.

### 3. GitLab endpoint: `ListMergeRequestDiffs`, not `/changes`

`client-go` marks `GetMergeRequestChanges` (the `/changes` endpoint Python used) as deprecated in favour of `ListMergeRequestDiffs` on the `/diffs` endpoint. Both return per-file diffs; `/diffs` is paginated (matches `ListMergeRequests` + `ListMergeRequestDiscussions` patterns in this project). The `MergeRequestDiff` Go struct exposes `OldPath`, `NewPath`, `Diff`, `NewFile`, `RenamedFile`, `DeletedFile` — identical 1:1 to `models.MRDiffFile`, so the raw-to-domain mapper is a straight copy.

Parity note: `Collapsed: true` / `TooLarge: true` responses carry `Diff: ""`, matching Python's `mr.changes()` path which returned an empty string for those cases. The Diff content view degrades gracefully — `renderDiffMarkup("")` returns the "Binary file or no diff available" hint.

## Consequences

- Adding the Conversation / Pipeline tabs later will reuse the same sub-pane pattern (each tab owns whatever child views it needs; layout consults `CurrentTab` to decide which family to mount).
- The `pendingFocus` token is a lightweight, single-slot channel between view state and layout — if future tabs need multi-step focus choreography, it should grow into a queue rather than accumulate ad-hoc booleans on DetailView.
- Cache namespace table now includes `mr_changes`; add to ADR 009's table when it's next revisited.

## References

- ADR 009 — cache design (cache.Do namespaces)
- ADR 010 — gocui layout + headless test harness
- ADR 012 — MRs view pattern (cross-view wiring via Views)
- lazygit `pkg/commands/patch/*` — patch parser + format helpers we echoed (kept implementation simpler since our tree has no collapse / range-select)
