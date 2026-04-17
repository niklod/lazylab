# API Client: python-gitlab

## Decision

Use `python-gitlab` library wrapped in an async adapter.

## Why

- Full GitLab API coverage (projects, MRs, pipelines, discussions, approvals, diffs)
- Handles pagination, authentication, API versioning automatically
- Reduces boilerplate vs raw httpx (500+ lines saved)

## Tradeoffs

- python-gitlab is synchronous — requires thread offloading via `anyio.to_thread.run_sync`
- Heavier dependency than raw httpx
- Wrapped in our own `GitLabClient` class for decoupling — backend swap is straightforward

## Alternatives Considered

- **httpx directly**: More control, lighter, but requires writing all URL construction and response parsing manually
