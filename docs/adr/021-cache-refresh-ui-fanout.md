# 021: Cache Refresh UI Fan-Out — Reverses ADR 009

## Status

Accepted. Supersedes ADR 009 "Go Cache Port — Generics + No Background TUI Re-Render" with respect to the no-event-emission rule. The generics / disk-format decisions from ADR 009 remain in force.

## Context

ADR 009 banned cache-side event emission. Background refreshes updated memory + disk silently; fresh data surfaced only on the next user-driven `Do` call. The reasoning was UX: unrelated invalidations repainting panes would be a flicker regression.

In practice the rule cost the user real content. Concrete repro: the app boots, serves the MR list from stale cache, kicks a background refresh that pulls the real list from GitLab (including a newly-opened MR), stores it to memory + disk — and the screen stays frozen on the stale version. The new MR only appears if the user restarts the app. The stale-while-revalidate contract is "show stale, swap to fresh automatically"; the Go port had truncated it to "show stale, swap on the next explicit action". The Python port (ADR 006) always fired `_on_refresh` and threaded it into Textual for exactly this reason.

The flicker argument in ADR 009 was correct in the abstract but the fix it chose was too blunt. Selective fan-out + cursor/search/scroll preservation removes the flicker without dropping the event.

## Decision

1. `internal/cache/cache.Cache` exposes a single-subscriber callback via `SetOnRefresh(fn func(namespace, key string))`. `nil` clears. The callback is stored via `atomic.Pointer` so reads on the refresh goroutine don't contend with the RWMutex that protects entries.
2. The callback fires from `scheduleRefresh` after a successful background refresh AND only when all three gates pass:
   - `stored` — the entry still existed when the new payload landed (no mid-flight invalidation resurrected a dead key).
   - `changed` — the new payload is not `reflect.DeepEqual` to the previous one.
   - No loader error, no recovered panic.
   Cache misses (synchronous first-time loads) never fire — there was no stale version on screen to swap for.
3. The TUI installs the dispatcher in `tui.Run` right after `views.New`. The dispatcher wraps the event in `g.Update` so all view state mutation happens on the main loop thread (matching the existing `fetchXAsync` → `g.Update` → `apply*Seq` pattern throughout `internal/tui/views/`).
4. `views.Views.Dispatch(namespace, key)` routes on namespace alone to exactly one pane handler (`Repos.OnCacheRefresh`, `MRs.OnCacheRefresh`, or `Detail.OnCacheRefresh`). The routing table is hard-coded; per-view handlers apply a second gate on the `key` to restrict re-fetch to the currently-displayed project / MR.
5. Each pane preserves state on reload:
   - Repos: cursor re-seated on the project with the same `ID` after `apply`; clamps to nearest when the ID is gone. Search query + `searchActive` flag untouched.
   - MRs: same pattern keyed by MR `IID`. A separate `reloadFromCacheRefresh` / `applySilentReload` path skips the loading-state wipe `SetProject` does — stale rows stay on screen until fresh data lands (true SWR).
   - Detail: `project` is now persisted on the struct; each sub-namespace (`mr`, `mr_approvals`, `mr_discussions`, `mr_changes`, `mr_pipeline`, `mr_conversation`, `job_trace`) re-issues its existing `fetchXAsync` helper, which already has a seq-counter guard against stale apply.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│ background refresh goroutine (cache/refresh.go)                         │
│   load → refreshIfPresent (DeepEqual) → fireRefresh(ns, key)            │
└──────────────────┬──────────────────────────────────────────────────────┘
                   │ callback on cache goroutine
                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ tui.Run dispatcher closure (installed via cache.SetOnRefresh)           │
│   g.Update(fn) → main loop                                              │
└──────────────────┬──────────────────────────────────────────────────────┘
                   │ on main loop thread
                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ views.Views.Dispatch(namespace, key)                                    │
│   switch namespace → Repos | MRs | Detail .OnCacheRefresh               │
└──────────────────┬──────────────────────────────────────────────────────┘
                   │ key matches current state?
                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ view re-issues existing fetchXAsync / reload helper                     │
│   cache.Do returns fresh memory hit → g.Update → applyXSeq              │
│   cursor-by-ID restored; query/filter untouched                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Files

| File | Role |
|------|------|
| `internal/cache/cache.go` | `RefreshFunc` alias, `onRefresh atomic.Pointer[RefreshFunc]`, `SetOnRefresh`, `fireRefresh`, DeepEqual-aware `refreshIfPresent` returning `(stored, changed bool)`. |
| `internal/cache/refresh.go` | `scheduleRefresh` gets `namespace` arg; fires callback after DeepEqual gate. |
| `internal/cache/do.go` | Passes `namespace` into `scheduleRefresh`. |
| `internal/tui/app.go` | Installs dispatcher (closure wrapping `g.Update` + `Views.Dispatch`). |
| `internal/tui/views/views.go` | `Dispatch(namespace, key)` routing table. |
| `internal/tui/views/repos.go` | `OnCacheRefresh`; cursor-by-ID restore in `apply`. |
| `internal/tui/views/mrs.go` | `OnCacheRefresh`; silent-reload path; cursor-by-IID restore; `ReloadFromCacheRefreshSync` test helper. |
| `internal/tui/views/detail.go` | `project` field on struct, stored in `commitMR`. |
| `internal/tui/views/detail_refresh.go` | `OnCacheRefresh` + per-namespace `refetchOverviewPart` / `refetchJobTrace`. |

## Deliberate choices

These were real tradeoffs, not defaults.

1. **Single-subscriber callback over multi-subscriber channel.** There is exactly one consumer (the TUI dispatcher). A channel buys multi-subscribe fan-out we don't need and costs a goroutine lifecycle to drain. If a second consumer ever appears, promote `onRefresh` to a slice under a lock without breaking the callsite.
2. **Post-construction `SetOnRefresh` setter instead of a `WithOnRefresh` functional option.** Cache construction happens in `cli.Run` before `tui.Run`; the TUI doesn't exist yet to provide the callback. A functional option would force awkward construction order; a setter keeps the existing wiring intact.
3. **DeepEqual gate is a choice, not a forced requirement.** The task spec marked it optional. Chosen because at TUI scale (dozens to a few hundred items) `reflect.DeepEqual` is well below the paint threshold, and suppressing identical-payload refreshes eliminates the main repaint-noise vector.
4. **Key reconstruction via `cache.MakeKey` in view handlers rather than per-fetch "last key" stashing.** The view's `OnCacheRefresh` rebuilds the expected key from current state and compares against the event key. This keeps the surface small at the cost of a subtle coupling: a future change to `MakeKey`'s format would silently drop events until the views are re-synced. If that ever becomes a maintenance burden, switch to a stashed-key-per-fetch index. Flagged here so the next contributor sees the same rake before stepping on it.
5. **Silent-reload path for MRs (no loading-state wipe).** The user-driven `SetProject` clears `v.all`/`v.filtered` so the pane flashes "Loading…". A refresh event must not cause that flicker — the old list stays rendered until the apply lands. Implemented via `beginSilentReload` / `applySilentReload` twinned with the existing `beginLoad` / `apply` pair.

## What prevents flicker

The original ADR 009 rejected events on the grounds they'd cause flicker. 021 keeps flicker away via layered selectivity:

- **Namespace routing** — an event only reaches the one pane that displays that namespace. `mr_pipeline` never touches Repos or MRs.
- **Key matching** — within the pane, the event's key must match the currently-displayed project/MR. A `mr_pipeline` event for MR 99 is dropped when MR 42 is showing.
- **DeepEqual gate** — identical payloads don't fire at all.
- **Cursor-by-ID preservation** — row inserts/removals don't jump the cursor to an unrelated MR.
- **Query + filter + scroll preservation** — refresh never writes to the search query, the `searchActive` flag, the state/owner filter, or the pane origin/scroll.

## What NOT to add

- Multi-subscriber channel API. Out of scope for a single TUI consumer.
- Forced full-tree repaint on any event. Breaks selectivity.
- Polling tickers for namespaces already covered by SWR. The SWR background refresh + fan-out now covers the live-update need; extra tickers just multiply HTTP calls. Pipeline's user-toggled live-log ticker remains, unchanged — it's a live trace, not a cache-namespace poll.
- Firing the callback on cache misses (synchronous first load). There is no stale copy on screen to swap for.
- Firing the callback when the refresh payload is byte-equal to the prior one. Noise.
- Firing the callback when a mid-flight `Invalidate` removed the entry. The refresh correctly discarded itself; surfacing the event would encourage consumers to re-fetch and resurrect the just-invalidated key.

## Alternatives considered

1. **Re-export the ADR 009 "no event" rule, add an explicit user-triggered refresh keybind instead.** Rejected: the user's complaint is about wanting SWR to work transparently. A manual refresh key is a separate feature (and can still live as the existing explicit refresh path).
2. **Event channel (`<-chan RefreshEvent`) instead of callback.** Works, but buys multi-subscriber semantics we don't need. See Deliberate Choices #1.
3. **Callback runs on main loop automatically (cache owns a `g.Update` reference).** Rejected: cache would need to import `gocui`, which inverts the dependency (cache is lower-level than TUI). The TUI owns the `g.Update` hop instead; cache stays UI-agnostic.
4. **Compare via JSON-marshaled bytes instead of `reflect.DeepEqual`.** Saves reflect overhead but serializes twice per refresh. Not worth it at TUI scale.

## Migration from ADR 009

ADR 009 is amended with a superseded banner pointing here. The "What NOT to add" list in ADR 009 is rewritten: the entries forbidding `OnRefresh` and `CacheRefreshed` are removed; polling-ticker guidance stays. The per-view polling task in `tasks.md` (Phase G6) is marked redundant for any namespace flowing through `cache.Do` — the SWR background refresh + fan-out now covers that case. Pipeline live-log polling remains unrelated.
