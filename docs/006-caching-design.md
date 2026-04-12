# 006: API Caching — Stale-While-Revalidate

## Decision

Implement a two-tier caching layer (in-memory + disk) around the GitLab API
functions using a `@cached` decorator.  The cache uses **stale-while-revalidate**
semantics: cached data is always served immediately, and stale entries are
refreshed transparently by a background `asyncio.Task`.

## Context

Every panel switch, MR selection, or tab change triggers one or more GitLab
API calls.  Without caching, the UI blocks on network round-trips each time,
making navigation feel sluggish — especially for data that changes
infrequently (project lists, MR diffs, pipeline details).

### Requirements

- **Instant UI**: never block the user on a cache miss if stale data exists.
- **Freshness**: data should converge to fresh within the configured TTL.
- **Persistence**: survive app restarts via disk serialisation.
- **Correctness**: mutations (close/merge MR) must invalidate affected entries.

## Architecture

```
┌──────────────────────────────────────────────────┐
│  @cached("namespace", model=MyModel)             │
│                                                  │
│  1. Check in-memory dict                         │
│     ├── HIT + FRESH  → return immediately        │
│     └── HIT + STALE  → return + schedule refresh │
│  2. Check disk cache                             │
│     ├── HIT + FRESH  → load into memory, return  │
│     └── HIT + STALE  → load, return, refresh     │
│  3. MISS → await original fn, store, return      │
└──────────────────────────────────────────────────┘
```

### Key Design Choices

| Choice | Rationale |
|--------|-----------|
| `time.time()` for timestamps | Allows disk entries to check staleness across app restarts (monotonic clock resets). |
| `inspect.signature` for key normalisation | Ensures `fn(1, "a")` and `fn(project_id=1, path="a")` produce the same cache key. |
| Deduplication of background refreshes | A `_pending_refreshes: set[str]` prevents multiple concurrent refreshes for the same key. |
| Lazy configuration via `_ensure_configured()` | Avoids circular imports (`cache.py` ← `context.py` ← `config.py`); reads TTL and cache dir from `LazyLabContext.config` on first use. |
| Per-namespace invalidation with prefix matching | `invalidate("mr_list:10:")` clears all MR lists for project 10 without touching project 20. |

### What is cached

| Namespace | Function | Model |
|-----------|----------|-------|
| `projects` | `list_projects` | `Project` |
| `project` | `get_project` | `Project` |
| `project_path` | `get_project_by_path` | `Project` |
| `mr_list` | `list_merge_requests` | `MergeRequest` |
| `mr` | `get_merge_request` | `MergeRequest` |
| `mr_changes` | `get_mr_changes` | `MRDiffData` |
| `mr_approvals` | `get_mr_approvals` | `ApprovalStatus` |
| `pipeline_latest` | `get_latest_pipeline_for_mr` | `Pipeline` |
| `pipeline_detail` | `get_pipeline_detail` | `PipelineDetail` |
| `job_trace` | `get_job_trace` | `None` (str) |

### What is NOT cached

Mutations: `close_merge_request`, `merge_merge_request`.  These call
`api_cache.invalidate_mr(project_id, mr_iid)` after the API call, which
clears all six MR-related namespaces for that MR.

### Disk serialisation

Pydantic models are serialised via `model_dump(mode="json")` with a
`__pydantic__` tag so the deserialiser knows to reconstruct them.  Lists,
`None`, and plain strings are stored as-is.

## Alternatives Considered

1. **Cache at the `GitLabClient` level** — rejected because the public API
   surface (module-level functions) is a better boundary; the client holds
   raw python-gitlab objects that are harder to serialise.

2. **TTL-only (no stale-while-revalidate)** — rejected because it still
   blocks the user on the first miss after expiry.

3. **No disk cache** — simpler, but startup would always require network
   calls.  The existing `SearchableDataTable` disk cache covers tables but
   not detail views (diffs, pipelines, approvals).

## Configuration

```yaml
cache:
  directory: ~/.config/lazylab/.cache
  ttl: 600  # seconds
```

## UI Refresh on Background Revalidation

When a background refresh completes, the cache fires an `_on_refresh(namespace, key)`
callback.  The `LazyLab` app wires this to a `CacheRefreshed` Textual message
that flows through the screen to active tab panes:

```
AsyncCache._background_refresh()
  → _on_refresh(namespace, key)
    → LazyLab.post_message(CacheRefreshed)
      → LazyLab.on_cache_refreshed()
        → LazyLabMainScreen.handle_cache_refresh()
          → MROverviewTabPane.handle_cache_refresh()   (mr_approvals, pipeline_latest)
          → MRPipelineTabPane.handle_cache_refresh()   (pipeline_detail)
```

Each widget checks the namespace and key against its current MR before
re-triggering its `@work` loader.  Since the cache was just refreshed, the
loader returns instantly from in-memory cache and updates the UI.

The callback does **not** fire on fresh cache hits or on failed refreshes.
