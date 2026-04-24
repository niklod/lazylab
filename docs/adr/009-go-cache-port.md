# 009: Go Cache Port — Generics + No Background TUI Re-Render

## Status

**Superseded (2026-04-24) by ADR 021.** The "no background TUI re-render" rule in this ADR has been reversed: background refreshes that change state now fire a `SetOnRefresh(namespace, key)` callback the TUI fans out selectively. The generics / disk-format / lifecycle decisions below remain in force. Do NOT apply the "What NOT to add" list to new code — it has been rewritten below.

## Decision

Port the Python file-backed cache (ADR 006) to Go as package `internal/cache/`,
preserving stale-while-revalidate semantics with two deliberate divergences:

1. **Generic `Do[T]` replaces the `@cached(model=...)` decorator.** Type
   reconstruction on disk load happens via `encoding/json` + the call-site
   type parameter. No explicit Pydantic-style model registry.
2. **No background-refresh event.** Python wires `_on_refresh(namespace, key)`
   → Textual `CacheRefreshed` message → selective widget reload. The Go port
   drops this entirely. Background refresh updates memory + disk silently;
   fresh data surfaces only on the next caller-driven `Do` (navigation,
   explicit refresh key, post-mutation reload).

## Context

Phase G1 requires a caching substrate before any GitLab read path is wired in
G3/G4. The Python design's SWR contract is sound — the open question was
whether to preserve the "background refresh pokes the UI" channel that Textual
wires up.

**User directive:** the TUI must not re-render in response to background cache
activity. Unrelated invalidations repainting the screen is a UX regression.
Stale data on screen is fine; silent cache updates are fine; flickering is
not.

This aligns cleanly with how gocui/tcell expect to be driven — the TUI loop
owns redraws, triggered by user input, not by arbitrary goroutines. A cache
event channel would force view code to choose between ignoring refreshes
(pointless plumbing) or forcing redraws (the behavior the user rejected).

## Architecture

```
┌────────────────────────────────────────────────────────────────┐
│  cache.Do[T](ctx, c, namespace, loader, args...) (T, error)    │
│                                                                │
│  1. In-memory hit                                              │
│     ├── FRESH → return immediately                             │
│     └── STALE → return stale, goroutine refreshes silently     │
│  2. Disk hit                                                   │
│     ├── FRESH → promote to memory, return                      │
│     └── STALE → promote, return, goroutine refreshes silently  │
│  3. MISS → loader(ctx) synchronously, store, return            │
└────────────────────────────────────────────────────────────────┘
```

### Files

| File | Concern |
|------|---------|
| `cache.go` | `Cache` struct, `New`, `Invalidate`, `InvalidateMR`, `InvalidateAll`, `Shutdown`, `put` |
| `entry.go` | in-memory `entry{data any, createdAt}` with `isStale(now, ttl)` |
| `disk.go` | afero-backed `loadDisk[T]`, `saveDisk`, `removeDiskFile`, key sanitization |
| `key.go` | public `MakeKey(namespace, args...)` — skips nil, joins with `:` |
| `refresh.go` | `scheduleRefresh` — dedup via `sync.Map` + `sync.WaitGroup` drain |
| `do.go` | generic `Do[T]` entry point |

### Key design choices

| Choice | Rationale |
|--------|-----------|
| Generic `Do[T]` as free function | Go does not allow type parameters on methods. `Do` takes the cache as first arg — called as `cache.Do[Project](ctx, c, "project", loader, id)`. |
| `sync.RWMutex` around entries map | Simpler than `sync.Map`; read-heavy workload justifies RW. |
| `sync.Map` for pending refreshes | Lock-free `LoadOrStore` gives atomic "am I first" dedup without touching the entries mutex. |
| `rootCtx` + `WaitGroup` + `Shutdown(ctx)` | Graceful drain with caller-specified deadline. Background loaders receive `rootCtx` and should respect cancellation. |
| `Invalidate` clears BOTH memory AND disk | Deliberate divergence from Python, which only clears memory and thus can serve stale data from disk after a mutation. For a TUI this would manifest as post-merge MR lists still showing the merged MR. |
| Functional option `WithClock(now)` | Deterministic tests without `time.Sleep`. |
| Functional option `WithLogger(logf)` | Logger-agnostic until logrus lands later in G7. |

### Disk format (wire-compatible with Python)

```json
{"created_at": 1715000000.0, "data": <T as JSON>}
```

Path: `{cfg.Directory}/api_{sanitized_key}.json` where sanitization is the
same `:` → `__`, `/` → `_`, `=` → `_`. Dir perm `0700`, file perm `0600`.

A fresh Go binary can read caches written by the Python binary (and vice
versa), which simplifies the cut-over in Phase G7. Go models carry the same
`json:"..."` tags as the Python Pydantic field aliases.

## What NOT to add

*(Rewritten 2026-04-24 for ADR 021. The event-emission ban from the original revision has been lifted; see ADR 021 for the supported `SetOnRefresh` API.)*

- Polling ticker inside `Cache`. Ownership stays with views that genuinely
  need sub-TTL cadence (the user-toggled pipeline live log is the only
  current example). Namespaces that already flow through `cache.Do` are
  covered by the SWR background refresh + the ADR 021 fan-out; a ticker on
  top is pure HTTP amplification.
- Multi-subscriber channel API on `Cache`. There is one consumer (the TUI
  dispatcher). Promote to a slice under a lock only if a second consumer
  actually appears. See ADR 021 "Deliberate choices".
- Forced full-tree repaint on any cache event. Selective routing +
  key-matching + cursor preservation is what keeps flicker away; broadcasting
  defeats all three.

## Alternatives considered

1. **1:1 Python port with `_on_refresh` callback** — rejected; violates user
   UX requirement. See Context.
2. **No generics, use `any` + caller-side type assertions** — works but
   pushes boilerplate into every call site. Generics were added to Go 1.18;
   the project is on 1.25. No reason to avoid them.
3. **Separate packages for memory vs disk tiers** — premature; the SWR logic
   interleaves the two tiers so tightly that splitting them would produce
   a thinner but more tangled API.
4. **`adrg/xdg` for cache path** — unnecessary; `config.defaultCacheDir()`
   already resolves `$XDG_CONFIG_HOME/lazylab/.cache` with a home-dir
   fallback (see ADR 007). Adding `adrg/xdg` would be dependency churn for
   zero behavioral change.

## Application plan

Phase G1 (this ADR): package lands, wired into `AppContext` and `cli.Run`,
but no call site uses `Do` yet. `Shutdown` is deferred at the end of `Run`
with a 2-second budget.

Phases G3–G5 apply `Do[T]` to read-only GitLab calls:

| Namespace | Function |
|-----------|----------|
| `projects` | `ListProjects` |
| `project` | `GetProject` |
| `project_path` | `GetProjectByPath` |
| `mr_list` | `ListMergeRequests` |
| `mr` | `GetMergeRequest` |
| `mr_changes` | `GetMRChanges` |
| `mr_approvals` | `GetMRApprovals` |
| `pipeline_latest` | `GetLatestPipelineForMR` |
| `pipeline_detail` | `GetPipelineDetail` |
| `job_trace` | `GetJobTrace` |

Phase G5 calls `ctx.Cache.InvalidateMR(projectID, mrIID)` after close/merge.

Phase G6 revisits whether any view needs live refresh. Expected answer:
pipeline status is the only plausible candidate, implemented as a local
polling goroutine in the pipeline view, not as a global cache event.
