# 012: MRs View, Filter Rotation, and Cross-View Wiring

## Decision

Phase G3 lands two pieces:

1. **`internal/gitlab/merge_requests.go`** exposes `ListMergeRequests`,
   `GetMergeRequest`, `GetMRApprovals`, and `GetCurrentUser` on `*Client`.
   Every read routes through `cache.Do[T]` when the client was built with
   `WithCache` — namespaces `mr_list`, `mr`, `mr_approvals`, `current_user`.
   Optional filters (`AuthorID`, `ReviewerID`) are `*int`; they reach `MakeKey`
   as `nil` when unset so the key elides the component, matching Python's
   `None`-skipping cache-key shape.

2. **`internal/tui/views/mrs.go`** renders the Merge Requests pane and owns
   its own keymap + ephemeral `mrs_search` pane (same pattern as
   `repos_search`):
   - `j/k/g/G` cursor nav, `/` opens search, Esc clears a submitted filter.
   - `s` cycles state filter: `opened → merged → closed → all → opened`.
   - `o` cycles owner filter: `all → mine → reviewer → all`.
   - Cycling refetches from GitLab; `mine`/`reviewer` also fetch
     `GetCurrentUser` to resolve the id.

   Cross-view wiring lives in `views.Views.selectProjectForMRs` (bound to
   `Enter` on the repos pane). Neither `ReposView` nor `MRsView` imports the
   other: the `Views` aggregator holds the edge.

3. **Stale-load guard.** Rapid filter toggles race each other — an older
   loader finishing after a newer one would clobber the newer results.
   `MRsView` increments a `loadSeq` counter on every `SetProject` and
   `apply(seq, ...)` drops results whose `seq` no longer matches. No mutex
   hand-off between loader goroutines is needed.

## Context

Python's `MRContainer` (`lazylab/ui/widgets/merge_requests.py`) reads filters
from config once and never rotates them at runtime; rotation was added in the
Go port because gocui does not expose a Textual-style settings modal. Two
filter bindings on the pane cost nothing and make the behaviour discoverable.

The cache-key parity question: Python's `@cached("mr_list", model=...)`
builds keys from positional args and skips `None`s — so `list_merge_requests(1,
"grp/x", state="opened")` and `list_merge_requests(1, "grp/x", state="opened",
author_id=None)` share a cache entry. The Go `MakeKey` helper already skips
`nil` args; the Go options struct passes `intPtrArg(p) any` so a `nil *int`
reaches `MakeKey` as a plain `nil`. Same key, same cache hit.

`GetCurrentUser` is cached under a single-arg namespace because the PAT
identifies exactly one user — a cache invalidation (e.g. token rotation)
would require restart anyway.

The Enter-on-repos binding belongs to neither view:
- Putting it on `ReposView` would force `ReposView` to import `MRsView` (or a
  message type) just to notify it.
- Putting it on `MRsView` would force `MRsView` to know the keyboard key that
  fires on a sibling pane.

`views.Views` already owns both; it is the natural home for the edge.

## Consequences

**Positive:**
- `MRsView.SetProjectSync(ctx, p)` mirrors `ReposView.LoadSync` — the e2e
  suite drives the fetch-and-apply path without running `g.MainLoop`, so the
  parity gate runs deterministically against `httptest.Server`.
- Filter rotation keeps config-derived defaults: `NewMRs` reads the config's
  `state_filter` / `owner_filter` once at construction, then owns them. No
  globals.
- `loadSeq` is a simple monotonic counter — cheaper than cancelling contexts
  mid-flight and avoids the complexity of a supersede-aware cache layer.

**Known follow-ups (not this task):**
- MR detail (overview/diff/conversation/pipeline) lands in G4. The detail pane
  will receive a `MergeRequest` from `MRsView.SelectedMR()` + a G4-owned
  binding.
- Cache invalidation after close/merge (G5) will call
  `ctx.Cache.InvalidateMR(projectID, mrIID)` — the `mr_list` entry is cleared
  by the broader `Invalidate("mr_list:{project_id}:*")` pattern (see ADR 009
  for the namespace table).
- A shared `SearchableTable[T]` abstraction across repos + MRs is deferred to
  G4: with two call sites the shape is finally visible, but the diff viewer
  in G4 will drive the third, making extraction safe.

## Alternatives considered

- **Single combined filter binding** (e.g. `f` opens a modal listing all
  filters). Rejected: modals in gocui cost a full pane + focus dance, and two
  bindings are fine for four filter values each.
- **Filter-aware cache keys per-filter file layout** — keep a separate disk
  subdirectory per `{state, owner}` combo. Rejected: `MakeKey` is already
  keyed on the args, so the default key shape handles it; a directory split
  would just complicate `cache.Invalidate` globbing.
- **Push state into the cache key as a hash** to keep keys short. Rejected:
  cache directories are debuggable artefacts; readable keys
  (`mr_list:42:opened:77`) beat opaque hashes when debugging a stale entry.
