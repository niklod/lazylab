# ADR 019: Go Conversation tab — single-pane thread cards

## Status

Accepted — implemented in Phase G4 "Conversation tab" sub-task.

## Context

The Conversation tab is the last unimplemented sub-tab in the MR Detail view and the closing gate of Phase G4 parity. Unlike Diff and Pipeline, the Python UI never shipped a Conversation view; the sole reference is `design/wireframes/conversation.js`. Questions raised before coding:

1. **Single-pane or multi-pane layout?** Diff uses two panes (tree + content); Pipeline uses one with a modal log overlay.
2. **How to render threads?** Flat list, card-with-spine, or tree-style collapsible?
3. **One cache namespace for discussions, or two (existing `mr_discussions` for stats + new one for the full payload)?**
4. **Prefetch on `SetMR`, like Diff/Pipeline?**
5. **Do `r` (toggle resolve) and `c` (new comment) belong in this sub-task, or in Phase G5 MR Actions?**

## Decision

### 1. Single pane with thread-card layout

`ConversationView` owns a single sub-pane inside the detail rect — mounted by `manageConversationSubpane` (`internal/tui/layout.go`) when `DetailView.currentTab == DetailTabConversation` and otherwise unmounted. The wireframe uses a single panel (`panelStage({ children: [p], ... })`); MR discussions are short enough that a split-pane tree+detail view would dominate the navigation budget without improving legibility.

Rejected alternatives:

- **Tree-list + note-detail split (Diff pattern).** Reply bodies are 1-5 lines on average. Splitting a narrow detail rect to show one 3-line reply at a time is hostile.
- **Modal overlay per thread (Pipeline log pattern).** Makes it impossible to eyeball "what's unresolved right now?" without opening threads one by one.
- **Reuse the ViewDetail base pane (like Overview).** Would mean rendering discussions inside the parent pane's frame, forcing DetailView.Render to own cursor highlighting — cursor work already lives on the child widgets.

### 2. Thread-card layout with vertical spine + two-level cursor

Each thread is a "card":

- Header row: `  ○ Thread · <id-slug> · <path:line> · unresolved` (red spine and bullet) or the resolved equivalent.
- Notes indented under the spine (`│` between, `╵` at the end, `╵` as a single-note marker).
- Author names in accent colour, timestamps via `theme.Relative`.

Sections, top to bottom: unresolved threads (always expanded) → resolved threads (collapsed to one dim line by default; `e` expands all) → divider → general comments (non-threaded notes).

System notes (`Note.System == true` — bot lifecycle events) are filtered out at render time but are counted by the cache-refreshed stats on the Overview tab. The pane keeps two cursor levels mirroring the wireframe:

- `j`/`k` → thread cursor (steps over unresolved → resolved-collapsed → general-comment headers).
- `J`/`K` → note cursor inside the selected thread (no-op for collapsed and general rows).

Rejected alternatives:

- **Flat list of rows with a single cursor.** Loses the "replies inside a thread" affordance; j/k from the last note of Thread 1 jumps to the first note of Thread 2 instead of the next thread header.
- **Tree-widget with open/close arrows per thread.** Indent + spine already communicate grouping; adding a third state (collapsed/expanded per thread) doubles the navigation surface with minimal gain.

### 3. Separate cache namespace `mr_conversation`

`mr_discussions` remains dedicated to `*models.DiscussionStats` (consumed by the Overview tab). The Conversation tab stores the full `[]*models.Discussion` under the new `mr_conversation` namespace.

Rejected alternative — unifying into one cache entry and deriving stats from the full list — would have required:

- Touching the stable `Overview` render path (`renderOverviewLocked` consumes `*DiscussionStats`, not a slice of discussions).
- Re-shaping `cache.Do`'s generic value type at the call site (stats-only consumers would have to pay the cost of the full payload).

Isolation wins: two producers, two consumers, two namespaces — trivial invalidation, no cross-feature coupling. `InvalidateMR` (internal/cache/cache.go) now clears both namespaces so G5 mutations can drop the right caches without caring which tab will ask next.

### 4. No prefetch on SetMR — fetch on first tab entry

Unlike Diff and Pipeline (both prefetched because the Overview row consumes their payload), Conversation has no Overview consumer. Fetching a full paginated discussions list for every MR the user clicks — even when they never open the tab — would be wasted work. Fetch is kicked only on the first `SetTab(DetailTabConversation, ...)` call since the last `commitMR`, mirroring the Diff path's pre-Overview behaviour.

Rejected alternative — unifying with `GetMRDiscussionStats` so Overview already warms the cache — was attractive but required refactoring Overview to derive stats from the discussions slice. Defer that refactor to G6 (Caching) where unification of all MR read paths is already on the roadmap.

### 5. `r` (toggle resolve) and `c` (new comment) are Phase G5

The keybind strip shows `r` and `c` for visual parity with the wireframe. Their handlers are registered as no-ops with inline TODO comments pointing at G5 (MR Actions), which already covers close/merge mutations and is the correct home for POST/PUT endpoints. Wiring dead keys here would be premature; silently swallowing a keystroke is less hostile than a "not implemented" flash message that trains users to ignore strip hints.

## Follow-ups deferred from post-implementation review

The 7-agent review cycle surfaced these items; all are non-blocking and are captured here so they aren't lost:

- **Pagination helper.** `internal/gitlab/discussions.go` `fetchMRDiscussions` and `internal/gitlab/merge_requests.go` `listMRDiscussionsRaw` drive the same `Discussions.ListMergeRequestDiscussions` endpoint with identical pagination shells but different per-page reducers. Extract a `paginateMRDiscussions(ctx, projectID, iid, reduce func([]*gogitlab.Discussion)) error` helper. Phase G6 (Caching) is the natural home — it already touches every GitLab read path.
- **`userFields` shim.** `toDomainUserFromBasic`, `domainUserFromNoteAuthor`, `domainUserFromNoteResolvedBy` all produce the same `models.User` from identical fields. A tiny unexported struct + one constructor would collapse ~25 lines across the `gitlab` package.
- **`rowKind` variants cull.** 14 enum values defined; only `rowKindUnresolvedHeader`, `rowKindResolvedHeader`, `rowKindUnresolvedNote`, `rowKindResolvedNote`, `rowKindGeneralHeader`, `rowKindResolvedCollapsed` are consulted. The rest are write-only markers; the `selectable` bool already drives motion logic. Worth consolidating when the next change touches `conversation.go`.
- **`threadCursor` derived from `selectedAnchor`.** Two fields that must stay in sync — `updateThreadCursorFromAnchorLocked` is called from five sites. `Cursor()` is test-only and off the hot path, so computing `threadCursor` on read eliminates the invariant burden.
- **`appendThreadRows` bool param → style struct.** The `isResolved bool` parameter flips three palette/kind pairs via sequential reassignment. Package-level `unresolvedStyle`/`resolvedStyle` value constants read cleaner.
- **Interface-on-consumer-side for `appcontext.AppContext.GitLab`.** `DetailView` depends on the concrete `*gitlab.Client` via the shared `AppContext` field. Narrow interfaces per widget (e.g. `discussionFetcher` here) would decouple tests — long-term debt shared with Diff/Pipeline/Approvals.

## Consequences

- `DetailView` grows five conversation-state fields. Split fetch methods (`beginConversationLoad`, `fetchConversationAsync`, `applyConversation`) live in `internal/tui/views/detail_conversation.go` so `detail.go` does not balloon past its current ~1200 lines (coding-style target is 800 LOC max; Diff/Pipeline already pushed this file over).
- Focus routing is trivial — `FocusOrder()` for `DetailTabConversation` is the base three-pane cycle with `ViewDetail` swapped for `ViewDetailConversation`. No multi-pane child sequence to maintain.
- Cache invalidation is one additional namespace prefix in `Cache.InvalidateMR`; G5 close/merge post-hooks already invalidate this list, so the new namespace comes along for free.
- Future reply / resolve mutations (G5) bind real handlers to `r`/`c` and trigger `InvalidateMR`; no structural change to this ADR.
- Parity cost: Python UI had no Conversation tab to match against, so this ADR defines the new source of truth. Any future Python-style ergonomics (e.g. inline reply without a modal) require a follow-up ADR.
