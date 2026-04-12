# 005: MR Close & Merge Actions

## Decision

MR close and merge actions use modal confirmation screens triggered by keybindings on `MRContainer`.

## Key Choices

### Keybinding placement: MRContainer (not detail tabs)

Actions live on the MR list container because:
- `MRContainer` owns `get_selected_mr()` — the data source for the action
- Natural "select and act" flow (browse list, press key)
- Consistent with `ReposContainer` owning `TOGGLE_FAVORITE`
- Detail tabs are read-only views; mutations there would be surprising

Keys: `x` (close — "x out"), `M` (merge — uppercase for deliberate destructive action).

### Modal pattern: push_screen with callback

Used `ModalScreen[T]` with `push_screen(screen, callback)` instead of `push_screen_wait()`. The callback is synchronous, so the selected MR is stored in `_pending_action_mr` before pushing the modal. No race condition since the modal blocks user interaction.

The `@work`-decorated execute methods handle the async API call from the sync callback.

### Post-action refresh

After close/merge:
1. `MRContainer` refreshes its own list via `load_merge_requests()`
2. `MRActionCompleted` message bubbles up to `LazyLabMainScreen`
3. Parent reloads detail tabs with updated MR state

### State guards

Both actions check `mr.state == OPENED` before showing the modal. Non-opened MRs trigger a warning notification instead.

## Alternatives Considered

- **Inline confirmation (no modal):** Simpler but no room for merge options (delete branch, pipeline gate). Modal needed for merge checkboxes.
- **Actions on detail tabs:** Rejected — detail tabs don't own the MR selection, would need message passing back to container.
