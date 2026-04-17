package tui

import (
	"fmt"

	// gocui wraps ErrUnknownView via github.com/go-errors/errors v1.0.2, which
	// predates Go 1.13's Unwrap() interface — so stdlib errors.Is returns false.
	// Use goerrors.Is to traverse the wrap chain by pointer-equality instead.
	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/tui/keymap"
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
			pv.Wrap = false
		}
		if v != nil && p.name == ViewRepos && v.Repos != nil {
			v.Repos.Render(pv)
		}
		if v != nil && p.name == ViewMRs && v.MRs != nil {
			v.MRs.Render(pv)
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

	if g.CurrentView() == nil {
		if _, err := g.SetCurrentView(ViewRepos); err != nil {
			return fmt.Errorf("set initial current view: %w", err)
		}
	}

	highlightFocused(g)

	return nil
}

// manageSearchPane mounts or removes an ephemeral single-line search input
// pinned to the top of its owning pane. The pane's existence is derived from
// `active` (the owning view's SearchActive reading), so a missed DeleteView
// from a handler self-heals on the next render tick.
func manageSearchPane(g *gocui.Gui, viewName string, active bool, r rect) error {
	if !active {
		if err := g.DeleteView(viewName); err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
			return fmt.Errorf("delete %s: %w", viewName, err)
		}

		return nil
	}

	sr := rect{x0: r.x0, y0: r.y0, x1: r.x1, y1: r.y0 + searchPaneHeight}
	sv, err := g.SetView(viewName, sr.x0, sr.y0, sr.x1-1, sr.y1-1, 0)
	firstCreate := goerrors.Is(err, gocui.ErrUnknownView)
	if err != nil && !firstCreate {
		return fmt.Errorf("set %s: %w", viewName, err)
	}
	if firstCreate {
		sv.Frame = true
		sv.Title = " Search "
		sv.Editable = true
		sv.Editor = gocui.DefaultEditor
	}

	return nil
}

func highlightFocused(g *gocui.Gui) {
	current := currentViewName(g)
	for _, name := range focusOrder {
		v, err := g.View(name)
		if err != nil {
			continue
		}
		if name == current {
			v.FrameColor = gocui.ColorGreen
			v.TitleColor = gocui.ColorGreen
		} else {
			v.FrameColor = gocui.ColorDefault
			v.TitleColor = gocui.ColorDefault
		}
	}
}
