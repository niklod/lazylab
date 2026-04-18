# ADR 014: Go Pipeline tab — flat stages listing + inline job log modal

## Status

Accepted — implemented in Phase G4 "Pipeline tab + inline job logs" sub-task.

## Context

The Python UI renders the MR Pipeline tab as a `HorizontalScroll` of `PipelineStageCard` widgets (one card per stage, each holding vertical `PipelineJobWidget`s), with a hidden-by-default `JobLogView` that slides over on Enter and hides on Esc. Porting to gocui raised four questions that the existing Diff-tab scaffold (ADR 013) only partially answered:

1. **How do stages render without a horizontal-scroll container?** gocui lays out by absolute rect only; there is no `HorizontalScroll`.
2. **Split the Pipeline area into stages + log, or replace one with the other?** Both approaches fit inside the existing sub-pane pattern.
3. **Separate cache entries for the latest pipeline and its jobs, or one combined `PipelineDetail`?** The data shape allows either.
4. **Does ANSI in the job trace need a custom parser, or does gocui's `OutputTrue` mode pass escape sequences through?**

## Decision

### 1. Stages render as a flat, vertical listing — one widget, no horizontal columns

`PipelineStagesView` mirrors `DiffTreeView` exactly: a `[]stageRow` where each row is either a bold-ANSI stage header (`job == nil`) or an indented job row carrying a pointer into the owning `PipelineDetail.Jobs` slice. j/k navigation skips header rows so Enter always lands on a real job; `MoveCursorToStart/End` snap to the first/last job row.

Rejected alternatives:

- **N top-level horizontal panes (one per stage).** Would require re-running `renderPanes` with a stage count that's only known post-fetch, re-computing pane widths, and handling the long-stage-name overflow case gocui has no scrolling for.
- **One stages pane with left-right cursor navigation between cards.** No `hjkl` primitive for "cursor moves to next card"; would reimplement half of gocui's view management.
- **Pipeline-as-overview — render stages as a string inside the detail pane.** Loses cursor selection entirely; Enter-on-job becomes impossible without a separate selector pane.

Parity cost: Python's left-right stage navigation (`h`/`l` for prev/next stage) is gone. Global `h`/`l` remains the focus cycle. j/k walking all jobs is a reasonable substitute for a list of 3-8 stages.

### 2. Log **replaces** stages when open — one sub-pane mounted at a time

When `DetailView.logOpen == true`, `managePipelineSubpanes` mounts `ViewDetailPipelineJobLog` filling the inner rect and unmounts `ViewDetailPipelineStages`. On Esc the reverse happens. One Pipeline-tab child view exists at any moment.

Mount-before-delete ordering is required: `gocui.DeleteView` does not clear the current-view pointer. If we deleted the stages pane first and then mounted the log pane, `g.CurrentView()` would dangle for the time between the two `SetView` calls. The manager always creates the incoming view, then calls `deleteViewIfPresent` on the outgoing one.

Rejected alternatives:

- **Split top (stages) + bottom (log).** Halves the log's readable area; stages scroll off-screen after 4-6 stages × 3-5 jobs. Python's original widget is modal-like for the same reason.
- **Log as a bordered floating overlay.** gocui has no z-order; overlapping views stack in layout-order and lose their frame colours.

`FocusOrder` encodes this dichotomy: when `DetailTabPipeline` is active, the order is `[repos, mrs, stages]` with the log closed and `[repos, mrs, log]` with it open. Global `h`/`l` cycles into and out of the Pipeline-tab family like any other tab.

### 3. Single combined `PipelineDetail` cache entry under `mr_pipeline`

`GetMRPipelineDetail(ctx, projectID, iid)` lists the MR's pipelines, takes the first, fetches its full record, lists its jobs — all under one cache key `(mr_pipeline, projectID, iid)`. Mirrors Python `get_pipeline_detail` exactly and avoids a second round-trip when a user clicks into the Pipeline tab: one `cache.Do[PipelineDetail]` hit, one network fetch on a miss.

Job traces use their own namespace `job_trace` keyed by `(projectID, jobID)`. Traces can be >100KB and shouldn't invalidate when the pipeline re-fetches; they expire independently on the cache TTL.

Rejected alternative: two separate entries (`mr_latest_pipeline` + `pipeline_jobs`). Atomic refresh becomes harder (two TTLs, two staleness windows); the combined shape also simplifies the UI state — the view holds one `*models.PipelineDetail` rather than juggling two futures.

### 4. ANSI SGR passthrough — everything else stripped (security)

`internal/tui/app.go` initialises gocui with `OutputMode: gocui.OutputTrue`, which forwards raw terminal escapes from the pane buffer to the underlying tcell screen. That's fine for diff rendering (we emit SGR bytes ourselves) but dangerous for job traces — CI stdout is attacker-influenced. A malicious pipeline could emit `\x1b[2J\x1b[H` (clear screen + home cursor) to overwrite the tab bar, `\x1b]8;;evil://payload\x07…\x1b]8;;\x07` to render a hyperlink with a misleading label, or bare `\r` / `\x08` to overdraw previously-rendered content.

`sanitizeTraceBody` in `internal/tui/views/pipeline_log.go` therefore allows only **SGR** (`\x1b[…m` — colour, bold, dim, reset) and strips every other CSI final, every OSC sequence (both BEL- and ST-terminated), bare ESC pairs, NUL, BS, and collapses CR / CRLF to LF. Unterminated CSI/OSC runs drop everything from the ESC to end-of-string. SGR is the only escape class a log actually needs; dropping the rest costs nothing visible and denies the entire TUI-chrome-overwrite attack surface.

Python used `rich.Text.from_ansi` which lands in a virtual Textual buffer — the rendering stack is what filters there. gocui doesn't have that layer, so sanitation has to happen at the ingestion boundary (`JobLogView.SetJob`).

### 5. Leaving the Pipeline tab closes the inline log

`[` / `]` is bound across the whole detail-family, including `ViewDetailPipelineJobLog`. If `SetTab` did not reset `logOpen` when leaving Pipeline, the next re-entry would hit a focus/mount mismatch: `focusTargetForTab(Pipeline) == stages`, `managePipelineSubpanes` mounts the log because `logOpen == true`, and `g.SetCurrentView("detail_pipeline_stages")` returns a wrapped `ErrUnknownView`.

`SetTab` and `SetTabSync` now call `resetJobLogLocked` when the previous tab was Pipeline and the new one is anything else. Re-entering Pipeline always starts on the stages pane — the same Python-era UX where the log is modal, not a persistent sibling.

## Consequences

- Adding the Conversation tab later will reuse `manageXSubpane` + `pendingFocus` without a new pattern.
- Future polling for live pipeline status (G6's "per-view polling for live status") can be driven from `PipelineStagesView` alone — the cache namespace split means a forced refresh of `mr_pipeline` doesn't blow away the `job_trace` a user is reading.
- The lock-ordering comment on `DetailView` extends to `{pipelineStages, jobLog}.mu` as siblings — the invariant remains "parent lock held while calling child methods; children never call parent".

## References

- ADR 009 — cache design (adds `mr_pipeline`, `job_trace` to the namespace table).
- ADR 010 — gocui layout + headless test harness.
- ADR 013 — Diff tab sub-pane strategy; Pipeline tab reuses the same `manageXSubpane` + `pendingFocus` mechanism.
- Python reference: `lazylab/ui/widgets/mr_pipeline.py` (stages/log widgets) and `lazylab/lib/gitlab/pipelines.py` (API semantics).
