package views

import (
	"context"
	"fmt"
	"strings"
	"sync"

	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/gitlab"
	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
)

const (
	mrsStatusNoProject = "Select a project to view merge requests."
	mrsStatusLoading   = "Loading merge requests…"
	mrsStatusEmpty     = "No merge requests match."
)

// MRsView renders the Merge Requests pane. Fed by repos-pane project
// selection via SetProject; supports state/owner filter toggles and
// substring search (parity with Python MRContainer).
type MRsView struct {
	g   *gocui.Gui
	app *appcontext.AppContext

	mu           sync.Mutex
	current      *models.Project
	all          []*models.MergeRequest
	allLower     []string
	filtered     []*models.MergeRequest
	cursor       int
	query        string
	searchActive bool
	loading      bool
	loadErr      error
	stateFilter  models.MRStateFilter
	ownerFilter  models.MROwnerFilter
	bannerLine   string
	loadSeq      uint64
}

// NewMRs constructs an MRsView bound to g and app. The initial state/owner
// filter values come from the loaded config (Python parity).
func NewMRs(g *gocui.Gui, app *appcontext.AppContext) *MRsView {
	state := app.Config.MergeRequests.StateFilter
	if !state.IsValid() {
		state = models.MRStateFilterOpened
	}
	owner := app.Config.MergeRequests.OwnerFilter
	if !owner.IsValid() {
		owner = models.MROwnerFilterAll
	}

	v := &MRsView{
		g:           g,
		app:         app,
		stateFilter: state,
		ownerFilter: owner,
	}
	v.bannerLine = renderBannerLine(state, owner)

	return v
}

// SetProject is called when the repos pane commits a project selection.
// It kicks off an async fetch; results are applied via g.Update so all state
// mutation happens on the main loop thread.
func (v *MRsView) SetProject(ctx context.Context, p *models.Project) {
	if p == nil {
		return
	}
	seq, state, owner := v.beginLoad(p)

	go func() {
		mrs, err := v.fetchMRs(ctx, p, state, owner)
		v.g.Update(func(_ *gocui.Gui) error {
			v.apply(seq, mrs, err)

			return nil
		})
	}()
}

// SetProjectSync fetches and applies MRs inline. Test-only entry point that
// mirrors ReposView.LoadSync — avoids the goroutine+g.Update hop so tests
// running without MainLoop see state deterministically.
func (v *MRsView) SetProjectSync(ctx context.Context, p *models.Project) error {
	if p == nil {
		return nil
	}
	seq, state, owner := v.beginLoad(p)

	mrs, err := v.fetchMRs(ctx, p, state, owner)
	v.apply(seq, mrs, err)

	return err
}

// beginLoad commits the start of a fetch under the mutex and returns the
// snapshot that the fetch goroutine will operate on. Taking the filter values
// out of the goroutine is a hard race-safety requirement — the cycle handlers
// mutate v.stateFilter/v.ownerFilter on the main loop thread, so reading them
// from a background goroutine would race.
func (v *MRsView) beginLoad(p *models.Project) (seq uint64, state models.MRStateFilter, owner models.MROwnerFilter) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.current = p
	v.loading = true
	v.loadErr = nil
	v.all = nil
	v.allLower = nil
	v.filtered = nil
	v.cursor = 0
	v.loadSeq++

	return v.loadSeq, v.stateFilter, v.ownerFilter
}

func (v *MRsView) fetchMRs(
	ctx context.Context,
	p *models.Project,
	state models.MRStateFilter,
	owner models.MROwnerFilter,
) ([]*models.MergeRequest, error) {
	opts := gitlab.ListMergeRequestsOptions{
		ProjectID:   p.ID,
		ProjectPath: p.PathWithNamespace,
		State:       state,
	}

	switch owner {
	case models.MROwnerFilterMine, models.MROwnerFilterReviewer:
		u, err := v.app.GitLab.GetCurrentUser(ctx)
		if err != nil {
			return nil, fmt.Errorf("fetch current user: %w", err)
		}
		if u != nil {
			id := u.ID
			if owner == models.MROwnerFilterMine {
				opts.AuthorID = &id
			} else {
				opts.ReviewerID = &id
			}
		}
	}

	mrs, err := v.app.GitLab.ListMergeRequests(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("fetch merge requests: %w", err)
	}

	return mrs, nil
}

func (v *MRsView) apply(seq uint64, mrs []*models.MergeRequest, loadErr error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if seq != v.loadSeq {
		return // a newer load superseded this one
	}
	v.loading = false
	if loadErr != nil {
		v.loadErr = loadErr

		return
	}
	v.loadErr = nil
	v.all = mrs
	v.rebuildLowerLocked()
	v.applyFilterLocked()
	if v.cursor >= len(v.filtered) {
		v.cursor = 0
	}
}

// CurrentProject returns the project currently driving this view, or nil.
func (v *MRsView) CurrentProject() *models.Project {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.current
}

// SearchActive reports whether the mrs search input is being edited.
func (v *MRsView) SearchActive() bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.searchActive
}

// Query returns the current filter query.
func (v *MRsView) Query() string {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.query
}

// StateFilter returns the active state filter.
func (v *MRsView) StateFilter() models.MRStateFilter {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.stateFilter
}

// OwnerFilter returns the active owner filter.
func (v *MRsView) OwnerFilter() models.MROwnerFilter {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.ownerFilter
}

// SelectedMR returns the merge request under the cursor, or nil.
func (v *MRsView) SelectedMR() *models.MergeRequest {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.cursor < 0 || v.cursor >= len(v.filtered) {
		return nil
	}

	return v.filtered[v.cursor]
}

// Render paints the mrs pane. Must be called from the layout callback.
func (v *MRsView) Render(pane *gocui.View) {
	v.mu.Lock()
	defer v.mu.Unlock()

	pane.Clear()
	pane.Highlight = true
	pane.SelBgColor = gocui.ColorGreen
	pane.SelFgColor = gocui.ColorBlack

	switch {
	case v.current == nil:
		pane.WriteString(mrsStatusNoProject + "\n")

		return
	case v.loading:
		pane.WriteString(mrsStatusLoading + "\n")

		return
	case v.loadErr != nil:
		pane.WriteString(fmt.Sprintf("Error loading merge requests: %v\n", v.loadErr))

		return
	case len(v.filtered) == 0:
		pane.WriteString(v.bannerLine)
		pane.WriteString(mrsStatusEmpty + "\n")

		return
	}

	pane.WriteString(v.bannerLine)
	var sb strings.Builder
	sb.Grow(len(v.filtered) * 48)
	for _, mr := range v.filtered {
		fmt.Fprintf(&sb, "!%d %s %s  %s\n",
			mr.IID, mrStateLetter(mr.State), mr.Author.Username, mr.Title,
		)
	}
	pane.WriteString(sb.String())

	// Offset cursor by one to account for the banner line so vim nav lands on data rows.
	if v.cursor >= 0 && v.cursor < len(v.filtered) {
		placeCursor(pane, v.cursor+1, len(v.filtered)+1)
	}
}

// Bindings returns the per-view keybindings owned by the mrs pane and its
// ephemeral search input.
func (v *MRsView) Bindings() []keymap.Binding {
	return []keymap.Binding{
		{View: keymap.ViewMRs, Key: 'j', Handler: v.handleDown},
		{View: keymap.ViewMRs, Key: 'k', Handler: v.handleUp},
		{View: keymap.ViewMRs, Key: 'g', Handler: v.handleTop},
		{View: keymap.ViewMRs, Key: 'G', Handler: v.handleBottom},
		{View: keymap.ViewMRs, Key: 's', Handler: v.handleCycleState},
		{View: keymap.ViewMRs, Key: 'o', Handler: v.handleCycleOwner},
		{View: keymap.ViewMRs, Key: '/', Handler: v.handleOpenSearch},
		{View: keymap.ViewMRs, Key: gocui.KeyEsc, Handler: v.handleClearFilter},
		{View: keymap.ViewMRsSearch, Key: gocui.KeyEnter, Handler: v.handleSubmitSearch},
		{View: keymap.ViewMRsSearch, Key: gocui.KeyEsc, Handler: v.handleCancelSearch},
	}
}

func (v *MRsView) handleDown(_ *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cursor+1 < len(v.filtered) {
		v.cursor++
	}

	return nil
}

func (v *MRsView) handleUp(_ *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cursor > 0 {
		v.cursor--
	}

	return nil
}

func (v *MRsView) handleTop(_ *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cursor = 0

	return nil
}

func (v *MRsView) handleBottom(_ *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if n := len(v.filtered); n > 0 {
		v.cursor = n - 1
	}

	return nil
}

func (v *MRsView) handleCycleState(_ *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	v.stateFilter = nextStateFilter(v.stateFilter)
	v.bannerLine = renderBannerLine(v.stateFilter, v.ownerFilter)
	p := v.current
	v.mu.Unlock()

	if p != nil {
		v.SetProject(context.Background(), p)
	}

	return nil
}

func (v *MRsView) handleCycleOwner(_ *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	v.ownerFilter = nextOwnerFilter(v.ownerFilter)
	v.bannerLine = renderBannerLine(v.stateFilter, v.ownerFilter)
	p := v.current
	v.mu.Unlock()

	if p != nil {
		v.SetProject(context.Background(), p)
	}

	return nil
}

func (v *MRsView) handleOpenSearch(g *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	v.searchActive = true
	v.mu.Unlock()

	g.Update(func(g *gocui.Gui) error {
		if _, err := g.SetCurrentView(keymap.ViewMRsSearch); err != nil {
			if !goerrors.Is(err, gocui.ErrUnknownView) {
				return fmt.Errorf("focus mrs search view: %w", err)
			}
		}

		return nil
	})

	return nil
}

func (v *MRsView) handleSubmitSearch(g *gocui.Gui, searchV *gocui.View) error {
	q := strings.TrimSpace(strings.TrimRight(searchV.Buffer(), "\n"))

	v.mu.Lock()
	v.query = q
	v.searchActive = false
	v.cursor = 0
	v.applyFilterLocked()
	v.mu.Unlock()

	return refocusMRs(g)
}

// handleClearFilter is the Esc handler on the mrs pane itself (after a query
// has been submitted and focus returned to the list). No-op with no query.
func (v *MRsView) handleClearFilter(_ *gocui.Gui, _ *gocui.View) error {
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

func (v *MRsView) handleCancelSearch(g *gocui.Gui, _ *gocui.View) error {
	v.mu.Lock()
	v.query = ""
	v.searchActive = false
	v.cursor = 0
	v.applyFilterLocked()
	v.mu.Unlock()

	return refocusMRs(g)
}

func (v *MRsView) rebuildLowerLocked() {
	v.allLower = make([]string, len(v.all))
	for i, mr := range v.all {
		v.allLower[i] = strings.ToLower(mr.Title + " " + mr.Author.Username)
	}
}

func (v *MRsView) applyFilterLocked() {
	if v.query == "" {
		v.filtered = append(v.filtered[:0], v.all...)

		return
	}
	needle := strings.ToLower(v.query)
	v.filtered = v.filtered[:0]
	for i, mr := range v.all {
		if strings.Contains(v.allLower[i], needle) {
			v.filtered = append(v.filtered, mr)
		}
	}
}

func refocusMRs(g *gocui.Gui) error {
	if err := g.DeleteView(keymap.ViewMRsSearch); err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
		return fmt.Errorf("delete mrs search view: %w", err)
	}
	if _, err := g.SetCurrentView(keymap.ViewMRs); err != nil {
		return fmt.Errorf("refocus mrs: %w", err)
	}

	return nil
}

// renderBannerLine builds the filter-status line shown at the top of the
// mrs pane. Called only when the filters change (constructor + cycle
// handlers); the result is cached in MRsView.bannerLine so Render doesn't
// allocate a fresh string on every layout tick.
func renderBannerLine(state models.MRStateFilter, owner models.MROwnerFilter) string {
	return fmt.Sprintf("[state=%s owner=%s]\n", state, owner)
}

func mrStateLetter(s models.MRState) string {
	switch s {
	case models.MRStateOpened:
		return "O"
	case models.MRStateMerged:
		return "M"
	case models.MRStateClosed:
		return "C"
	}

	return "?"
}

func nextStateFilter(f models.MRStateFilter) models.MRStateFilter {
	switch f {
	case models.MRStateFilterOpened:
		return models.MRStateFilterMerged
	case models.MRStateFilterMerged:
		return models.MRStateFilterClosed
	case models.MRStateFilterClosed:
		return models.MRStateFilterAll
	}

	return models.MRStateFilterOpened
}

func nextOwnerFilter(f models.MROwnerFilter) models.MROwnerFilter {
	switch f {
	case models.MROwnerFilterAll:
		return models.MROwnerFilterMine
	case models.MROwnerFilterMine:
		return models.MROwnerFilterReviewer
	}

	return models.MROwnerFilterAll
}
