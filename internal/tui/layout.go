package tui

import (
	"fmt"

	// gocui wraps ErrUnknownView via github.com/go-errors/errors v1.0.2, which
	// predates Go 1.13's Unwrap() interface — so stdlib errors.Is returns false.
	// Use goerrors.Is to traverse the wrap chain by pointer-equality instead.
	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"
)

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

func layout(g *gocui.Gui) error {
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
		v, err := g.SetView(p.name, p.rect.x0, p.rect.y0, p.rect.x1-1, p.rect.y1-1, 0)
		firstCreate := goerrors.Is(err, gocui.ErrUnknownView)
		if err != nil && !firstCreate {
			return fmt.Errorf("set view %q: %w", p.name, err)
		}
		if firstCreate {
			v.Frame = true
			v.Title = p.title
			v.Wrap = false
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
