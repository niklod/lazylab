package views

import (
	"context"
	"fmt"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/tui/keymap"
)

// Views aggregates every per-pane widget. Phase G3 ships repos + mrs;
// mr-detail tabs are added in G4.
type Views struct {
	Repos *ReposView
	MRs   *MRsView
}

func New(g *gocui.Gui, app *appcontext.AppContext) *Views {
	return &Views{
		Repos: NewRepos(g, app),
		MRs:   NewMRs(g, app),
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
	}

	return out
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
