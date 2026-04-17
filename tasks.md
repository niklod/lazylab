# LazyLab Tasks

## Phase 1: Project Scaffold + Config + API Client

- [x] Create project scaffold (pyproject.toml, Makefile, .gitignore)
- [x] Implement YAML config with Pydantic models
- [x] Implement LazyLabContext singleton
- [x] Implement GitLab API client wrapper (python-gitlab + anyio thread offloading)
- [x] Implement Pydantic models (User, Project, MergeRequest, Pipeline, ApprovalStatus)
- [x] Implement CLI entry point (`gt version`, `gt run`)
- [x] Implement file-based cache
- [x] All Phase 1 tests pass (28 unit tests)

## Phase 2: Core UI — Repositories Panel

- [x] Implement base widgets (VimLikeDataTable, SearchableDataTable[T], LazyLabContainer, ToggleableSearchInput)
- [x] Implement main app and screen layout (LazyLabMainScreen, MainViewPane, SelectionsPane, SelectionDetailsPane)
- [x] Implement ReposContainer with SearchableDataTable[Project], favorites, search
- [x] Implement vim-like keybindings (hjkl, g/G, /, [, ])
- [x] Auto-focus repos table after loading
- [x] E2E test passes (app launches and renders)

## Phase 3: Merge Requests List

- [x] Implement MR API calls (list_merge_requests, get_merge_request, get_mr_approvals)
- [x] Implement MRContainer widget with status icons, search, filter
- [x] Implement MR filtering (by state, mine, reviewer)
- [x] Fix: parent forwards RepoSelected to MRContainer (messages bubble up only)

## Phase 4: MR Detail — Overview Tab

- [x] Implement MROverviewTabPane (author, date, status, branches, conflicts, comments)
- [x] Implement async approval status loading
- [x] Implement async pipeline status loading (get_latest_pipeline_for_mr)
- [x] Implement tab switching with [ and ] keys
- [x] Add placeholder tabs for Diff, Conversation, Pipeline
- [x] All tests pass (28 unit + 1 e2e), lint clean, 0 pyright errors

## Bugfixes

- [x] Rename CLI command from `lazylab` to `gt`, config path to `~/.config/lazylab/`
- [x] Auto-focus repos table after project load (Enter key not working)
- [x] Forward RepoSelected to MRContainer via parent (Textual messages only bubble up)

## Phase 5: MR Detail — Diff Tab

- [x] Add MRDiffFile and MRDiffData Pydantic models
- [x] Add get_mr_changes() API function (fetches MR file changes via python-gitlab)
- [x] Add ctrl+d / ctrl+u keybindings for diff scrolling
- [x] Implement DiffFileTree widget (Tree[MRDiffFile] with directory grouping, status icons)
- [x] Implement DiffContentView widget (VerticalScroll + Static with Rich markup coloring)
- [x] Implement MRDiffTabContent layout (Horizontal split: file tree 30% + diff content 70%)
- [x] Replace MRDiffTabPane placeholder with real implementation (async load via @work)
- [x] Unit tests for models (7 tests) and diff rendering (9 tests)
- [x] E2E test for diff tab rendering
- [x] All tests pass (44 unit + 2 e2e), lint clean, 0 pyright errors

## Future Phases
- [ ] Phase 6: Conversation tab — GitLab discussions/notes
  - **DoD:** Threaded comments display with resolve status
  - **Testing:** E2E test for conversation rendering
- [x] Phase 7: Pipeline tab — stage visualization, job logs
  - **DoD:** Pipeline stages shown as blocks with job status
  - **Testing:** E2E test for pipeline visualization
  - [x] Add PipelineJob and PipelineDetail Pydantic models
  - [x] Add PIPELINE_JOB_STATUS_ICONS to constants
  - [x] Add get_pipeline_detail() API function (fetches pipeline + jobs)
  - [x] Add OPEN_IN_BROWSER keybinding (`o` key)
  - [x] Create mr_pipeline.py with PipelineJobWidget, PipelineStageCard, PipelineStagesView, MRPipelineTabContent
  - [x] Replace MRPipelineTabPane placeholder with real implementation
  - [x] Unit tests (16 new tests: models, icons, grouping, duration formatting)
  - [x] E2E tests (3 new tests: stage rendering, empty pipeline, browser opening)
  - [x] Add get_job_trace() API function (fetches job log via python-gitlab)
  - [x] Add JobLogView widget (inline log viewer with ANSI→Rich conversion)
  - [x] Add JobSelected message for Enter-on-job interaction
  - [x] Add CLOSE_LOG keybinding (Escape to close log panel)
  - [x] E2E test for inline job log viewing + Escape to close
  - [x] ADR: docs/004-pipeline-tab-design.md
- [x] Phase 8: MR actions — close, merge (create/edit deferred)
  - **DoD:** Close and merge via modal confirmation screens, state guards, post-action refresh
  - **Testing:** 4 unit tests + 8 E2E tests for modal flows, guards, cancellation
  - [x] Add close_merge_request() and merge_merge_request() API functions
  - [x] Add CLOSE_MR (`x`) and MERGE_MR (`M`) keybindings
  - [x] Add MRActionCompleted message
  - [x] Create CloseMRScreen and MergeMRScreen modal screens
  - [x] Integrate action methods into MRContainer with state guards
  - [x] Handle MRActionCompleted in LazyLabMainScreen to refresh detail tabs
  - [x] Unit tests for API functions (4 tests)
  - [x] E2E tests for close/merge flows (8 tests)
  - [x] ADR: docs/005-mr-actions-design.md
- [ ] Phase 9: Polish — command palette, error handling, caching
  - **DoD:** Production-quality UX
  - **Testing:** Full test suite >80% coverage
  - [x] Implement AsyncCache with stale-while-revalidate (in-memory + disk)
  - [x] Add @cached decorator for async API functions
  - [x] Apply caching to all read-only GitLab API functions (projects, MRs, pipelines, approvals, diffs, job traces)
  - [x] Add cache invalidation after mutations (close/merge MR)
  - [x] Lazy cache configuration from LazyLabContext.config
  - [x] Unit tests (25 new tests: entry staleness, put/get, invalidation, disk roundtrip, decorator, background refresh, dedup, lazy config)
  - [x] E2E tests (2 new tests: cached project list, MR invalidation)
  - [x] Fix test isolation: reset api_cache + LazyLabContext between tests, explicit cache dir in all E2E _mock_config
  - [x] ADR: docs/006-caching-design.md
  - [x] Add CacheRefreshed message + _on_refresh callback to AsyncCache
  - [x] Wire callback through App → Screen → active tab panes (MROverviewTabPane, MRPipelineTabPane)
  - [x] UI auto-updates when background refresh completes (approvals, pipeline status, pipeline detail)
  - [x] Unit tests for callback (3 tests: fires on refresh, not on hit, not on failure)
  - [ ] Command palette
  - [ ] Error handling improvements

---

## Go Rewrite

Active on `go-rewrite` branch. See `docs/migration/` for overview, stack rationale, module mapping, and full phase plan.

### Phase G1: Scaffold + Config + API Client
- [x] Create Go module, Makefile (`build`/`lint`/`test`/`test-e2e`), golangci-lint + goreleaser configs, `lazylab version` + `lazylab run` stub, e2e CLI tests
  - **DoD:** `make build` produces `bin/lazylab`; `make lint` / `make test` / `make test-e2e` all clean; `goreleaser release --snapshot --clean` builds three-platform archives.
  - **Testing:** subprocess-level e2e drives `lazylab version`, `lazylab run`, no-subcommand, unknown-subcommand, `--help`; unit tests cover the handler functions and error wrapping.
- [x] Implement `internal/config/` with yaml.v3 + dario.cat/mergo + afero (`XDG_CONFIG_HOME` env + home-dir fallback; adrg/xdg dropped for Python-parity on macOS, see ADR 007)
  - **DoD:** loads `~/.config/lazylab/config.yaml`, applies defaults via mergo, reads token from `LAZYLAB_GITLAB_TOKEN`.
  - **Testing:** unit test with afero in-memory FS covering missing file, partial file, full file, env override, invalid YAML, URL trailing-slash strip, save round-trip (85% coverage).
- [x] Implement `internal/appcontext/AppContext` (package renamed from `context` to avoid shadowing stdlib)
  - **DoD:** constructed in `cmd/lazylab/main.go`, carries config + current project, no globals. GitLab client field added by the gitlab-client task.
  - **Testing:** unit tests assert fields wired, `WithCurrentProject` returns a fresh copy (100% coverage).
- [x] Pick and document (ADR) the Go GitLab client library; implement `internal/gitlab/client.go`
  - **DoD:** thin wrapper with thread-safe client; e2e smoke hitting a fake HTTP server.
  - **Testing:** unit test against `httptest.Server`; ADR committed in `docs/adr/`.
  - **Outcome:** chose `gitlab.com/gitlab-org/api/client-go` (ADR 008). `internal/gitlab.Client` wraps it with URL/token validation and `WithHTTPClient` option for test injection; suite covers validation errors, upstream error wrap, and `httptest.Server` round-trip with `PRIVATE-TOKEN` header assertion.
- [x] Implement `internal/models/` (User, Project, MergeRequest, Pipeline, PipelineJob, PipelineDetail, ApprovalStatus, DiscussionStats, MRDiffFile, MRDiffData + enums)
  - **DoD:** types mirror Python Pydantic fields 1:1; JSON round-trip tested; enum `IsValid`/state predicates added.
  - **Testing:** testify suites per file — round-trip + fixture decode + enum tables (100% coverage).
- [x] Implement `lazylab version` and `lazylab run` subcommands via flaggy
  - **DoD:** both subcommands parse and exit with correct codes; `run` loads config, builds GitLab client, constructs `AppContext`, and exits cleanly (no TUI).
  - **Testing:** unit tests cover run happy-path, missing-config seed, invalid YAML wrap, client-build error wrap, write-error wrap. E2E covers version output, run happy-path with `LAZYLAB_CONFIG`, missing-token non-zero exit, help flag, and unknown subcommand.
- [x] Port file-backed cache (ADR 006) to `internal/cache/`
  - **DoD:** stale-while-revalidate semantics preserved; race-free under `-race`; generic `Do[T]` replaces `@cached(model=...)`; background refresh silent (no TUI event) per user UX requirement — see ADR 009.
  - **Testing:** testify suites covering miss/hit/stale/refresh/dedup/invalidate/InvalidateMR/InvalidateAll/Shutdown/ctx-cancel/disk-round-trip/corrupt-file; all run under `-race`.
  - **Outcome:** `internal/cache/` ships with `cache.Do[T]`, `MakeKey`, `Invalidate`, `InvalidateMR`, `InvalidateAll`, `Shutdown`. Wired into `AppContext.Cache` and `cli.Run` (deferred 2s shutdown). No call site uses it yet — applied to GitLab read functions in Phase G6.

### Phase G2: Repositories Panel
- [x] 3-pane gocui layout + vim-style key bindings in `internal/tui/keys.go`
  - **DoD:** `hjkl`, `g`, `G`, `/`, `[`, `]` registered; focus cycles correctly.
  - **Testing:** e2e drives keys and asserts focused view name.
  - **Outcome:** `internal/tui/` ships layout + central binding table + pure focus cycle; `cli.Run` wires `tui.Run` and surfaces `ErrRequiresTTY` on non-TTY stdout. Integration tests use `gocui.NewGui(NewGuiOpts{Headless: true})` to drive focus transitions in-process (ADR 010). `j/k/g/G/`,`/`,`[`,`]` handlers are registered no-op stubs; real behaviour lands with the widgets in G2-task-2 and G4.
- [x] `views/repos.go` with searchable table + favourites
  - **DoD:** renders projects, search filters in-place, favourite toggle persists.
  - **Testing:** e2e mirrors Python `test_app_launch.py`.
  - **Outcome:** `internal/tui/views/repos.go` renders the project list with ☆/★ icons, vim-style cursor (`j/k/g/G`), substring search (`/` opens an editable `repos_search` pane, Enter submits, Esc cancels), and favourite toggle (`t`) that persists via `Config.Save` and re-sorts favourites first by `LastActivityAt` desc. `internal/gitlab/projects.go` ships the uncached `ListProjects` wrapper with pagination and Python-parity defaults (`membership=true, archived=false, order_by=last_activity_at, sort=desc`). `internal/tui/keymap/` holds the shared `Binding` type + pane-name constants so `internal/tui/views` contributes per-view bindings without importing `internal/tui` (import-cycle break). `internal/appcontext.AppContext` now carries `FS` + `ConfigPath` so the view can persist favourites under tests with afero. Parity gate `tests/e2e/repos_render_test.go` asserts rendering, in-place filter, and YAML persistence end-to-end (see ADR 011).

### Phase G3: Merge Requests List
- [ ] `internal/gitlab/merge_requests.go`: List/Get/Approvals
  - **DoD:** 1:1 feature parity with Python equivalents; errors wrapped with context.
  - **Testing:** unit tests against `httptest.Server`.
- [ ] `views/mrs.go` with status icons + filters (state, mine, reviewer)
  - **DoD:** filter toggles change the table content.
  - **Testing:** e2e toggles each filter and snapshots the table.

### Phase G4: MR Detail Tabs
- [ ] Overview tab
  - **DoD:** author, date, state, branches, conflicts, comment count rendered.
  - **Testing:** unit snapshot + e2e.
- [ ] Diff tab (file tree + side-by-side viewer)
  - **DoD:** file tree navigable, diff colored, ctrl+d/u scroll.
  - **Testing:** unit tests for diff parsing; e2e for tree+diff rendering.
- [ ] Conversation tab
  - **DoD:** threaded discussions with resolve status.
  - **Testing:** e2e for conversation rendering.
- [ ] Pipeline tab + inline job logs
  - **DoD:** stages as blocks, Enter opens log, Esc closes.
  - **Testing:** e2e open/close log.

### Phase G5: MR Actions
- [ ] Close + merge actions with modal confirmation views
  - **DoD:** state guards prevent closed→close; post-action refresh via cache invalidation.
  - **Testing:** e2e for confirm/cancel on both actions.

### Phase G6: Caching
- [ ] Apply `cache.Do[T]` to read-only GitLab functions in `internal/gitlab/*.go`
  - **DoD:** every read function (`ListProjects`, `GetProject`, `ListMergeRequests`, `GetMergeRequest`, `GetMRChanges`, `GetMRApprovals`, `GetLatestPipelineForMR`, `GetPipelineDetail`, `GetJobTrace`) routes through `cache.Do` with the namespace table in ADR 009.
  - **Testing:** unit tests with `-race` verifying concurrent reads deduplicate via the existing cache dedup path; integration test against `httptest.Server` asserting second identical call does NOT hit the server while fresh.
  - **Partial progress (ahead of phase):** `ListProjects` already routes through `cache.Do` — `internal/gitlab/projects.go` gates on `c.cache != nil`, `WithCache(c)` option wires it from `cli.Run`, and `TestListProjects_CachedClient_ReusesResultOnSecondCall` asserts dedup. Remaining read functions still need the same wrapping when G6 proper lands.
- [ ] Wire `ctx.Cache.InvalidateMR(projectID, mrIID)` after close/merge mutations (G5 work references this)
  - **DoD:** after a close/merge, the next read of any of the 7 MR namespaces re-fetches from GitLab.
  - **Testing:** unit test: cache MR, mutate, assert next read calls loader; e2e: close MR and observe list row disappears on next refocus.
- [ ] Decide per-view polling for live status (pipeline candidate) — explicitly NOT a cache-level event
  - **DoD:** if implemented, a pipeline view owns its ticker calling `Do` with a short TTL override or a forced-refresh path; no global `CacheRefreshed` event exists (guarded by ADR 009).
  - **Testing:** view-level test: advance clock past poll interval, assert view re-renders with new data. Absence test: grep `internal/cache/` for `OnRefresh`/`CacheRefreshed`/`chan.*Event` — must be zero matches.

### Phase G7: Polish + Cut-Over
- [ ] Command palette, error handling improvements
  - **DoD:** palette lists registered commands; errors surface as toasts, not crashes.
  - **Testing:** e2e for palette invocation + error path.
- [ ] GoReleaser config producing macOS/Linux binaries + Docker image + release hardening
  - **DoD:** `goreleaser release --snapshot --clean` succeeds locally; release artifacts are cosign-signed, SBOM (SPDX) generated, third-party actions in `release.yml` pinned by commit SHA, `id-token: write` granted to the release job.
  - **Testing:** CI dry-run stage; verify `.sig` + `.sbom.spdx.json` present next to each archive; `cosign verify-blob` succeeds against a snapshot build.
- [ ] Cut-over: merge `go-rewrite` → `master`, delete Python tree, tag release
  - **DoD:** full Python e2e scenarios pass against Go binary; no Python runtime required.
  - **Testing:** green CI + tagged release artefact.
