package tui

import (
	"fmt"

	// gocui wraps ErrUnknownView via github.com/go-errors/errors v1.0.2, which
	// predates Go 1.13's Unwrap() interface — so stdlib errors.Is returns false.
	// Use goerrors.Is to traverse the wrap chain by pointer-equality instead.
	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/tui/keymap"
	"github.com/niklod/lazylab/internal/tui/theme"
	"github.com/niklod/lazylab/internal/tui/views"
)

const searchPaneHeight = 3

type rect struct {
	x0, y0, x1, y1 int
}

type paneLayout struct {
	repos, mrs, detail rect
}

// paneRects computes the 3-pane rectangles for a terminal of the given size.
// Left column is 40% wide; inside it, repos occupies the top half and mrs the bottom.
// Right column (detail) occupies the remaining 60% full-height.
// Returned by value to avoid a per-tick heap allocation.
func paneRects(maxX, maxY int) paneLayout {
	leftW := maxX * 40 / 100
	topH := maxY / 2

	return paneLayout{
		repos:  rect{0, 0, leftW, topH},
		mrs:    rect{0, topH, leftW, maxY},
		detail: rect{leftW, 0, maxX, maxY},
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
