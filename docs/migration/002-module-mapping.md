# Go Rewrite: Module Mapping

Mapping from the Python package tree to the target Go layout. Used to make "where does X go" decisions mechanical.

## Target Go layout

```
cmd/
  lazylab/
    main.go              # entry point, registers flaggy subcommands
internal/
  cli/                   # CLI command handlers (run, version)
  config/                # YAML config loading, XDG paths, defaults + mergo merge
  context/               # global AppContext (config, gitlab client, current project)
  gitlab/                # GitLab API client and endpoint wrappers
    client.go
    merge_requests.go
    pipelines.go
    projects.go
    users.go
  models/                # domain types (Project, MergeRequest, Pipeline, User, Diff...)
  cache/                 # file-backed + in-memory cache with stale-while-revalidate
  tui/                   # gocui views, layout, key bindings
    app.go
    keys.go
    views/
      repos.go
      mrs.go
      mr_overview.go
      mr_diff.go
      mr_pipeline.go
      mr_conversation.go
      info.go
  messages/              # typed events passed between views (RepoSelected, MRSelected, ...)
  logging/               # logrus configuration
pkg/                     # (empty unless a package becomes safe to export)
```

## Package-by-package map

| Python (master) | Go (`go-rewrite`) | Notes |
|-----------------|-------------------|-------|
| `lazylab/__main__.py` | `cmd/lazylab/main.go` | Entry point. |
| `lazylab/cli.py` (Click) | `internal/cli/` + `cmd/lazylab/main.go` (flaggy) | `gt run`, `gt version` subcommands. |
| `lazylab/version.py` | `internal/cli/version.go` or `//go:embed VERSION` | Single source of truth wired to goreleaser ldflags. |
| `lazylab/lib/config.py` | `internal/config/` | YAML + XDG + mergo defaults. |
| `lazylab/lib/context.py` | `internal/appcontext/` | `AppContext` struct, not a global — injected explicitly. Package named `appcontext` to avoid shadowing stdlib `context`. |
| `lazylab/lib/constants.py` | `internal/models/enums.go` (MR state, pipeline status) + per-view constants | Split: domain enums → `models`, UI symbols → view that uses them. |
| `lazylab/lib/logging.py` | `internal/logging/` | logrus setup, humanlog hook in dev. |
| `lazylab/lib/messages.py` | `internal/messages/` | Typed event structs; dispatch via channel on the AppContext. |
| `lazylab/lib/bindings.py` | `internal/tui/keys.go` | gocui key-binding registration helpers. |
| `lazylab/lib/cache.py` | `internal/cache/` | Port the async-cache + stale-while-revalidate semantics from ADR 006 using goroutines. Generic `Do[T]` + silent background refresh (no TUI event) — see ADR 009. |
| `lazylab/lib/gitlab/client.py` | `internal/gitlab/client.go` | Thin wrapper over `gitlab.com/gitlab-org/api/client-go`. See ADR 008. |
| `lazylab/lib/gitlab/projects.py` | `internal/gitlab/projects.go` | 1:1 function mapping. |
| `lazylab/lib/gitlab/merge_requests.py` | `internal/gitlab/merge_requests.go` | 1:1. |
| `lazylab/lib/gitlab/pipelines.py` | `internal/gitlab/pipelines.go` | 1:1. |
| `lazylab/lib/gitlab/users.py` | `internal/gitlab/users.go` | 1:1. |
| `lazylab/models/gitlab.py` | `internal/models/*.go` | One Go file per domain type (`project.go`, `merge_request.go`, ...). |
| `lazylab/ui/app.py` | `internal/tui/app.go` | gocui `Gui` setup and layout. |
| `lazylab/ui/screens/primary.py` | `internal/tui/layout.go` | 3-pane layout. |
| `lazylab/ui/widgets/common.py` | `internal/tui/views/common.go` | Shared view helpers (searchable table). |
| `lazylab/ui/widgets/info.py` | `internal/tui/views/info.go` | Welcome tab. |
| `lazylab/ui/widgets/repositories.py` | `internal/tui/views/repos.go` | Project list + favourites. |
| `lazylab/ui/widgets/merge_requests.py` | `internal/tui/views/mrs.go` + sibling files per tab | Split tabs into own files per the small-files rule. |
| `lazylab/ui/widgets/mr_diff.py` | `internal/tui/views/mr_diff.go` | File tree + diff renderer. |
| `tests/unit/*.py` | `internal/*/*_test.go` | Co-located `_test.go` files. |
| `tests/e2e/*.py` | `tests/e2e/*_test.go` | Kept as a top-level integration tree. |
| `pyproject.toml` | `go.mod` + `go.sum` | — |
| `Makefile` | `Makefile` (updated targets, same names) | `build`/`lint`/`test`/`test-e2e` rewired to Go tools. |

## Rules

- No cyclic imports between `internal/gitlab`, `internal/tui`, `internal/context`. Messages flow through `internal/messages`.
- No global singletons. `AppContext` is constructed in `cmd/lazylab/main.go` and passed explicitly.
- Pure domain types (`internal/models`) import nothing from `internal/gitlab` or `internal/tui`.
- A new Go file that cannot be placed using the table above requires a new ADR or an extension of this table.
