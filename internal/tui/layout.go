package tui

import (
	"fmt"
	"time"

	// gocui wraps ErrUnknownView via github.com/go-errors/errors v1.0.2, which
	// predates Go 1.13's Unwrap() interface — so stdlib errors.Is returns false.
	// Use goerrors.Is to traverse the wrap chain by pointer-equality instead.
	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/tui/keymap"
	"github.com/niklod/lazylab/internal/tui/theme"
	"github.com/niklod/lazylab/internal/tui/views"
)

const (
	searchPaneHeight = 3
	// footerHeight is the outer row count reserved at the bottom of the
	// screen for the global FooterView. gocui's InnerHeight = outer - 2
	// (see view.go:534 — "if it doesn't [have a frame], the view is made
	// 1 larger on all sides"), so for two rendered lines (meta +
	// keybinds, per design/wireframes/layout.js:60-62) the outer height
	// must be 4.
	footerHeight = 4
)

type rect struct {
	x0, y0, x1, y1 int
}

type paneLayout struct {
	repos, mrs, detail, footer rect
}

// paneRects computes the 3-pane rectangles for a terminal of the given size.
// Left column is 40% wide; inside it, repos occupies the top half and mrs the bottom.
// Right column (detail) occupies the remaining 60% full-height.
// The bottom footerHeight rows are reserved for the global FooterView and
// subtracted from every pane's y1 so the content never overlaps the strip.
// Returned by value to avoid a per-tick heap allocation.
func paneRects(maxX, maxY int) paneLayout {
	leftW := maxX * 40 / 100
	panesBottom := maxY - footerHeight
	if panesBottom < 2 {
		// Terminal shorter than the footer itself — let panes fall back to
		// the full viewport; footer rect will collapse and manageFooter
		// no-ops via its size guard.
		panesBottom = maxY
	}
	topH := panesBottom / 2

	return paneLayout{
		repos:  rect{0, 0, leftW, topH},
		mrs:    rect{0, topH, leftW, panesBottom},
		detail: rect{leftW, 0, maxX, panesBottom},
		footer: rect{0, panesBottom, maxX, maxY},
	}
}

type pane struct {
	name  string
	rect  rect
	title string
}

// layout renders the static 3-pane frame. Kept as a free function so the
// existing test suite can call it directly (no views dependency); app.go
// uses NewManager when a Views instance is present.
func layout(g *gocui.Gui) error {
	return renderPanes(g, nil)
}

// NewManager returns a gocui manager that renders the 3-pane frame and then
// delegates per-pane drawing to the Views. Exported so e2e tests can install
// the production layout against a headless Gui without re-implementing the
// frame.
func NewManager(v *views.Views) func(*gocui.Gui) error {
	return func(g *gocui.Gui) error {
		return renderPanes(g, v)
	}
}

func renderPanes(g *gocui.Gui, v *views.Views) error {
	maxX, maxY := g.Size()
	if maxX < 2 || maxY < 2 {
		return nil
	}
	r := paneRects(maxX, maxY)
	panes := [...]pane{
		{name: ViewRepos, rect: r.repos, title: " Repositories "},
		{name: ViewMRs, rect: r.mrs, title: " Merge Requests "},
		{name: ViewDetail, rect: r.detail, title: " Details "},
	}

	for _, p := range panes {
		pv, err := g.SetView(p.name, p.rect.x0, p.rect.y0, p.rect.x1-1, p.rect.y1-1, 0)
		firstCreate := goerrors.Is(err, gocui.ErrUnknownView)
		if err != nil && !firstCreate {
			return fmt.Errorf("set view %q: %w", p.name, err)
		}
		if firstCreate {
			pv.Frame = true
			pv.Title = p.title
			pv.Wrap = p.name == ViewDetail
		}
		if v != nil && p.name == ViewRepos && v.Repos != nil {
			v.Repos.Render(pv)
		}
		if v != nil && p.name == ViewMRs && v.MRs != nil {
			v.MRs.Render(pv)
		}
		if v != nil && p.name == ViewDetail && v.Detail != nil {
			v.Detail.Render(pv)
		}
	}

	if v != nil && v.Repos != nil {
		if err := manageSearchPane(g, keymap.ViewReposSearch, v.Repos.SearchActive(), r.repos); err != nil {
			return err
		}
	}
	if v != nil && v.MRs != nil {
		if err := manageSearchPane(g, keymap.ViewMRsSearch, v.MRs.SearchActive(), r.mrs); err != nil {
			return err
		}
	}
	if v != nil && v.Detail != nil {
		if err := manageDiffSubpanes(g, v, r.detail); err != nil {
			return err
		}
		if err := managePipelineSubpanes(g, v, r.detail); err != nil {
			return err
		}
		if err := manageConversationSubpane(g, v, r.detail); err != nil {
			return err
		}
		if err := applyPendingDetailFocus(g, v); err != nil {
			return err
		}
	}
	if v != nil && v.ActionsModal != nil {
		if err := manageMRActionsModal(g, v, maxX, maxY); err != nil {
			return err
		}
	}
	if v != nil && v.Footer != nil {
		if err := manageFooter(g, v, r.footer); err != nil {
			return err
		}
	}

	if g.CurrentView() == nil {
		if _, err := g.SetCurrentView(ViewRepos); err != nil {
			return fmt.Errorf("set initial current view: %w", err)
		}
	}

	highlightFocused(g)

	return nil
}

// manageDiffSubpanes mounts the Diff-tab's file-tree + content panes when
// the Detail view's active tab is Diff; removes them otherwise. Lives
// inside the detail rect — pane row 0 is the tab bar, child panes occupy
// rows 1..end. Frame is disabled to avoid nested borders inside detail's
// frame; the 30/70 split is visual, enforced by a 1-column column gap.
func manageDiffSubpanes(g *gocui.Gui, v *views.Views, detail rect) error {
	active := v.Detail != nil && v.Detail.CurrentTab() == views.DetailTabDiff
	if !active {
		if err := deleteViewIfPresent(g, keymap.ViewDetailDiffTree); err != nil {
			return err
		}

		return deleteViewIfPresent(g, keymap.ViewDetailDiffContent)
	}

	inner := rect{
		x0: detail.x0 + 1,
		y0: detail.y0 + 2,
		x1: detail.x1 - 1,
		y1: detail.y1 - 1,
	}
	if inner.x1-inner.x0 < 10 || inner.y1-inner.y0 < 3 {
		return nil
	}

	treeWidth := (inner.x1 - inner.x0) * 30 / 100
	if treeWidth < 8 {
		treeWidth = 8
	}
	treeRect := rect{x0: inner.x0, y0: inner.y0, x1: inner.x0 + treeWidth, y1: inner.y1}
	contentRect := rect{x0: inner.x0 + treeWidth + 1, y0: inner.y0, x1: inner.x1, y1: inner.y1}

	if err := mountSubpane(g, keymap.ViewDetailDiffTree, treeRect, func(pv *gocui.View) {
		pv.Frame = false
		pv.Wrap = false
	}); err != nil {
		return err
	}
	if err := mountSubpane(g, keymap.ViewDetailDiffContent, contentRect, func(pv *gocui.View) {
		pv.Frame = false
		pv.Wrap = false
	}); err != nil {
		return err
	}

	if tree, err := g.View(keymap.ViewDetailDiffTree); err == nil && v.Detail.DiffTree() != nil {
		v.Detail.DiffTree().Render(tree)
	}
	if content, err := g.View(keymap.ViewDetailDiffContent); err == nil && v.Detail.DiffContent() != nil {
		v.Detail.DiffContent().Render(content)
	}

	return nil
}

// managePipelineSubpanes mounts exactly one of the Pipeline-tab child panes
// (stages when the log is closed, log when open) inside the detail rect.
// Mount-before-delete order matters: gocui.DeleteView does not clear the
// current-view pointer, so when the user toggles the log we must create
// the incoming pane first, then delete the outgoing one — otherwise
// SetCurrentView would dangle for one tick.
func managePipelineSubpanes(g *gocui.Gui, v *views.Views, detail rect) error {
	active := v.Detail != nil && v.Detail.CurrentTab() == views.DetailTabPipeline
	if !active {
		if err := deleteViewIfPresent(g, keymap.ViewDetailPipelineStages); err != nil {
			return err
		}

		return deleteViewIfPresent(g, keymap.ViewDetailPipelineJobLog)
	}

	inner := rect{
		x0: detail.x0 + 1,
		y0: detail.y0 + 2,
		x1: detail.x1 - 1,
		y1: detail.y1 - 1,
	}
	if inner.x1-inner.x0 < 10 || inner.y1-inner.y0 < 3 {
		return nil
	}

	if v.Detail.LogOpen() {
		if err := mountSubpane(g, keymap.ViewDetailPipelineJobLog, inner, func(pv *gocui.View) {
			pv.Frame = false
			pv.Wrap = false
		}); err != nil {
			return err
		}
		if err := deleteViewIfPresent(g, keymap.ViewDetailPipelineStages); err != nil {
			return err
		}
		if pv, err := g.View(keymap.ViewDetailPipelineJobLog); err == nil && v.Detail.JobLog() != nil {
			v.Detail.JobLog().Render(pv)
		}

		return nil
	}

	if err := mountSubpane(g, keymap.ViewDetailPipelineStages, inner, func(pv *gocui.View) {
		pv.Frame = false
		pv.Wrap = false
	}); err != nil {
		return err
	}
	if err := deleteViewIfPresent(g, keymap.ViewDetailPipelineJobLog); err != nil {
		return err
	}
	if pv, err := g.View(keymap.ViewDetailPipelineStages); err == nil && v.Detail.PipelineStages() != nil {
		v.Detail.PipelineStages().Render(pv)
	}

	return nil
}

// manageConversationSubpane mounts a single pane inside the detail rect
// when the Conversation tab is active; removes it otherwise. No mode toggle
// (unlike managePipelineSubpanes) — the conversation view is a plain list.
func manageConversationSubpane(g *gocui.Gui, v *views.Views, detail rect) error {
	active := v.Detail != nil && v.Detail.CurrentTab() == views.DetailTabConversation
	if !active {
		return deleteViewIfPresent(g, keymap.ViewDetailConversation)
	}

	inner := rect{
		x0: detail.x0 + 1,
		y0: detail.y0 + 2,
		x1: detail.x1 - 1,
		y1: detail.y1 - 1,
	}
	if inner.x1-inner.x0 < 10 || inner.y1-inner.y0 < 3 {
		return nil
	}

	if err := mountSubpane(g, keymap.ViewDetailConversation, inner, func(pv *gocui.View) {
		pv.Frame = false
		// Wrap is done manually at the view level — gocui's built-in
		// soft-wrap would start each continuation line at column 0,
		// losing the spine/indent alignment for note bodies.
		pv.Wrap = false
	}); err != nil {
		return err
	}
	if pv, err := g.View(keymap.ViewDetailConversation); err == nil && v.Detail.Conversation() != nil {
		v.Detail.Conversation().Render(pv)
	}

	return nil
}

// manageMRActionsModal mounts a centered confirmation sub-pane on top of
// the 3-pane frame when v.ActionsModal.IsActive(), removes it otherwise.
// Height depends on kind (close is tighter than merge). Focus handling is
// owned by the FocusOrder override in views.Views — this function only
// deals with geometry and painting. Mount happens AFTER all other
// sub-panes so the modal sits on top of them in z-order.
func manageMRActionsModal(g *gocui.Gui, v *views.Views, maxX, maxY int) error {
	if !v.ActionsModal.IsActive() {
		return deleteViewIfPresent(g, keymap.ViewMRActionsModal)
	}

	snap := v.ActionsModal.Snapshot()
	h := views.ModalHeight(snap.Kind, len(snap.ErrLines))
	w := views.ModalWidth
	if w > maxX-4 {
		w = maxX - 4
	}
	if h > maxY-4 {
		h = maxY - 4
	}
	if w < 10 || h < 4 {
		return nil
	}

	x0 := (maxX - w) / 2
	y0 := (maxY - h) / 2
	r := rect{x0: x0, y0: y0, x1: x0 + w, y1: y0 + h}

	title := views.TitleFor(snap.Kind)
	if err := mountSubpane(g, keymap.ViewMRActionsModal, r, func(pv *gocui.View) {
		pv.Frame = true
		pv.Wrap = false
		pv.Title = title
		pv.FrameColor = theme.ColorAccent
		pv.TitleColor = theme.ColorAccent
	}); err != nil {
		return err
	}

	pv, err := g.View(keymap.ViewMRActionsModal)
	if err != nil {
		// The pane was just mounted above; g.View failing here would be
		// a gocui invariant break. Swallowing to nil would hide real
		// bugs, so surface the error wrapped.
		return fmt.Errorf("fetch mr action modal pane: %w", err)
	}
	// Title can change kind-to-kind (re-Open with different kind). Re-apply
	// each tick so the border tracks the active kind. Cheap.
	pv.Title = title
	pv.FrameColor = theme.ColorAccent
	pv.TitleColor = theme.ColorAccent

	// RenderSnap shares the snapshot we already took above so the pane height
	// and body are always computed from the same mutex observation — no
	// single-frame race where the pane is sized for N err lines but Render
	// writes N+1.
	v.ActionsModal.RenderSnap(pv, snap)

	return nil
}

// manageFooter mounts the global two-line footer (keybind strip + meta
// breadcrumb) pinned to the bottom of the terminal. Always mounted — never
// hidden — because the wireframe (design/wireframes/layout.js:60-62)
// treats the strip as a persistent discovery surface. Modal overlays sit
// above it; the hint line switches to the modal's hints via FooterState.
func manageFooter(g *gocui.Gui, v *views.Views, r rect) error {
	if r.y1-r.y0 < footerHeight || r.x1-r.x0 < 10 {
		return deleteViewIfPresent(g, keymap.ViewFooter)
	}

	if err := mountSubpane(g, keymap.ViewFooter, r, func(pv *gocui.View) {
		pv.Frame = false
		pv.Wrap = false
	}); err != nil {
		return err
	}

	pv, err := g.View(keymap.ViewFooter)
	if err != nil {
		return fmt.Errorf("fetch footer pane: %w", err)
	}

	v.Footer.Render(pv, buildFooterState(g, v))

	return nil
}

// buildFooterState snapshots every input FooterView consumes so Render
// remains pure. Reads happen on the layout goroutine — all cross-view
// lookups use existing public accessors that own their own locks.
func buildFooterState(g *gocui.Gui, v *views.Views) views.FooterState {
	st := views.FooterState{
		FocusedView: currentViewName(g),
		Now:         time.Now(),
	}

	if v.MRs != nil {
		snap := v.MRs.FooterSnap()
		if snap.Project != nil {
			st.RepoPath = snap.Project.PathWithNamespace
		}
		if snap.Selected != nil {
			st.MRIID = snap.Selected.IID
		}
		st.MRIndex = snap.Index
		st.MRTotal = snap.Total
		st.LastSync = snap.LastSync
	}
	if v.Repos != nil && st.RepoPath == "" {
		if p := v.Repos.SelectedProject(); p != nil {
			st.RepoPath = p.PathWithNamespace
		}
	}
	st.SearchActive = (v.Repos != nil && v.Repos.SearchActive()) ||
		(v.MRs != nil && v.MRs.SearchActive())
	if v.ActionsModal != nil {
		snap := v.ActionsModal.Snapshot()
		st.ModalActive = snap.Active
		st.ModalKind = snap.Kind
	}

	return st
}

func mountSubpane(g *gocui.Gui, name string, r rect, onCreate func(*gocui.View)) error {
	pv, err := g.SetView(name, r.x0, r.y0, r.x1-1, r.y1-1, 0)
	firstCreate := goerrors.Is(err, gocui.ErrUnknownView)
	if err != nil && !firstCreate {
		return fmt.Errorf("set %s: %w", name, err)
	}
	if firstCreate && onCreate != nil {
		onCreate(pv)
	}

	return nil
}

func deleteViewIfPresent(g *gocui.Gui, name string) error {
	if err := g.DeleteView(name); err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
		return fmt.Errorf("delete %s: %w", name, err)
	}

	return nil
}

// applyPendingDetailFocus consumes a tab-switch focus request posted by
// DetailView.SetTab. Runs after manageDiffSubpanes so the target view is
// guaranteed to exist (or not — caller swallows ErrUnknownView for the
// Overview case where the base pane is always mounted by renderPanes).
func applyPendingDetailFocus(g *gocui.Gui, v *views.Views) error {
	target := v.Detail.ConsumePendingFocus()
	if target == "" {
		return nil
	}
	if _, err := g.SetCurrentView(target); err != nil {
		return fmt.Errorf("focus %q: %w", target, err)
	}

	return nil
}

// manageSearchPane mounts or removes an ephemeral single-line search input
// pinned to the top of its owning pane. The pane's existence is derived from
// `active` (the owning view's SearchActive reading), so a missed DeleteView
// from a handler self-heals on the next render tick.
func manageSearchPane(g *gocui.Gui, viewName string, active bool, r rect) error {
	if !active {
		return deleteViewIfPresent(g, viewName)
	}

	sr := rect{x0: r.x0, y0: r.y0, x1: r.x1, y1: r.y0 + searchPaneHeight}

	return mountSubpane(g, viewName, sr, func(sv *gocui.View) {
		sv.Frame = true
		sv.Title = " Search "
		sv.Editable = true
		sv.Editor = gocui.DefaultEditor
	})
}

func highlightFocused(g *gocui.Gui) {
	current := currentViewName(g)
	currentInDetail := detailFamily(current)

	painted := map[string]struct{}{}
	paint := func(name string) {
		v, err := g.View(name)
		if err != nil {
			return
		}
		painted[name] = struct{}{}
		focused := name == current || (name == ViewDetail && currentInDetail)
		if focused {
			v.FrameColor = theme.ColorAccent
			v.TitleColor = theme.ColorAccent

			return
		}
		v.FrameColor = gocui.ColorDefault
		v.TitleColor = gocui.ColorDefault
	}

	for _, name := range focusOrderFn() {
		paint(name)
	}
	// ViewDetail may be absent from the Diff-tab focus order (its sub-panes
	// own the cycle) but the frame must still reflect focus state. Paint
	// the parent pane explicitly so the green border tracks the family.
	if _, seen := painted[ViewDetail]; !seen {
		paint(ViewDetail)
	}
}
