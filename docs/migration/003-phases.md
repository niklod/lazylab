# Go Rewrite: Phases

Phased migration mirroring the original Python phases. Each phase ends with a **feature-parity gate**: the same e2e scenario must pass against the Go binary as against the Python binary.

Tasks for each phase live in `../../tasks.md` under the "Go Rewrite" section. Every task keeps the project-mandated DoD + Testing Steps format.

## Phase G1 — Scaffold + Config + API Client

- Create Go module, Makefile targets (`build`/`lint`/`test`/`test-e2e`), golangci-lint config, goreleaser config.
- Implement `internal/config/` (yaml.v3 + adrg/xdg + mergo for defaults, afero-backed I/O).
- Implement `internal/context/AppContext`.
- Decide and record (ADR) which Go GitLab client library to use (candidate: `xanzy/go-gitlab`). Wrap in `internal/gitlab/client.go`.
- Implement `internal/models/` for `User`, `Project`, `MergeRequest`, `Pipeline`, `ApprovalStatus`.
- Implement `gt version` and `gt run` subcommands via flaggy.
- Port the file-backed cache (ADR 006) to `internal/cache/`.

**Parity gate:** `gt version` prints the correct version; `gt run` exits cleanly with no TUI (or with a stub TUI) when config is valid.

## Phase G2 — Repositories Panel

- gocui layout: 3-pane grid (repos / mrs / detail).
- Vim-like key bindings (`h j k l g G / [ ]`) registered centrally in `internal/tui/keys.go`.
- `views/repos.go` with searchable table, favourites, sort.

**Parity gate:** e2e test `test_repos_render.go` mirrors the Python `test_app_launch.py` and passes.

## Phase G3 — Merge Requests List

- `internal/gitlab/merge_requests.go`: `ListMergeRequests`, `GetMergeRequest`, `GetMRApprovals`.
- `views/mrs.go`: table with status icons, filters (state, mine, reviewer).
- Wire `RepoSelected` → refresh MR table.

**Parity gate:** MR list renders and filters behave identically to Python.

## Phase G4 — MR Detail Tabs

- Overview tab: author, date, status, branches, conflicts, comment count.
- Diff tab: file tree + side-by-side diff (port ADR 003 approach).
- Conversation tab: threaded discussions.
- Pipeline tab: stage blocks + inline job logs (ADR 004).
- Tab switching with `[` / `]`.

**Parity gate:** all four tabs render with real data; job log open/close works.

## Phase G5 — MR Actions

- `close_merge_request` / `merge_merge_request` API calls.
- Modal confirmation views (`x` to close, `M` to merge).
- Post-action cache invalidation.
- (Create/edit MR deferred, matching Python scope.)

**Parity gate:** close and merge flows complete with state guards and refresh.

## Phase G6 — Caching

- Port async cache with stale-while-revalidate (ADR 006) using goroutines + sync primitives.
- `@cached`-equivalent decorator pattern (Go: higher-order wrapper function).
- Apply to all read-only GitLab calls. Invalidate on mutations.
- `CacheRefreshed` event propagation to active views.

**Parity gate:** cached project list test + MR invalidation test pass; background refresh updates the UI live.

## Phase G7 — Polish and Cut-Over

- Command palette.
- Error handling improvements (user-friendly messages, logrus with context).
- goreleaser dry-run producing macOS/Linux binaries + Docker image.
- CHANGELOG and README update.
- **Cut-over:** merge `go-rewrite` → `master`. Delete Python code. Tag first Go release.

**Parity gate:** full e2e suite green against Go binary. No Python runtime required to use `gt`.
