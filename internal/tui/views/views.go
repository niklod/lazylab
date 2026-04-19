package views

import (
	"context"
	"fmt"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
)

// Views aggregates every per-pane widget. Phase G4 adds the Detail pane
// (overview sub-task); tab dispatch lands with the diff/conversation/pipeline
// sub-tasks.
type Views struct {
	Repos  *ReposView
	MRs    *MRsView
	Detail *DetailView
}

func New(g *gocui.Gui, app *appcontext.AppContext) *Views {
	return &Views{
		Repos:  NewRepos(g, app),
		MRs:    NewMRs(g, app),
		Detail: NewDetail(g, app),
	}
}

// Bindings aggregates per-view bindings plus the cross-view wiring that links
// repos-pane project selection to the mrs pane. Cross-view bindings live here,
// not on either view, so neither view has to know about the other.
func (v *Views) Bindings() []keymap.Binding {
	if v == nil {
		return nil
	}
	var out []keymap.Binding
	if v.Repos != nil {
		out = append(out, v.Repos.Bindings()...)
		out = append(out, keymap.Binding{
			View:    keymap.ViewRepos,
			Key:     gocui.KeyEnter,
			Handler: v.selectProjectForMRs,
		})
	}
	if v.MRs != nil {
		out = append(out, v.MRs.Bindings()...)
		out = append(out, keymap.Binding{
			View:    keymap.ViewMRs,
			Key:     gocui.KeyEnter,
			Handler: v.selectMRForDetail,
		})
	}
	if v.Detail != nil {
		out = append(out, v.detailBindings()...)
	}

	return out
}

// FocusOrder returns the active focus cycle — includes the Diff-tab
// sub-panes when the Diff tab is showing, the Pipeline-tab sub-panes
// (stages alone, or log alone when expanded) when Pipeline is showing,
// otherwise the plain 3-pane cycle. Installed into the tui package via
// SetFocusOrderProvider at startup.
func (v *Views) FocusOrder() []string {
	base := []string{keymap.ViewRepos, keymap.ViewMRs, keymap.ViewDetail}
	if v == nil || v.Detail == nil {
		return base
	}
	switch v.Detail.CurrentTab() {
	case DetailTabDiff:
		return []string{
			keymap.ViewRepos,
			keymap.ViewMRs,
			keymap.ViewDetailDiffTree,
			keymap.ViewDetailDiffContent,
		}
	case DetailTabPipeline:
		child := keymap.ViewDetailPipelineStages
		if v.Detail.LogOpen() {
			child = keymap.ViewDetailPipelineJobLog
		}

		return []string{keymap.ViewRepos, keymap.ViewMRs, child}
	}

	return base
}

// detailBindings produces the full per-view binding set for the Detail
// pane and its Diff-tab sub-panes. `[` / `]` are duplicated across every
// detail-family view because gocui dispatches by focused view name —
// binding only on ViewDetail leaves the user stuck in the Diff tab.
func (v *Views) detailBindings() []keymap.Binding {
	cycleKeys := []rune{'[', ']'}
	detailFamily := []string{
		keymap.ViewDetail,
		keymap.ViewDetailDiffTree,
		keymap.ViewDetailDiffContent,
		keymap.ViewDetailPipelineStages,
		keymap.ViewDetailPipelineJobLog,
	}

	var out []keymap.Binding
	for _, k := range cycleKeys {
		for _, view := range detailFamily {
			delta := 1
			if k == '[' {
				delta = -1
			}
			out = append(out, keymap.Binding{
				View:    view,
				Key:     k,
				Handler: v.cycleDetailTab(delta),
			})
		}
	}

	out = append(out, v.diffTreeBindings()...)
	out = append(out, v.diffContentBindings()...)
	out = append(out, v.pipelineStagesBindings()...)
	out = append(out, v.pipelineJobLogBindings()...)

	return out
}

func (v *Views) diffTreeBindings() []keymap.Binding {
	return []keymap.Binding{
		{View: keymap.ViewDetailDiffTree, Key: 'j', Handler: v.diffTreeMove(1)},
		{View: keymap.ViewDetailDiffTree, Key: 'k', Handler: v.diffTreeMove(-1)},
		{View: keymap.ViewDetailDiffTree, Key: gocui.KeyArrowDown, Handler: v.diffTreeMove(1)},
		{View: keymap.ViewDetailDiffTree, Key: gocui.KeyArrowUp, Handler: v.diffTreeMove(-1)},
		{View: keymap.ViewDetailDiffTree, Key: 'g', Handler: v.diffTreeMoveToStart},
		{View: keymap.ViewDetailDiffTree, Key: 'G', Handler: v.diffTreeMoveToEnd},
		{View: keymap.ViewDetailDiffTree, Key: gocui.KeyEnter, Handler: v.diffTreeSelect},
		// Ctrl+D / Ctrl+U on the tree pane scroll the diff CONTENT, not the
		// tree cursor. Users reach for these to read a long diff; rebinding
		// them to tree-cursor motion was a usability bug — tree navigation
		// already has j/k + g/G, and the content pane is where half-page
		// movement pays off.
		{View: keymap.ViewDetailDiffTree, Key: gocui.KeyCtrlD, Handler: v.diffContentHalfPage(+1)},
		{View: keymap.ViewDetailDiffTree, Key: gocui.KeyCtrlU, Handler: v.diffContentHalfPage(-1)},
	}
}

func (v *Views) diffContentBindings() []keymap.Binding {
	return []keymap.Binding{
		{View: keymap.ViewDetailDiffContent, Key: 'j', Handler: v.diffContentScroll(1)},
		{View: keymap.ViewDetailDiffContent, Key: 'k', Handler: v.diffContentScroll(-1)},
		{View: keymap.ViewDetailDiffContent, Key: gocui.KeyArrowDown, Handler: v.diffContentScroll(1)},
		{View: keymap.ViewDetailDiffContent, Key: gocui.KeyArrowUp, Handler: v.diffContentScroll(-1)},
		{View: keymap.ViewDetailDiffContent, Key: gocui.KeyCtrlD, Handler: v.diffContentHalfPage(+1)},
		{View: keymap.ViewDetailDiffContent, Key: gocui.KeyCtrlU, Handler: v.diffContentHalfPage(-1)},
	}
}

func (v *Views) pipelineStagesBindings() []keymap.Binding {
	return []keymap.Binding{
		{View: keymap.ViewDetailPipelineStages, Key: 'j', Handler: v.pipelineStagesMove(1)},
		{View: keymap.ViewDetailPipelineStages, Key: 'k', Handler: v.pipelineStagesMove(-1)},
		{View: keymap.ViewDetailPipelineStages, Key: gocui.KeyArrowDown, Handler: v.pipelineStagesMove(1)},
		{View: keymap.ViewDetailPipelineStages, Key: gocui.KeyArrowUp, Handler: v.pipelineStagesMove(-1)},
		{View: keymap.ViewDetailPipelineStages, Key: 'g', Handler: v.pipelineStagesMoveToStart},
		{View: keymap.ViewDetailPipelineStages, Key: 'G', Handler: v.pipelineStagesMoveToEnd},
		{View: keymap.ViewDetailPipelineStages, Key: gocui.KeyEnter, Handler: v.openJobLog},
		{View: keymap.ViewDetailPipelineStages, Key: 'r', Handler: v.retryCurrentJob},
		{View: keymap.ViewDetailPipelineStages, Key: 'R', Handler: v.forceRefreshPipeline},
		{View: keymap.ViewDetailPipelineStages, Key: 'o', Handler: v.openCurrentJobInBrowser},
		{View: keymap.ViewDetailPipelineStages, Key: 'a', Handler: v.toggleAutoRefresh},
	}
}

func (v *Views) pipelineJobLogBindings() []keymap.Binding {
	return []keymap.Binding{
		{View: keymap.ViewDetailPipelineJobLog, Key: 'j', Handler: v.pipelineLogScroll(1)},
		{View: keymap.ViewDetailPipelineJobLog, Key: 'k', Handler: v.pipelineLogScroll(-1)},
		{View: keymap.ViewDetailPipelineJobLog, Key: gocui.KeyArrowDown, Handler: v.pipelineLogScroll(1)},
		{View: keymap.ViewDetailPipelineJobLog, Key: gocui.KeyArrowUp, Handler: v.pipelineLogScroll(-1)},
		{View: keymap.ViewDetailPipelineJobLog, Key: gocui.KeyCtrlD, Handler: v.pipelineLogHalfPage(+1)},
		{View: keymap.ViewDetailPipelineJobLog, Key: gocui.KeyCtrlU, Handler: v.pipelineLogHalfPage(-1)},
		{View: keymap.ViewDetailPipelineJobLog, Key: 'g', Handler: v.pipelineLogScrollToTop},
		{View: keymap.ViewDetailPipelineJobLog, Key: 'G', Handler: v.pipelineLogScrollToBottom},
		{View: keymap.ViewDetailPipelineJobLog, Key: gocui.KeyEsc, Handler: v.closeJobLog},
		{View: keymap.ViewDetailPipelineJobLog, Key: 'r', Handler: v.retryCurrentJob},
		{View: keymap.ViewDetailPipelineJobLog, Key: 'R', Handler: v.forceRefreshPipeline},
		{View: keymap.ViewDetailPipelineJobLog, Key: 'o', Handler: v.openCurrentJobInBrowser},
		{View: keymap.ViewDetailPipelineJobLog, Key: 'a', Handler: v.toggleAutoRefresh},
		{View: keymap.ViewDetailPipelineJobLog, Key: 'y', Handler: v.copyLogBody},
	}
}

//nolint:contextcheck // gocui handler signature is fixed; background ctx is intentional.
func (v *Views) retryCurrentJob(_ *gocui.Gui, _ *gocui.View) error {
	if v.Detail == nil {
		return nil
	}
	if err := v.Detail.RetryCurrentJob(context.Background(), v.currentProject()); err != nil {
		return fmt.Errorf("retry job: %w", err)
	}

	return nil
}

//nolint:contextcheck // gocui handler signature is fixed; background ctx is intentional.
func (v *Views) forceRefreshPipeline(_ *gocui.Gui, _ *gocui.View) error {
	if v.Detail == nil {
		return nil
	}
	v.Detail.ForceRefreshPipeline(context.Background())

	return nil
}

func (v *Views) openCurrentJobInBrowser(_ *gocui.Gui, _ *gocui.View) error {
	if v.Detail == nil {
		return nil
	}
	if err := v.Detail.OpenCurrentJobInBrowser(); err != nil {
		return fmt.Errorf("open in browser: %w", err)
	}

	return nil
}

func (v *Views) toggleAutoRefresh(_ *gocui.Gui, _ *gocui.View) error {
	if v.Detail == nil {
		return nil
	}
	v.Detail.ToggleAutoRefresh()

	return nil
}

func (v *Views) copyLogBody(_ *gocui.Gui, _ *gocui.View) error {
	if v.Detail == nil {
		return nil
	}
	if err := v.Detail.CopyLogBody(); err != nil {
		return fmt.Errorf("copy log: %w", err)
	}

	return nil
}

func (v *Views) pipelineLogScrollToTop(g *gocui.Gui, _ *gocui.View) error {
	pv := v.pipelineLogPane(g)
	if pv == nil {
		return nil
	}
	v.Detail.JobLog().ScrollToTop(pv)

	return nil
}

func (v *Views) pipelineLogScrollToBottom(g *gocui.Gui, _ *gocui.View) error {
	pv := v.pipelineLogPane(g)
	if pv == nil {
		return nil
	}
	v.Detail.JobLog().ScrollToBottom(pv)

	return nil
}

func (v *Views) pipelineStagesMove(delta int) keymap.HandlerFunc {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		if v.Detail == nil || v.Detail.PipelineStages() == nil {
			return nil
		}
		v.Detail.PipelineStages().MoveCursor(delta)

		return nil
	}
}

func (v *Views) pipelineStagesMoveToStart(_ *gocui.Gui, _ *gocui.View) error {
	if v.Detail == nil || v.Detail.PipelineStages() == nil {
		return nil
	}
	v.Detail.PipelineStages().MoveCursorToStart()

	return nil
}

func (v *Views) pipelineStagesMoveToEnd(_ *gocui.Gui, _ *gocui.View) error {
	if v.Detail == nil || v.Detail.PipelineStages() == nil {
		return nil
	}
	v.Detail.PipelineStages().MoveCursorToEnd()

	return nil
}

//nolint:contextcheck // gocui handler signature is fixed; background ctx is intentional.
func (v *Views) openJobLog(_ *gocui.Gui, _ *gocui.View) error {
	if v.Detail == nil {
		return nil
	}
	v.Detail.OpenJobLog(v.currentProject())

	return nil
}

func (v *Views) closeJobLog(_ *gocui.Gui, _ *gocui.View) error {
	if v.Detail == nil {
		return nil
	}
	v.Detail.CloseJobLog()

	return nil
}

func (v *Views) pipelineLogScroll(delta int) keymap.HandlerFunc {
	return func(g *gocui.Gui, _ *gocui.View) error {
		pv := v.pipelineLogPane(g)
		if pv == nil {
			return nil
		}
		v.Detail.JobLog().ScrollBy(pv, delta)

		return nil
	}
}

func (v *Views) pipelineLogHalfPage(direction int) keymap.HandlerFunc {
	return func(g *gocui.Gui, _ *gocui.View) error {
		pv := v.pipelineLogPane(g)
		if pv == nil {
			return nil
		}
		_, innerH := pv.InnerSize()
		step := innerH / 2
		if step <= 0 {
			step = 1
		}
		v.Detail.JobLog().ScrollBy(pv, direction*step)

		return nil
	}
}

func (v *Views) pipelineLogPane(g *gocui.Gui) *gocui.View {
	if v.Detail == nil || v.Detail.JobLog() == nil || g == nil {
		return nil
	}
	pv, err := g.View(keymap.ViewDetailPipelineJobLog)
	if err != nil {
		return nil
	}

	return pv
}

// cycleDetailTab returns a handler that advances the detail pane's active
// tab. The returned closure cannot accept a context because gocui's
// keybinding handler signature is fixed at (g, pv) — internal fetches fall
// back to context.Background() by design (see G3 follow-ups for similar
// handlers in MRsView). contextcheck flags the chain; silenced here.
//
//nolint:contextcheck // gocui handler has no context; background is intentional
func (v *Views) cycleDetailTab(delta int) keymap.HandlerFunc {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		if v.Detail == nil {
			return nil
		}
		next := nextDetailTab(v.Detail.CurrentTab(), delta)
		project := v.currentProject()
		v.Detail.SetTab(next, project)

		return nil
	}
}

func (v *Views) diffTreeMove(delta int) keymap.HandlerFunc {
	return func(g *gocui.Gui, _ *gocui.View) error {
		if v.Detail == nil || v.Detail.DiffTree() == nil {
			return nil
		}
		if v.Detail.DiffTree().MoveCursor(delta) {
			v.pushDiffSelection(g)
		}

		return nil
	}
}

func (v *Views) diffTreeMoveToStart(g *gocui.Gui, _ *gocui.View) error {
	if v.Detail == nil || v.Detail.DiffTree() == nil {
		return nil
	}
	v.Detail.DiffTree().MoveCursorToStart()
	v.pushDiffSelection(g)

	return nil
}

func (v *Views) diffTreeMoveToEnd(g *gocui.Gui, _ *gocui.View) error {
	if v.Detail == nil || v.Detail.DiffTree() == nil {
		return nil
	}
	v.Detail.DiffTree().MoveCursorToEnd()
	v.pushDiffSelection(g)

	return nil
}

func (v *Views) diffTreeSelect(g *gocui.Gui, _ *gocui.View) error {
	if v.Detail == nil {
		return nil
	}
	v.Detail.SelectDiffFile(g)

	return nil
}

func (v *Views) diffContentScroll(delta int) keymap.HandlerFunc {
	return func(g *gocui.Gui, _ *gocui.View) error {
		pv := v.diffContentPane(g)
		if pv == nil {
			return nil
		}
		v.Detail.DiffContent().ScrollBy(pv, delta)

		return nil
	}
}

// diffContentHalfPage scrolls the diff CONTENT pane by half its visible
// height. Works whether the handler fires from the tree or the content
// pane — we look up ViewDetailDiffContent directly via g.View instead of
// trusting the inbound `pv` (which would be the tree when Ctrl+D is hit
// with focus on the file list).
func (v *Views) diffContentHalfPage(direction int) keymap.HandlerFunc {
	return func(g *gocui.Gui, _ *gocui.View) error {
		pv := v.diffContentPane(g)
		if pv == nil {
			return nil
		}
		_, innerH := pv.InnerSize()
		step := innerH / 2
		if step <= 0 {
			step = 1
		}
		v.Detail.DiffContent().ScrollBy(pv, direction*step)

		return nil
	}
}

func (v *Views) diffContentPane(g *gocui.Gui) *gocui.View {
	if v.Detail == nil || v.Detail.DiffContent() == nil || g == nil {
		return nil
	}
	pv, err := g.View(keymap.ViewDetailDiffContent)
	if err != nil {
		return nil
	}

	return pv
}

func (v *Views) pushDiffSelection(g *gocui.Gui) {
	if v.Detail == nil {
		return
	}
	v.Detail.SelectDiffFile(g)
}

func (v *Views) currentProject() *models.Project {
	if v.MRs == nil {
		return nil
	}

	return v.MRs.CurrentProject()
}

func nextDetailTab(current DetailTab, delta int) DetailTab {
	n := detailTabCount

	return DetailTab(((int(current)+delta)%n + n) % n)
}

// placeCursor sets the pane's Origin and Cursor so that contentRow (the
// 0-indexed line in the content buffer the caller wants highlighted) is
// visible within the pane. gocui's SetCursor is relative to the viewport
// (the on-screen row), not the content — so for a scrolled pane the caller
// must place origin + cursor together.
//
// Does NOT write to the pane buffer; call after the content has been
// written in Render.
func placeCursor(pane *gocui.View, contentRow, totalLines int) {
	_, innerH := pane.InnerSize()
	if innerH <= 0 {
		return
	}

	oy := 0
	if totalLines > innerH {
		_, currentOY := pane.Origin()
		oy = currentOY
		switch {
		case contentRow < oy:
			oy = contentRow
		case contentRow >= oy+innerH:
			oy = contentRow - innerH + 1
		}
		if maxOY := totalLines - innerH; oy > maxOY {
			oy = maxOY
		}
		if oy < 0 {
			oy = 0
		}
	}
	pane.SetOrigin(0, oy)
	pane.SetCursor(0, contentRow-oy)
}

// selectProjectForMRs copies the repos pane's selected project into the mrs
// pane, kicks off an async fetch, and moves focus to the mrs pane so the
// user can navigate the list without a follow-up `l` press.
func (v *Views) selectProjectForMRs(g *gocui.Gui, _ *gocui.View) error {
	if v.Repos == nil || v.MRs == nil {
		return nil
	}
	p := v.Repos.SelectedProject()
	if p == nil {
		return nil
	}
	v.MRs.SetProject(context.Background(), p)
	if _, err := g.SetCurrentView(keymap.ViewMRs); err != nil {
		return fmt.Errorf("focus mrs pane: %w", err)
	}

	return nil
}

// selectMRForDetail populates the detail pane with the MR under the mrs-pane
// cursor. Focus stays on the mrs pane — the detail pane has no bindings yet
// (tabs land with the diff/conversation/pipeline sub-tasks), so moving focus
// there would be a dead end. Global `h`/`l` still cycles focus manually.
func (v *Views) selectMRForDetail(_ *gocui.Gui, _ *gocui.View) error {
	if v.MRs == nil || v.Detail == nil {
		return nil
	}
	mr := v.MRs.SelectedMR()
	if mr == nil {
		return nil
	}
	v.Detail.SetMR(v.MRs.CurrentProject(), mr)

	return nil
}
