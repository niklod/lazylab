package views

import (
	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/tui/keymap"
)

// Views aggregates every per-pane widget. Phase G2 ships only the repos pane;
// mrs and detail-tab views are added in G3/G4.
type Views struct {
	Repos *ReposView
}

func New(g *gocui.Gui, app *appcontext.AppContext) *Views {
	return &Views{Repos: NewRepos(g, app)}
}

// Bindings aggregates per-view bindings for tui.Bind to register in one pass.
// G2 only has the repos pane; later phases append their views here.
func (v *Views) Bindings() []keymap.Binding {
	if v == nil || v.Repos == nil {
		return nil
	}

	return v.Repos.Bindings()
}
