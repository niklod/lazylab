package views

import (
	"context"
	"fmt"

	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/gitlab"
	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
)

// openCloseModal handles the `x` key. Registered on the MRs pane and every
// detail-family view so the user can close from anywhere the MR is in
// context (Overview / Diff / Conversation / Pipeline / Log). State-guards
// against non-opened MRs and hands focus to the modal sub-pane.
//
//nolint:contextcheck // gocui handler signature is fixed; actions use context.Background by design.
func (v *Views) openCloseModal(g *gocui.Gui, source *gocui.View) error {
	return v.openMRActionModal(g, source, ModalClose, "close")
}

// openMergeModal handles `M`. Same guards as close, same multi-view
// registration.
//
//nolint:contextcheck // gocui handler signature is fixed; actions use context.Background by design.
func (v *Views) openMergeModal(g *gocui.Gui, source *gocui.View) error {
	return v.openMRActionModal(g, source, ModalMerge, "merge")
}

func (v *Views) openMRActionModal(g *gocui.Gui, source *gocui.View, kind ModalKind, verb string) error {
	if v.MRs == nil || v.ActionsModal == nil {
		return nil
	}
	sourceName := ""
	if source != nil {
		sourceName = source.Name()
	}
	mr := v.resolveActionMR(sourceName)
	if mr == nil {
		return nil
	}
	if !mr.IsOpen() {
		// Toast stays on the MRs pane regardless of origin — it owns the
		// single transient-status strip in the footer area.
		v.MRs.SetTransientStatus(fmt.Sprintf("Cannot %s: MR !%d is %s", verb, mr.IID, mr.State))

		return nil
	}

	v.ActionsModal.Open(kind, mr)
	v.actionOriginView = sourceName
	if g == nil {
		return nil
	}
	// The pane does not exist until the next layout tick mounts it. Queue
	// a focus shift via g.Update so SetCurrentView sees the mounted sub-pane.
	g.Update(func(gg *gocui.Gui) error {
		_, err := gg.SetCurrentView(keymap.ViewMRActionsModal)
		if err == nil {
			return nil
		}
		// ErrUnknownView can fire on the very first tick after Open if
		// the layout hasn't run yet. The FocusOrder override will pick
		// up the pane once it mounts, so swallow.
		if goerrors.Is(err, gocui.ErrUnknownView) {
			return nil
		}

		return fmt.Errorf("focus mr action modal: %w", err)
	})

	return nil
}

// resolveActionMR picks the MR the action should operate on based on the
// originating pane. Detail-family panes prefer the MR the user is looking at
// (DetailView.CurrentMR) — the MRs-pane cursor may have drifted after the
// user Entered detail and browsed a neighbour. MRs pane uses its own cursor.
// Falls back to the MRs cursor if DetailView is empty so a stale detail view
// cannot swallow the action.
func (v *Views) resolveActionMR(sourceName string) *models.MergeRequest {
	if keymap.IsDetailFamily(sourceName) && v.Detail != nil {
		if mr := v.Detail.CurrentMR(); mr != nil {
			return mr
		}
	}

	return v.MRs.SelectedMR()
}

// confirmMRAction runs the mutation. Guarded by the modal's Busy flag so a
// double-Enter cannot stack two in-flight requests.
//
//nolint:contextcheck // gocui handler signature is fixed; actions use context.Background by design.
func (v *Views) confirmMRAction(_ *gocui.Gui, _ *gocui.View) error {
	if v.ActionsModal == nil {
		return nil
	}
	snap := v.ActionsModal.Snapshot()
	if !snap.Active || snap.MR == nil || snap.Busy {
		return nil
	}
	project := v.currentProject()
	if project == nil {
		return nil
	}

	v.ActionsModal.SetBusy(true)
	ctx := context.Background()

	switch snap.Kind {
	case ModalClose:
		go v.runCloseAction(ctx, project, snap.MR)
	case ModalMerge:
		go v.runMergeAction(ctx, project, snap.MR, gitlab.AcceptOptions{
			Squash:                   snap.Squash,
			ShouldRemoveSourceBranch: snap.DeleteBranch,
		})
	}

	return nil
}

// cancelMRAction dismisses the modal. Esc is ignored while Busy because
// mutations are short and cancelling mid-flight could desync cache state.
func (v *Views) cancelMRAction(_ *gocui.Gui, _ *gocui.View) error {
	if v.ActionsModal == nil {
		return nil
	}
	if v.ActionsModal.Busy() {
		return nil
	}
	v.ActionsModal.Close()
	v.restoreFocusAfterModal()

	return nil
}

func (v *Views) runCloseAction(ctx context.Context, project *models.Project, mr *models.MergeRequest) {
	if v.app == nil || v.app.GitLab == nil {
		// Unreachable in production (app wires the client at startup) but a
		// unit test building a bare Views{} could hit this. Surface the
		// failure instead of silently wedging the modal on busy=true with
		// Esc ignored by the busy-guard.
		v.ActionsModal.SetErr("internal: gitlab client unavailable")
		v.triggerRedraw()

		return
	}
	updated, err := v.app.GitLab.CloseMergeRequest(ctx, project.ID, mr.IID, project.PathWithNamespace)
	v.finishMRAction(ctx, project, mr, "Closed", updated, err)
}

func (v *Views) runMergeAction(ctx context.Context, project *models.Project, mr *models.MergeRequest, opts gitlab.AcceptOptions) {
	if v.app == nil || v.app.GitLab == nil {
		v.ActionsModal.SetErr("internal: gitlab client unavailable")
		v.triggerRedraw()

		return
	}
	updated, err := v.app.GitLab.AcceptMergeRequest(ctx, project.ID, mr.IID, project.PathWithNamespace, opts)
	v.finishMRAction(ctx, project, mr, "Merged", updated, err)
}

//nolint:contextcheck // DetailView.SetMR owns its own context propagation (see G4 ADRs); the ctx arg scopes only the MRs list refetch.
func (v *Views) finishMRAction(
	ctx context.Context,
	project *models.Project,
	mr *models.MergeRequest,
	verbDone string,
	updated *models.MergeRequest,
	err error,
) {
	if err != nil {
		v.ActionsModal.SetErr(err.Error())
		v.triggerRedraw()

		return
	}
	v.ActionsModal.Close()
	if v.MRs != nil {
		v.MRs.SetProject(ctx, project)
		v.MRs.SetTransientStatus(fmt.Sprintf("%s !%d", verbDone, mr.IID))
	}
	if v.Detail != nil && updated != nil {
		// SetMR internally dispatches per-tab fetches on context.Background
		// by design (see G4 ADRs); the ctx passed here is used only for the
		// MRs list refetch above.
		v.Detail.SetMR(project, updated)
	}
	v.restoreFocusAfterModal()
	v.triggerRedraw()
}

// restoreFocusAfterModal parks focus back on the pane that opened the
// modal, so a user who hit `M` from the Pipeline tab stays on Pipeline
// after merge instead of getting yanked to the MRs list. Falls back to the
// MRs pane when the origin pane no longer exists (e.g. the Log pane
// auto-closed on tab switch, or the tab was cycled while the mutation was
// in flight). Queued via g.Update because the modal pane is still mounted
// until the next tick deletes it; the origin read + clear happens inside
// the closure so the write in openMRActionModal and this read both run on
// the main goroutine.
func (v *Views) restoreFocusAfterModal() {
	if v.g == nil {
		return
	}
	v.g.Update(func(gg *gocui.Gui) error {
		target := v.focusRestoreTarget(gg)
		v.actionOriginView = ""
		if _, err := gg.SetCurrentView(target); err != nil {
			return fmt.Errorf("restore focus after modal: %w", err)
		}

		return nil
	})
}

// focusRestoreTarget picks the pane name that should receive focus after the
// modal dismisses. Pure function over (v.actionOriginView, gg.View lookup)
// so unit tests can exercise it without draining the gocui Update queue.
func (v *Views) focusRestoreTarget(gg *gocui.Gui) string {
	if origin := v.actionOriginView; origin != "" && gg != nil {
		if _, err := gg.View(origin); err == nil {
			return origin
		}
	}

	return keymap.ViewMRs
}

// triggerRedraw asks gocui to repaint on the next tick after an async
// mutation handler mutates state off the main goroutine.
func (v *Views) triggerRedraw() {
	if v.g == nil {
		return
	}
	v.g.Update(func(*gocui.Gui) error { return nil })
}
