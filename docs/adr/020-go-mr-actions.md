# 020: Go MR Actions (Close + Merge)

## Decision

Phase G5 ports Python's MR close + merge mutations to the gocui TUI. The action flow — state-guarded key → centered confirmation modal → async mutation → cache invalidation → list + detail refresh — mirrors ADR 005 where practical but diverges where the terminal runtime demands it.

## Key Choices

### Mutation layer: self-invalidating functions

`internal/gitlab/mr_actions.go` owns `CloseMergeRequest` and `AcceptMergeRequest`. Both end with `c.InvalidateMR(projectID, iid)` before returning the mapped domain model, matching Python's `api_cache.invalidate_mr(...)` call inside `close_merge_request` / `merge_merge_request`. The caller cannot forget to invalidate; the consequence — a second GET for the refreshed MR state — is cheap and deterministic.

`AcceptOptions` exposes `Squash` and `ShouldRemoveSourceBranch` only. The deprecated `MergeWhenPipelineSucceeds` field is not wired (see "Merge toggles" below); the current SDK field for that behaviour is `AutoMerge` (see `gitlab.com/gitlab-org/api/client-go@v1.46.0/merge_requests.go:767`). Future work that adds an auto-merge toggle should set `AutoMerge`, not the deprecated field.

### Modal pattern: centered sub-pane, not `push_screen`

Textual's `ModalScreen` + `push_screen_wait` has no gocui analogue. Instead, a single widget `MRActionModal` (in `mr_action_modal.go`) keeps the two variants behind a `ModalKind` enum. The layout package mounts/deletes a centered sub-pane named `ViewMRActionsModal` driven off `IsActive()` — the same self-healing pattern used for the search inputs (`manageSearchPane`).

Focus is trapped by a `FocusOrder` override: while the modal is active, the returned slice is `[ViewMRActionsModal]` alone. Enter/Esc/d/s bindings land on that view, so they cannot leak through to the MRs pane.

### Keybindings on the MRs pane

`x` (close, "x out") and `M` (merge, uppercase to demand intent) are bound on `ViewMRs`, preserving Python's rationale from ADR 005: the MRs container owns `SelectedMR()` and a natural "browse → act" flow. The detail pane stays read-only.

### Merge toggles: wireframe parity, Python divergence

The Python UI exposed `Delete source branch` + `Merge when pipeline succeeds`. The `design/wireframes/modals.js` mock exposes `Squash commits` + `Delete source branch` with key hints `s`/`d`. This phase takes the wireframe's toggle set because:

1. `design_parity.md` memory policy: consult wireframes before building UI; do not improvise.
2. Squash is a hard-to-reach action in the web UI; surfacing it in the keyboard flow matches the LazyLab value proposition.
3. Merge-when-pipeline-succeeds semantics moved under `AutoMerge` in recent GitLab versions and overlap with merge trains; dropping it now avoids shipping a subtly-different knob. A future phase can re-add an auto-merge toggle bound to `AutoMerge`.

Delete-source-branch defaults on (the common workflow); squash defaults off (opt-in per-MR decision).

### State guards and transient status

Both `x` and `M` gate the modal on `mr.IsOpen()` (existing domain method). Non-opened MRs trigger a dim toast on the MRs pane header (`MRsView.SetTransientStatus`) — mirrors Python's `app.notify(severity="warning")` without requiring global notification infra.

The same transient-status line doubles as the post-action outcome (`Closed !N`, `Merged !N`). Auto-clears on the next `SetProject`.

### Confirm path: async with busy flag

The confirm handler flips `MRActionModal.SetBusy(true)` and hands off to a goroutine (`runCloseAction` / `runMergeAction`). Enter is a no-op while `Busy()` so double-Enter cannot stack two in-flight requests. Esc is also ignored while busy — mutations are short (typically <1s) and cancelling mid-flight could desync the cache state (the invalidate call happens after the HTTP response).

### Post-action refresh

On success:

1. `MRActionModal.Close()` — flips the pane out of existence on the next tick.
2. `MRsView.SetProject(...)` — refetches the list (cache was invalidated on the server-facing side by the mutation function).
3. `DetailView.SetMR(...)` — repaints overview + kicks off the standard stats/approvals/diff/pipeline refetch chain.
4. `MRsView.SetTransientStatus("Closed !N" / "Merged !N")` — user-visible confirmation.

On error, the modal stays open, `SetErr` records the message, and the user can retry or Esc out.

## Alternatives Considered

- **Global notification overlay à la Textual's `app.notify`** — rejected: only one consumer in G5, and the MRs header was already a natural home for single-line toasts. Can be extracted later if Repos/Detail gain similar needs.
- **Binding actions on the detail pane** — rejected for the same reason as ADR 005: detail is read-only, MRs owns selection.
- **Per-kind modal structs** — rejected; the shared field set is nearly identical, and the toggle methods no-op cleanly on the non-applicable kind.
- **Hard-dismiss on error** — rejected; forcing the user to re-press `M` after a transient failure (pipeline not yet green, permission check) is hostile.

## Files

- `internal/gitlab/mr_actions.go` — `CloseMergeRequest`, `AcceptMergeRequest`, `AcceptOptions`
- `internal/tui/views/mr_action_modal.go` — widget
- `internal/tui/views/mr_actions.go` — open/confirm/cancel handlers
- `internal/tui/views/mrs.go` — `SetTransientStatus`
- `internal/tui/layout.go` — `manageMRActionsModal`
- `internal/tui/views/views.go` — `Views.ActionsModal`, `FocusOrder` override
- `internal/tui/keymap/binding.go` — `ViewMRActionsModal`
