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

## Future Phases

- [ ] Phase 5: Diff tab — built-in viewer + optional external pager
  - **DoD:** Diff tab shows file-based diff with syntax highlighting
  - **Testing:** E2E test for diff rendering
- [ ] Phase 6: Conversation tab — GitLab discussions/notes
  - **DoD:** Threaded comments display with resolve status
  - **Testing:** E2E test for conversation rendering
- [ ] Phase 7: Pipeline tab — stage visualization, job logs
  - **DoD:** Pipeline stages shown as blocks with job status
  - **Testing:** E2E test for pipeline visualization
- [ ] Phase 8: MR actions — create, close, merge, edit
  - **DoD:** All CRUD actions work via modal screens
  - **Testing:** E2E tests for each action
- [ ] Phase 9: Polish — command palette, error handling, caching
  - **DoD:** Production-quality UX
  - **Testing:** Full test suite >80% coverage
