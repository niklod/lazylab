# Go Rewrite: Stack Rationale

Source-of-truth for package choices in the Go rewrite. Compressed summary in `~/.claude/projects/-Users-niklod-code-lazylab/memory/stack_rationale_summary.md` (auto-memory).

## TUI and terminal

| Package | Why |
|---------|-----|
| `jesseduffield/gocui` (fork) | Richer layout and view primitives than upstream `jroimartin/gocui`. Proven in `lazygit`. |
| `gdamore/tcell/v2` | Cross-platform terminal I/O. Backend used by gocui. |
| `gookit/color` | High-level ANSI color API. |
| `lucasb-eyer/go-colorful` | Color-space operations for theming/diff highlighting. |
| `rivo/uniseg` | Correct grapheme-cluster width for CJK, emoji, combining marks. |
| `kyokomi/emoji` | Emoji short-code → glyph rendering (GitLab discussions contain `:emoji:`). |

## Git / CLI ergonomics

| Package | Why |
|---------|-----|
| `cli/go-gh/v2` | Reusable CLI primitives (tables, pagers) from the GitHub CLI. |
| `stefanhaller/git-todo-parser` | Rebase-todo parsing if interactive git operations are added. |
| `creack/pty` | PTY allocation for running subprocesses (e.g. `git diff` with pager). |

## Utilities

| Package | Why |
|---------|-----|
| `samber/lo` | `Map`/`Filter`/`Reduce` helpers that are idiomatic in Go 1.21+ and shrink collection boilerplate. |
| `spf13/afero` | FS abstraction. Used to inject an in-memory FS in unit tests for config and cache paths. |
| `golang.org/x/exp` | Access to generics-era helpers (`slices`, `maps`) while we sit on Go 1.25. |

## Configuration and CLI flags

| Package | Why |
|---------|-----|
| `gopkg.in/yaml.v3` | YAML parser for `~/.config/gitlab-tui/config.yaml`. |
| `adrg/xdg` | XDG base-dir lookup (`XDG_CONFIG_HOME`, `XDG_CACHE_HOME`). |
| `integrii/flaggy` | CLI flag parser with subcommand support. Matches `gt run` / `gt version` layout. |

## Search

| Package | Why |
|---------|-----|
| `sahilm/fuzzy` | Fuzzy matching for project/MR search. |
| `fuzzy-patricia` | Trie-backed fuzzy matching for large result sets. |

## Logging

| Package | Why |
|---------|-----|
| `sirupsen/logrus` | Structured logger with levels. Matches the Python `ll` logger's field-based logging style. |
| `aybabtme/humanlog` | Pretty-print logrus JSON during development. |

## Tests

| Package | Why |
|---------|-----|
| `stretchr/testify` | Standard assertion/mocking library. |
| `sasha-s/go-deadlock` | Drop-in `sync.Mutex` replacement that panics on deadlocks. Useful when porting the async cache. |

## Miscellaneous

| Package | Why |
|---------|-----|
| `atotto/clipboard` | Cross-platform clipboard for MR URL / commit SHA copy. |
| `dario.cat/mergo` | Deep-merge user config over defaults. |
| `go-errors/errors` | Errors with stack traces for log context. |

## Build and release

| Tool | Why |
|------|-----|
| Go toolchain + Makefile | Standard. Makefile targets stay the same names as Python era (`build`, `lint`, `test`, `test-e2e`). |
| GoReleaser | Cross-platform binaries (macOS / Linux, amd64 / arm64), Homebrew tap, Docker image. |
| golangci-lint | Aggregate linter. |
| Docker | Containerised build. |

## Rules for additions

- A new dependency requires an ADR entry in `docs/adr/`.
- Before adding a package, check if an existing entry in this table already covers the role. Reuse over sprawl.
- `~/.claude/projects/-Users-niklod-code-lazylab/memory/stack_rationale_summary.md` is regenerated (or manually kept in sync) whenever this file changes.
