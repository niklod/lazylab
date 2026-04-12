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

- [x] Rename CLI command from `lazylab` to `gt`, config path to `~/.config/gitlab-tui/`
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
