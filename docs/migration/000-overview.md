# Go Rewrite: Overview

## What

Rewrite LazyLab from Python/Textual to Go 1.25 using `jesseduffield/gocui` on `gdamore/tcell/v2`.

## Why

- Single static binary distribution (goreleaser) — simpler install than Python + `uv` on end-user machines.
- TUI performance: compiled Go + tcell has lower input-latency ceiling than Textual's async compositor.
- Typing and concurrency: native goroutines/channels map cleanly to the existing thread-offloaded GitLab calls.

## Non-goals

- Changing the UX. The Go build must feel identical to the Python build from the user's seat.
- Changing the config file schema or path (`~/.config/gitlab-tui/config.yaml`).
- Changing the CLI entry point name (`lazylab`).

## Strategy

- **Branch layout:** rewrite lives on `go-rewrite`. `master` keeps the Python implementation as the executable spec for parity checks and bugfixes until cut-over.
- **Phased migration:** mirror the Python phase list (see `003-phases.md`). Each phase closes with a feature-parity gate — the same e2e scenario must pass against the Go binary as against the Python binary.
- **Python as reference:** keep `lazylab/` on `go-rewrite` until the corresponding Go feature ships, then delete it phase-by-phase. Do not delete Python code ahead of its Go replacement.
- **Shared infra:** repo-level infra (README, CLAUDE.md, docs/, tasks.md) is shared between branches. Merge organizational commits back to `master` when useful so both branches stay navigable.

## Parity criteria (cut-over gate)

- Every phase's e2e test translated and passing in Go.
- CLI surface identical: `lazylab run`, `lazylab version`, flags, exit codes.
- Config path and schema unchanged.
- Cache path + format compatible, or explicit migration documented.
- Goreleaser build producing macOS/Linux binaries and Docker image.
- CHANGELOG entry for the cut-over release.

## References

- Stack rationale: `001-stack-rationale.md`
- Module mapping: `002-module-mapping.md`
- Phase plan: `003-phases.md`
- Active tasks: `../../tasks.md` (Go Rewrite section)
