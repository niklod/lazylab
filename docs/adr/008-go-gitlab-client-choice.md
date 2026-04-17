# 008 — Go GitLab client: `gitlab.com/gitlab-org/api/client-go`

## What

The Go rewrite uses `gitlab.com/gitlab-org/api/client-go` for all GitLab REST
API access, wrapped behind our own `internal/gitlab.Client` struct.

## Why

- Direct functional equivalent of `python-gitlab` (ADR 001): full coverage of
  the endpoints lazylab touches — projects, merge requests, pipelines,
  discussions, approvals, diffs, job traces.
- Official successor to the community `xanzy/go-gitlab` library; the GitLab
  Inc. team took over maintenance and the import path in 2024. Upstream is
  actively maintained against current GitLab releases.
- Handles pagination, authentication (`PRIVATE-TOKEN` / OAuth), retries via
  `hashicorp/go-retryablehttp`, and response decoding. Without it we would
  repeat ~500 lines of URL construction and JSON plumbing that Python's
  wrapper already saved us from.
- Ergonomic option pattern (`ClientOptionFunc`) composes cleanly with a thin
  project wrapper, so we can inject `httptest.Server` URLs and custom
  `*http.Client`s in tests.

We wrap it in `internal/gitlab.Client` rather than exporting the upstream type
directly so that:

1. Callers depend on our domain types (`internal/models`), not upstream DTOs.
2. The client library can be swapped later without touching every call site —
   same decoupling the Python `GitLabClient` provides.
3. Boundary validation (empty URL / token) lives in one place.

## Tradeoffs

- The upstream package name is `gitlab` which collides with our
  `internal/gitlab` package. Resolved by importing upstream as
  `gogitlab "gitlab.com/gitlab-org/api/client-go"` inside our wrapper.
- Transitive deps added: `hashicorp/go-retryablehttp`, `hashicorp/go-cleanhttp`,
  `google/go-querystring`, `golang.org/x/oauth2`, `golang.org/x/time`. All are
  well-known, permissively licensed, and already standard in the GitLab/HashiCorp
  ecosystem.
- Upstream DTOs use pointer-heavy optional fields. Conversion to our
  `internal/models` types lives in endpoint files (`projects.go`, etc.) as
  they land in later phases.

## Alternatives considered

- **`xanzy/go-gitlab`** — the historical canonical library. Now redirects to
  the official repo above. Using it would pin us to an archived import path.
- **Raw `net/http`** — maximum control, but re-implements pagination, auth,
  retries, and response parsing with no offsetting benefit.
- **GraphQL client** — GitLab's GraphQL schema is still partial for the
  endpoints we use (pipeline jobs, MR approvals); the REST client is a
  superset for our scope.
