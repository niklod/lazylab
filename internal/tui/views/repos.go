// Package views owns the per-pane widgets mounted inside the TUI's 3-pane
// layout. Each file here corresponds to one pane (or to one MR detail tab).
//
// Views depend on keymap (shared Binding type + pane-name constants) but do
// NOT import the parent tui package — tui imports views at startup to wire
// them. Going the other way would form an import cycle.
package views

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/gitlab"
	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
	"github.com/niklod/lazylab/internal/tui/theme"
)

const (
	iconFavorited     = "★"
	iconNoFav         = "  " // two spaces keep the path column aligned with favs
	reposPaneTitle    = "[1] Repositories"
	reposLoadingTitle = "Loading projects…"
)

// ReposView renders the project list pane: favourite indicator + path,
// search filter, vim-style navigation, favourite toggle with YAML persistence.
type ReposView struct {
	g   *gocui.Gui
	app *appcontext.AppContext

	mu           sync.Mutex
	all          []*models.Project
	allLower     []string // lowercase PathWithNamespace per v.all index; caches tolower to make search allocation-free
	favSet       map[string]bool
	filtered     []*models.Project
	cursor       int
	query        string
	searchActive bool
	loading      bool
	loadErr      error
}

// NewRepos constructs a ReposView bound to g and app. No I/O happens until Load.
func NewRepos(g *gocui.Gui, app *appcontext.AppContext) *ReposView {
	return &ReposView{g: g, app: app, loading: true}
}

// Load fetches the project list in a goroutine and publishes the result via
// g.Update so all state mutation happens on the main loop thread.
func (v *ReposView) Load(ctx context.Context) {
	go func() {
		projects, err := v.fetchProjects(ctx)
		v.g.Update(func(_ *gocui.Gui) error {
			v.apply(projects, err)

			return nil
		})
	}()
}

// LoadSync fetches and applies projects inline. Used by integration tests that
// run without a gocui MainLoop to consume g.Update callbacks. Never call from
// production code — the main loop path is Load.
func (v *ReposView) LoadSync(ctx context.Context) error {
	projects, err := v.fetchProjects(ctx)
	v.apply(projects, err)

	return err
}

func (v *ReposView) fetchProjects(ctx context.Context) ([]*models.Project, error) {
	ps, err := v.app.GitLab.ListProjects(ctx, gitlab.ListProjectsOptions{})
	if err != nil {
		return nil, fmt.Errorf("fetch projects: %w", err)
	}

	return ps, nil
}

func (v *ReposView) apply(projects []*models.Project, loadErr error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.loading = false
	if loadErr != nil {
		v.loadErr = loadErr

		return
	}
	v.loadErr = nil
	v.all = projects
	v.refreshFavSetLocked()
	v.sortLocked()
	v.applyFilterLocked()
	if v.cursor >= len(v.filtered) {
		v.cursor = 0
	}
}

func (v *ReposView) rebuildLowerLocked() {
	v.allLower = make([]string, len(v.all))
	for i, p := range v.all {
		v.allLower[i] = strings.ToLower(p.PathWithNamespace)
	}
}

func (v *ReposView) refreshFavSetLocked() {
	v.favSet = favoriteSet(v.app.Config.Repositories.Favorites)
}

// SearchActive reports whether the search input is currently being edited.
// Read by the layout tick to decide whether to mount the search pane.
func (v *ReposView) SearchActive() bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.searchActive
}

// Query returns the current filter query. Used by the layout to seed the
// search view's buffer on re-creation.
func (v *ReposView) Query() string {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.query
}

// SelectedProject returns the project under the cursor, or nil if the list
// is empty. The returned pointer shares the underlying model with the view;
// treat it as read-only.
func (v *ReposView) SelectedProject() *models.Project {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.cursor < 0 || v.cursor >= len(v.filtered) {
		return nil
	}

	return v.filtered[v.cursor]
}

// Render paints the pane's buffer. Must be called from the layout callback so
// it runs on the main loop thread.
func (v *ReposView) Render(pane *gocui.View) {
	v.mu.Lock()
	defer v.mu.Unlock()

	pane.Clear()
	pane.Highlight = true
	pane.SelBgColor = theme.ColorAccent
	pane.SelFgColor = theme.ColorSelectionFg

	switch {
	case v.loading:
		pane.WriteString(reposPaneTitle + "\n\n")
		pane.WriteString(theme.Wrap(theme.FgDim, reposLoadingTitle) + "\n")

		return
	case v.loadErr != nil:
		pane.WriteString(reposPaneTitle + "\n\n")
		pane.WriteString(theme.Wrap(theme.FgErr, fmt.Sprintf("✕ Error loading projects: %v", v.loadErr)) + "\n")

		return
	case len(v.filtered) == 0:
		pane.WriteString(reposHeader(0, len(v.all)))
		writeReposEmptyState(pane, v.query == "")

		return
	}

	pane.WriteString(reposHeader(len(v.filtered), len(v.all)))

	innerW, _ := pane.InnerSize()
	now := time.Now()
	for _, p := range v.filtered {
		icon := iconNoFav
		iconW := 2 // iconNoFav is two spaces
		if v.favSet[p.PathWithNamespace] {
			icon = accent(iconFavorited)
			iconW = 1 // "★" is a single cell
		}
		ago := theme.Relative(p.LastActivityAt, now)
		pane.WriteString(formatRepoRow(icon, iconW, p.PathWithNamespace, ago, innerW) + "\n")
	}

	// Header line at row 0 means data rows start at row 1; offset cursor +1.
	if v.cursor >= 0 && v.cursor < len(v.filtered) {
		placeCursor(pane, v.cursor+1, len(v.filtered)+1)
	}
}

// reposHeader returns the dim "[1] Repositories · N/M" line that prefixes
// every populated render. Trailing newline included; the empty body that
// follows starts on the next row.
func reposHeader(filtered, total int) string {
	return reposPaneTitle + dim(fmt.Sprintf(" · %d/%d", filtered, total)) + "\n"
}

// writeReposEmptyState picks between the first-run hint (no projects loaded
// at all → point at the config) and the filter-miss hint (data loaded but
// nothing matches the query). Wording mirrors states.js.
func writeReposEmptyState(pane *gocui.View, firstRun bool) {
	var sb strings.Builder
	sb.WriteString("\n")
	if firstRun {
		sb.WriteString(dim(" No projects yet.") + "\n\n")
		sb.WriteString(dim(" Add a GitLab token to:") + "\n")
		sb.WriteString(dim(" ~/.config/lazylab/config.yaml") + "\n\n")
		sb.WriteString(" " + accent("o") + dim("  opens the config in $EDITOR") + "\n")
		pane.WriteString(sb.String())

		return
	}
	sb.WriteString(dim(" No projects match.") + "\n\n")
	sb.WriteString(dim(" Press ") + accent("Esc") + dim(" to clear the filter,") + "\n")
	sb.WriteString(dim(" or ") + accent("/") + dim(" to refine it.") + "\n")
	pane.WriteString(sb.String())
}

// Bindings returns the per-view keybindings owned by the repos pane, both on
// the pane itself (j/k/g/G/t//) and on the ephemeral search input view
// (Enter/Esc).
func (v *ReposView) Bindings() []keymap.Binding {
	return []keymap.Binding{
		{View: keymap.ViewRepos, Key: 'j', Handler: v.handleDown},
		{View: keymap.ViewRepos, Key: 'k', Handler: v.handleUp},
		{View: keymap.ViewRepos, Key: 'g', Handler: v.handleTop},
		{View: keymap.ViewRepos, Key: 'G', Handler: v.handleBottom},
		{View: keymap.ViewRepos, Key: 't', Handler: v.handleToggleFavorite},
		{View: keymap.ViewRepos, Key: '/', Handler: v.handleOpenSearch},
		{View: keymap.ViewRepos, Key: gocui.KeyEsc, Handler: v.handleClearFilter},
		{View: keymap.ViewReposSearch, Key: gocui.KeyEnter, Handler: v.handleSubmitSearch},
		{View: keymap.ViewReposSearch, Key: gocui.KeyEsc, Handler: v.handleCancelSearch},
	}
}

func (v *ReposView) handleDown(_ *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cursor+1 < len(v.filtered) {
		v.cursor++
	}

	return nil
}

func (v *ReposView) handleUp(_ *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cursor > 0 {
		v.cursor--
	}

	return nil
}

func (v *ReposView) handleTop(_ *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cursor = 0

	return nil
}

func (v *ReposView) handleBottom(_ *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if n := len(v.filtered); n > 0 {
		v.cursor = n - 1
	}

	return nil
}

func (v *ReposView) handleOpenSearch(g *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	v.searchActive = true
	v.mu.Unlock()

	// The layout tick will create the search view; focus it on the next tick.
	g.Update(func(g *gocui.Gui) error {
		if _, err := g.SetCurrentView(keymap.ViewReposSearch); err != nil {
			if !goerrors.Is(err, gocui.ErrUnknownView) {
				return fmt.Errorf("focus search view: %w", err)
			}
		}

		return nil
	})

	return nil
}

func (v *ReposView) handleSubmitSearch(g *gocui.Gui, searchV *gocui.View) error {
	q := strings.TrimSpace(strings.TrimRight(searchV.Buffer(), "\n"))

	v.mu.Lock()
	v.query = q
	v.searchActive = false
	v.cursor = 0
	v.applyFilterLocked()
	v.mu.Unlock()

	return refocusRepos(g)
}

// handleClearFilter is bound to Esc on the repos pane itself (not the search
// input): after `/`+text+Enter has submitted a filter and focus has returned
// to the list, Esc drops the filter and restores the full project list.
// No-op when there is no active filter.
func (v *ReposView) handleClearFilter(_ *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.query == "" {
		return nil
	}
	v.query = ""
	v.cursor = 0
	v.applyFilterLocked()

	return nil
}

func (v *ReposView) handleCancelSearch(g *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	v.query = ""
	v.searchActive = false
	v.cursor = 0
	v.applyFilterLocked()
	v.mu.Unlock()

	return refocusRepos(g)
}

func (v *ReposView) handleToggleFavorite(_ *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.cursor < 0 || v.cursor >= len(v.filtered) {
		return nil
	}
	selected := v.filtered[v.cursor]
	if selected == nil {
		return nil
	}

	path := selected.PathWithNamespace
	current := v.app.Config.Repositories.Favorites
	next := toggleFavoriteList(current, path)

	prev := current
	v.app.Config.Repositories.Favorites = next
	if err := v.app.Config.Save(v.app.FS, v.app.ConfigPath); err != nil {
		v.app.Config.Repositories.Favorites = prev

		return fmt.Errorf("persist favorites: %w", err)
	}

	v.refreshFavSetLocked()
	v.sortLocked()
	v.applyFilterLocked()

	return nil
}

func (v *ReposView) sortLocked() {
	favs := v.favSet
	sort.SliceStable(v.all, func(i, j int) bool {
		fi, fj := favs[v.all[i].PathWithNamespace], favs[v.all[j].PathWithNamespace]
		if fi != fj {
			return fi
		}

		return v.all[i].LastActivityAt.After(v.all[j].LastActivityAt)
	})
	// v.all was reordered, so the parallel lowercase cache must be rebuilt
	// to stay index-aligned with v.all for applyFilterLocked.
	v.rebuildLowerLocked()
}

func (v *ReposView) applyFilterLocked() {
	if v.query == "" {
		v.filtered = append(v.filtered[:0], v.all...)

		return
	}
	needle := strings.ToLower(v.query)
	v.filtered = v.filtered[:0]
	for i, p := range v.all {
		if strings.Contains(v.allLower[i], needle) {
			v.filtered = append(v.filtered, p)
		}
	}
}

func refocusRepos(g *gocui.Gui) error {
	if err := g.DeleteView(keymap.ViewReposSearch); err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
		return fmt.Errorf("delete search view: %w", err)
	}
	if _, err := g.SetCurrentView(keymap.ViewRepos); err != nil {
		return fmt.Errorf("refocus repos: %w", err)
	}

	return nil
}

func favoriteSet(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, s := range list {
		out[s] = true
	}

	return out
}

// toggleFavoriteList returns a new slice with path removed if present or
// appended if absent — never mutates the input.
func toggleFavoriteList(list []string, path string) []string {
	if i := slices.Index(list, path); i >= 0 {
		return slices.Delete(slices.Clone(list), i, i+1)
	}

	return append(slices.Clone(list), path)
}
