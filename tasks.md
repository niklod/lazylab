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
- [x] `internal/gitlab/merge_requests.go`: List/Get/Approvals + `GetCurrentUser`
  - **DoD:** 1:1 feature parity with Python equivalents; errors wrapped with context; cache namespaces `mr_list`, `mr`, `mr_approvals`, `current_user` routed through `cache.Do`.
  - **Testing:** unit tests against `httptest.Server` cover pagination, author/reviewer filter URL args, upstream error wrap, input validation, cached dedup, approvals `approved_by.user` decoding, and current user mapping.
  - **Outcome:** thin wrappers over `gogitlab.MergeRequestsService` + `MergeRequestApprovalsService`; `ListMergeRequestsOptions` mirrors Python (`state`, `author_id`, `reviewer_id`). Optional filters skipped in `MakeKey` so cache keys match Python's None-skipping.
- [ ] G3 follow-ups surfaced by parallel code review (not blocking — track and revisit when a third caller appears or the next phase touches the site)
  - **DoD:** decide per-item keep/defer; each item either filed as its own ADR+task or explicitly declined here.
  - **Testing:** race test for filter-cycle path (already added; see `TestMRsViewSuite`).
  - `MakeKey` nil-skip footgun: optional-arg callers must tag their slots (see `intPtrArg`). Future fix — emit a sentinel for nil in `MakeKey` so positional identity is preserved without caller convention. Cache-layer change, schedule with G6.
  - `handleCycleState`/`handleCycleOwner` use `context.Background()`. App-scoped cancellable context plumbing lands with G7 polish.
  - `SearchableList[T]` abstraction: decline until G4 introduces a third searchable list (diff file tree). Two call sites is below the project's "three similar lines" threshold for extraction.
- [x] `views/mrs.go` with status icons + filters (state, mine, reviewer)
  - **DoD:** filter toggles rotate state (opened → merged → closed → all) and owner (all → mine → reviewer → all); each toggle re-fetches; search `/` filters in-place on title+author; Enter on repos pane drives MR load.
  - **Testing:** unit suite (13 tests) for filter rotation, fetch happy-path, error wrapping, search, cursor, stale-load guard; e2e suite (4 tests) covers project-selection → MR render, state cycle, owner cycle (asserts `author_id=77` on the wire), and search.
  - **Outcome:** view owns its own keymap + `mrs_search` ephemeral pane; `views.Views.selectProjectForMRs` is the cross-view wiring so neither view imports the other.

### Phase G4: MR Detail Tabs
- [x] Overview tab
  - **DoD:** author, date, state, branches, conflicts, comment count rendered.
  - **Testing:** unit snapshot + e2e.
  - **Outcome:** `internal/tui/views/detail.go` ships `DetailView` — renderer of `*models.MergeRequest` with title, author, Python-parity date (`2006-01-02 15:04`), state letter+name, `source → target` branches, conflicts, and a comment count with resolved-thread breakdown (`N` when no resolvable discussions, `N (X/Y resolved)` otherwise — mirrors Python `_comments_text`); empty-state hint when no MR is selected. Overview string is cached on SetMR and re-built when discussion stats arrive (MRsView.bannerLine parity pattern) so the layout tick doesn't allocate a fresh builder per redraw. Detail-pane `Wrap` is set once at firstCreate in `renderPanes`, not per tick. Discussion stats load asynchronously: SetMR commits MR + nil-stats overview, then a goroutine fetches via `Client.GetMRDiscussionStats` and `g.Update`s the cache — cancelled by `statsSeq` generation counter when the user switches MR mid-flight. `SetMRSync` is the test-only inline variant. `Views.selectMRForDetail` handles `Enter` on the mrs pane, reads `MRsView.CurrentProject` so the fetch has a project ID, and populates detail without shifting focus (detail pane has no bindings yet — deferred to diff/conversation/pipeline sub-tasks; global `h`/`l` still cycles). `internal/gitlab/merge_requests.go` grows `GetMRDiscussionStats` — paginated `Discussions.ListMergeRequestDiscussions`, aggregates per Python rule (discussion counts as resolvable iff its first note is resolvable; counts as resolved iff every resolvable note is resolved), cached under `mr_discussions`. Unit suite for views (13 tests) covers nil-MR hint, all-field rendering, conflict branch, SetMR replacement, SetMR(nil) clears content, state-letter table (opened/merged/closed/unknown), `selectMRForDetail` nil-guard branches, `commentsText` with/without stats, SetMRSync fetch + render + stale-seq guard. Unit suite for gitlab (3 new tests) covers discussion-stats aggregation, upstream-error wrap, input validation. E2E suite (5 tests) drives repos load → mrs load → Enter-on-mr → overview assertion, focus stays on mrs, cursor-down + Enter replaces content, Enter on empty MR list preserves the hint, SetMRSync renders `N (X/Y resolved)` end-to-end.
  - **Follow-ups (surfaced by code review, deferred)**: (1) E2E harness extraction — `tests/e2e/{mrs_render,detail_overview}_test.go` share ~60 lines of httptest+afero+gocui setup. Below the "three similar lines" threshold today; extract when the diff-tab e2e (G4 next sub-task) lands. (2) Layout's per-pane render dispatch (3× `if v.X != nil` blocks) will scale poorly when tabs land; revisit with a small `Renderer` interface then.
- [x] Overview tab — reviewers line
  - **DoD:** Overview renders a `Reviewers: @a, @b` line directly below Author when the MR has reviewers; the line is omitted entirely when the reviewers list is empty.
  - **Testing:** unit tests for `reviewersText` (empty → "", single, multiple comma-joined) + render tests for with/without reviewers. E2E: `Feature Alpha` fixture grows two reviewers and the existing overview test asserts `"Reviewers: @carol, @dave"`; the `Bugfix Beta` fixture (no reviewers) asserts the line is absent.
  - **Outcome:** `internal/models/merge_request.go` grows `Reviewers []User`; `internal/gitlab/merge_requests.go` adds `domainUsersFromBasic` and maps `BasicMergeRequest.Reviewers` in `toDomainMergeRequestFromBasic` — payload already rides with the list/get MR calls, no extra fetch. `internal/tui/views/detail.go` adds `reviewersText(users []models.User) string` (empty-in, empty-out; `@user, @user…` otherwise) and renders it conditionally after the Author line inside `renderOverview` so the line vanishes when there are no reviewers.
- [x] Overview tab — approvals line
  - **DoD:** Overview renders `Approvals: N/M approvals received` — green ✓ when all required approvals collected, red ✗ when missing, dim `no approvals required` when the project has no approval rules, dim `loading…` until fetch returns.
  - **Testing:** unit tests for `approvalsText` across all four branches + `SetMRSync` fetch+render; E2E: three scenarios (no approvals required, 0/1 missing → red ✗, 2/2 received → green ✓).
  - **Outcome:** `internal/tui/views/detail.go` grows `approvals *models.ApprovalStatus` + `approvalsSeq` state (reset on `commitMR`, guarded by seq in `applyApprovals` mirroring `applyStats`), `fetchApprovals` goroutine kicked off from `SetMR` parallel to stats/diff, `SetMRSync` fetches inline for tests. `renderOverview` grows a fourth arg and re-renders the cache on every async arrival. `approvalsText()` — nil → dim "loading…"; `ApprovalsRequired == 0` → dim "no approvals required" (avoids misleading 0/0 ✗ on projects without approval rules); otherwise `N/M approvals received` coloured green ✓ when `Approved == true`, red ✗ when not. Client `Client.GetMRApprovals` existed since phase G3; this sub-task wires it into the UI. Unit tests: 9 new (approvalsText branches, stale-seq guard, SetMRSync red/green render paths, loading-hint render). E2E: three new tests in `tests/e2e/detail_overview_test.go`; default handler now serves `/approvals` with a zero-required fixture so existing tests don't regress.
- [x] Diff tab (file tree + side-by-side viewer)
  - **DoD:** file tree navigable, diff colored, ctrl+d/u scroll.
  - **Testing:** unit tests for diff parsing; e2e for tree+diff rendering.
  - **Outcome:** `internal/gitlab/merge_requests.go` grows `GetMRChanges` (routed through `cache.Do` with namespace `mr_changes`), backed by `client-go` `ListMergeRequestDiffs` (paginated replacement for the deprecated `/changes` endpoint — see ADR 012). `internal/tui/views/detail.go` grows a `DetailTab` enum + `currentTab`/`pendingFocus` state + `SetTab`/`SetTabSync`/`SelectDiffFile`; the overview body still caches via `renderOverview`, diff body is delegated to two new widgets in `internal/tui/views/diff_tree.go` (`DiffTreeView` — flat tree, status-letter icons, cursor-skip-over-dirs, placeCursor-based highlighting mirroring repos/mrs) and `internal/tui/views/diff_content.go` (`DiffContentView` + pure `renderDiffMarkup` function — ANSI bold/cyan/green/red per Python parity rules). `internal/tui/layout.go` mounts the two diff sub-panes inside the detail rect (`manageDiffSubpanes`, ephemeral like `manageSearchPane`) and consumes a `pendingFocus` token from DetailView after mount so `[`/`]` tab-cycle can park focus on a pane that didn't exist until that tick. `[`/`]` bindings are duplicated across all three detail-family views (ViewDetail + diff tree + diff content) because gocui dispatches by focused view name. `internal/tui/names.go` swaps the static `focusOrder` to a `focusOrderFn` pointer and adds `SetFocusOrderProvider` so `focusNext`/`focusPrev` extend the cycle to `[detail_diff_tree, detail_diff_content]` when the Diff tab is active. Unit tests: `diff_content_test.go` (8 tests — rendering, scroll clamp, scroll-to-top, empty/loading/error states), `diff_tree_test.go` (9 tests — directory grouping, cursor skip, status-letter table, empty states), `detail_test.go` grows 7 tests (tab cycle, diff fetch + tree population, empty-diff hint, error wrap, cross-MR reset). E2E: `tests/e2e/mr_diff_test.go` (4 tests — tree+content render, j + Enter swaps content, ctrl+d scrolls content, `[`/`]` cycles tab + focuses correct pane). Persistence contract: ADR 012 covers sub-pane strategy, bind-per-view rationale, and `ListMergeRequestDiffs` choice.
- [ ] Conversation tab
  - **DoD:** threaded discussions with resolve status.
  - **Testing:** e2e for conversation rendering.
- [x] Design-palette theme + Overview parity
  - **DoD:** panel-frame focus + selection colours come from the design palette (`#d97757` accent); Overview tab matches `design/project/wireframes/overview.js` (title, subtitle, 12-char key column, colored state dot, pipeline row populated via prefetch, relative "Updated" time, dashed rule + Description block); tab bar shows dim brackets/pipes with accent-bold current tab.
  - **Testing:** unit + e2e suites updated; `make build && make lint && make test && make test-e2e` green.
  - **Outcome:** New package `internal/tui/theme` (theme.go + timeago.go + tests) exposes truecolor SGR strings (`FgOK/Warn/Err/Info/Accent/Merged/Draft`) and `gocui.Attribute` accent for tcell-side frames/selections, plus a `Relative(t, now)` helper. `internal/tui/views/ansi.go` redirects legacy `ansiRed/Green/Yellow/Cyan/Reset/Bold/Dim` identifiers at the theme tokens so `diff_content.go`, `pipeline_log.go`, and `pipeline_stages.go` pick up the design colours without source changes. `detail.go` rewritten: tab bar uses `dim[…]` brackets + `accent bold` current tab, overview uses fixed 12-char key column with `writeRow` helper, state renders as `theme.Dot(color) + word` (ok/merged/err/draft), new `Pipeline` row (prefetched in `SetMR` same pattern as diff) shows `● status  #ID · duration` with pipelineLoaded/loading/no-pipeline tri-state, `Updated` uses `theme.Relative`, description block appears below a dashed rule when non-empty. Selection/frame colour sites migrated in `mrs.go`, `repos.go`, `diff_tree.go`, `pipeline_stages.go`, `layout.go`. Tests updated: new `theme_test.go`, `timeago_test.go`, `detail_test.go` reworked for new fields + `sgrPrefix` helper (gocui re-serialises SGR with trailing `;`), `app_test.go`+`layout_test.go` swapped to `theme.ColorAccent`, `tests/e2e/detail_overview_test.go` fixtures + assertions updated (substring-based, `[]` pipeline handler added). ADR `docs/adr/015-go-theme-palette.md` records palette centralisation + truecolor-via-OutputTrue decision.

- [x] Pipeline tab + inline job logs
  - **DoD:** stages as blocks, Enter opens log, Esc closes.
  - **Testing:** e2e open/close log.
  - **Outcome:** `internal/gitlab/pipelines.go` adds `GetMRPipelineDetail(ctx, projectID, iid)` (mr → latest pipeline → jobs, single cache entry `mr_pipeline`) and `GetJobTrace(ctx, projectID, jobID)` (namespace `job_trace`). Widget split: `internal/tui/views/pipeline_stages.go` (`PipelineStagesView` — flat `stageRow[]` with bold stage headers + indented jobs, cursor skips headers mirroring `DiffTreeView`, ANSI-coloured status icons, `formatJobDuration` for Python-parity `Xs` / `Ym Xs`) and `internal/tui/views/pipeline_log.go` (`JobLogView` — header + ANSI-passthrough trace body; gocui `OutputTrue` + the same pane-buffer path the Diff viewer uses makes a custom ANSI parser unnecessary). `DetailView` grows `pipelineDetail/pipelineSeq/pipelineLoading/pipelineErr` + `logOpen/logJob/logSeq/logLoading/logErr`, new public `PipelineStages()/JobLog()/LogOpen()/PipelineDetailSnapshot()`, async `fetchPipelineAsync` + inline `SetTabSync` Pipeline branch, `OpenJobLog`/`OpenJobLogSync`/`CloseJobLog` (stale-seq guarded mirroring `applyDiff`/`applyStats`), `focusTargetForTab(DetailTabPipeline) == ViewDetailPipelineStages`. Layout: `internal/tui/layout.go` `managePipelineSubpanes` mounts exactly one of stages/log inside the detail rect with strict **mount-before-delete** ordering (gocui `DeleteView` does not clear the current-view pointer, so the incoming pane must exist before the outgoing one is removed). `FocusOrder` returns `[repos, mrs, stages]` when Pipeline tab active + log closed, `[repos, mrs, log]` when open; detail-family tab-cycle (`[`/`]`) is bound on both new views so cycling out of Pipeline from the child pane works. Bindings (`views.go`): stages → j/k/↑/↓/g/G/Enter; log → j/k/↑/↓/Ctrl+D/Ctrl+U/Esc. Unit tests: `pipelines_test.go` (6 — pagination, nil pipeline, error wrap, input validation, trace fetch, cache dedup), `pipeline_stages_test.go` (8 suite + 2 standalone — grouping preserves API order, cursor skips headers, status icon table, duration format table, render branches), `pipeline_log_test.go` (6 — header + body, empty-trace placeholder, ANSI passthrough, loading/error, scroll clamp, scroll-to-top), `detail_test.go` grows 8 (SetTabSync Pipeline populates stages, empty pipeline, error wrap, stale-seq guard, cross-MR reset, OpenJobLogSync populates + marks open, CloseJobLog resets + requests stages focus, close-when-not-open no-op). E2E `tests/e2e/mr_pipeline_test.go` drives load → Pipeline tab → assert stages + job names + stage headers; j then Enter on the failing job → asserts log buffer contains `step 2 FAILED` + stages unmounted; Esc → asserts log unmounted, stages remounted + focused; tab-cycle from pipeline pane reaches Conversation. ADR `docs/adr/014-go-pipeline-tab.md` records the flat-listing choice, log-replaces-stages modal, single cache entry, ANSI passthrough.

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
- [x] Repos & MRs list design parity — match `design/project/wireframes/layout.js` for icons, colours, header, alignment, and empty/loading copy
  - **DoD:** Repos pane shows accent-coloured `★` only for favourites (no glyph for non-favs), right-aligned dim relative timestamp, `[1] Repositories · N/M` header. MRs pane shows coloured state glyphs (●/◐/✓/✕), draft detection by title prefix (`Draft:` / `[Draft]` / `[WIP]` / `WIP:`), `<icon> !IID title @author` order with right-aligned dim author, `[2] Merge Requests · state:X · owner:Y · N/M` header. Both panes use white-on-accent selection (`theme.ColorSelectionFg`) and switch to instructional empty-state copy from `design/project/wireframes/states.js` (first-run vs filter-miss for repos; live filter values + S/O/R hint for MRs).
  - **Testing:** `make build && make lint && make test && make test-e2e`. Unit: `internal/tui/views/row_format_test.go` (column alignment, ellipsis truncation, grapheme-aware width), `internal/tui/views/repos_test.go::TestRender_HeaderAndFavouriteIcon_MatchDesign` + empty-state cases, `internal/tui/views/mrs_test.go::TestRender_StateGlyphs_ColouredPerDesign` + empty-state case, `internal/models/merge_request_test.go::TestMergeRequest_IsDraft`. E2E: updated `tests/e2e/repos_render_test.go` and `tests/e2e/mrs_render_test.go` assert on the new header / icon shape.
- [ ] Command palette, error handling improvements
  - **DoD:** palette lists registered commands; errors surface as toasts, not crashes.
  - **Testing:** e2e for palette invocation + error path.
- [ ] GoReleaser config producing macOS/Linux binaries + Docker image + release hardening
  - **DoD:** `goreleaser release --snapshot --clean` succeeds locally; release artifacts are cosign-signed, SBOM (SPDX) generated, third-party actions in `release.yml` pinned by commit SHA, `id-token: write` granted to the release job.
  - **Testing:** CI dry-run stage; verify `.sig` + `.sbom.spdx.json` present next to each archive; `cosign verify-blob` succeeds against a snapshot build.
- [ ] Cut-over: merge `go-rewrite` → `master`, delete Python tree, tag release
  - **DoD:** full Python e2e scenarios pass against Go binary; no Python runtime required.
  - **Testing:** green CI + tagged release artefact.
