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
		// Guard surfaces inside the modal itself — single feedback channel for
		// every failure mode (guard + network). Modal opens locked so Enter
		// is a no-op until the user presses Esc.
		v.ActionsModal.OpenGuarded(kind, mr, fmt.Sprintf("Cannot %s: MR !%d is %s", verb, mr.IID, mr.State))
	} else {
		v.ActionsModal.Open(kind, mr)
	}
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
	if !snap.Active || snap.MR == nil || snap.Busy || snap.Locked {
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
	v.runMRMutation(ctx, project, func(ctx context.Context) (*models.MergeRequest, error) {
		return v.app.GitLab.CloseMergeRequest(ctx, project.ID, mr.IID, project.PathWithNamespace)
	})
}

func (v *Views) runMergeAction(ctx context.Context, project *models.Project, mr *models.MergeRequest, opts gitlab.AcceptOptions) {
	v.runMRMutation(ctx, project, func(ctx context.Context) (*models.MergeRequest, error) {
		return v.app.GitLab.AcceptMergeRequest(ctx, project.ID, mr.IID, project.PathWithNamespace, opts)
	})
}

// runMRMutation wraps the nil-client guard + finish-or-error tail that the
// close/merge paths share. The unreachable nil-client branch is preserved
// (a unit test building a bare Views{} could hit it) so the modal surfaces
// the failure instead of wedging on busy=true with Esc ignored.
func (v *Views) runMRMutation(
	ctx context.Context,
	project *models.Project,
	call func(context.Context) (*models.MergeRequest, error),
) {
	if v.app == nil || v.app.GitLab == nil {
		v.ActionsModal.SetErr("internal: gitlab client unavailable")
		v.triggerRedraw()

		return
	}
	updated, err := call(ctx)
	v.finishMRAction(ctx, project, updated, err)
}

//nolint:contextcheck // DetailView.SetMR owns its own context propagation (see G4 ADRs); the ctx arg scopes only the MRs list refetch.
func (v *Views) finishMRAction(
	ctx context.Context,
	project *models.Project,
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
		// List refetch drops the just-mutated MR from the opened filter; the
		// MR disappearing from the pane is the success signal. No toast.
		v.MRs.SetProject(ctx, project)
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
