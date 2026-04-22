package views

import (
	"context"
	"fmt"
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
	mrsPaneTitle       = "[2] Merge Requests"
	mrsStatusNoProject = "Select a project to view merge requests."
	mrsStatusLoading   = "Loading merge requests…"
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
	// lastSync is the wall-clock time of the most recent successful MR
	// list commit. Footer renders "last sync <N>s ago" off this value.
	// Zero when no successful load has completed yet.
	lastSync time.Time
	loadSeq  uint64
	// cancelLoad aborts the in-flight fetch when a newer SetProject
	// supersedes it. Without this, rapid filter cycling (s/o) piles up
	// goroutines that loadSeq discards the results of but can't stop from
	// running to completion against the GitLab API.
	cancelLoad context.CancelFunc
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

	return v
}

// SetProject is called when the repos pane commits a project selection.
// It kicks off an async fetch; results are applied via g.Update so all state
// mutation happens on the main loop thread.
func (v *MRsView) SetProject(ctx context.Context, p *models.Project) {
	if p == nil {
		return
	}
	fetchCtx, seq, state, owner := v.beginLoad(ctx, p)

	go func() {
		mrs, err := v.fetchMRs(fetchCtx, p, state, owner)
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
	fetchCtx, seq, state, owner := v.beginLoad(ctx, p)

	mrs, err := v.fetchMRs(fetchCtx, p, state, owner)
	v.apply(seq, mrs, err)

	return err
}

// beginLoad commits the start of a fetch under the mutex and returns the
// snapshot that the fetch goroutine will operate on plus a cancellable child
// context. Taking the filter values out of the goroutine is a hard race-
// safety requirement — the cycle handlers mutate v.stateFilter/v.ownerFilter
// on the main loop thread, so reading them from a background goroutine would
// race. The previous fetch's cancel is invoked so rapid key presses (s/o)
// abort in-flight HTTP calls instead of leaking goroutines.
func (v *MRsView) beginLoad(
	parent context.Context,
	p *models.Project,
) (ctx context.Context, seq uint64, state models.MRStateFilter, owner models.MROwnerFilter) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cancelLoad != nil {
		v.cancelLoad()
	}
	ctx, v.cancelLoad = context.WithCancel(parent)
	v.current = p
	v.loading = true
	v.loadErr = nil
	v.all = nil
	v.allLower = nil
	v.filtered = nil
	v.cursor = 0
	v.loadSeq++

	return ctx, v.loadSeq, v.stateFilter, v.ownerFilter
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
	v.lastSync = time.Now()
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

// LastSync returns the wall-clock of the most recent successful list
// commit, or the zero time when no load has succeeded yet. The footer
// renders "last sync <N>s ago" off this value.
func (v *MRsView) LastSync() time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.lastSync
}

// CursorInfo returns the 1-based cursor position and total row count in
// the filtered list. Both are zero when no MRs are loaded or the pane has
// no selection — the footer suppresses the "N/M" segment in that case.
func (v *MRsView) CursorInfo() (index, total int) {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.cursorInfoLocked()
}

// FooterSnap is a single-lock read of every field the global footer
// needs from MRsView. Without this, FooterView rebuilds its state with
// four separate lock acquisitions per tick — and the snapshot could
// straddle a mid-flight apply() commit (MRIID from one generation,
// index/total from the next). Caller must not mutate Selected or Project.
type FooterSnap struct {
	Project  *models.Project
	Selected *models.MergeRequest
	Index    int
	Total    int
	LastSync time.Time
}

func (v *MRsView) FooterSnap() FooterSnap {
	v.mu.Lock()
	defer v.mu.Unlock()

	snap := FooterSnap{
		Project:  v.current,
		LastSync: v.lastSync,
	}
	snap.Index, snap.Total = v.cursorInfoLocked()
	if snap.Index > 0 && snap.Index <= len(v.filtered) {
		snap.Selected = v.filtered[snap.Index-1]
	}

	return snap
}

func (v *MRsView) cursorInfoLocked() (index, total int) {
	total = len(v.filtered)
	if v.cursor < 0 || v.cursor >= total {
		return 0, total
	}

	return v.cursor + 1, total
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
	pane.SelBgColor = theme.ColorAccent
	pane.SelFgColor = theme.ColorSelectionFg

	switch {
	case v.current == nil:
		pane.WriteString(mrsPaneTitle + "\n\n" + dim(" "+mrsStatusNoProject) + "\n")

		return
	case v.loading:
		pane.WriteString(mrsPaneTitle + "\n\n" + dim(" "+mrsStatusLoading) + "\n")

		return
	case v.loadErr != nil:
		pane.WriteString(mrsPaneTitle + "\n\n" +
			theme.Wrap(theme.FgErr, fmt.Sprintf(" ✕ Error loading merge requests: %v", v.loadErr)) + "\n")

		return
	case len(v.filtered) == 0:
		pane.WriteString(mrsHeader(0, len(v.all), v.stateFilter, v.ownerFilter))
		writeMRsEmptyState(pane, v.stateFilter, v.ownerFilter)

		return
	}

	pane.WriteString(mrsHeader(len(v.filtered), len(v.all), v.stateFilter, v.ownerFilter))

	innerW, _ := pane.InnerSize()
	for _, mr := range v.filtered {
		glyph, color := mrStateGlyph(mr.State, mr.IsDraft())
		icon := theme.Wrap(color, glyph)
		// All state glyphs are single-cell per _helpers.js / layout.js.
		pane.WriteString(formatMRRow(icon, mr.IID, mr.Title, mr.Author.Username, innerW) + "\n")
	}

	if v.cursor >= 0 && v.cursor < len(v.filtered) {
		placeCursor(pane, v.cursor+1, len(v.filtered)+1)
	}
}

// mrsHeader builds the dim "[2] Merge Requests · state:X · owner:Y · N/M"
// row that prefixes every populated render. Computed per render — at typical
// list sizes the cost is below the noise floor of the gocui paint, and the
// alternative (caching it on the view) requires careful invalidation across
// every mutator.
func mrsHeader(filtered, total int, state models.MRStateFilter, owner models.MROwnerFilter) string {
	meta := fmt.Sprintf(" · state:%s · owner:%s · %d/%d", state, owner, filtered, total)

	return mrsPaneTitle + dim(meta) + "\n"
}

// writeMRsEmptyState renders the dim "no MRs match …" hint with the live
// filter values and the keys that change them, mirroring states.js.
func writeMRsEmptyState(pane *gocui.View, state models.MRStateFilter, owner models.MROwnerFilter) {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(dim(fmt.Sprintf(" No MRs match state:%s owner:%s", state, owner)) + "\n\n")
	sb.WriteString(dim(" Press ") + accent("S") + dim(" or ") + accent("O") + dim(" to change filters,") + "\n")
	sb.WriteString(dim(" or ") + accent("R") + dim(" to refresh.") + "\n")
	pane.WriteString(sb.String())
}

// mrStateGlyph maps an MR state plus draft flag to a coloured glyph per the
// design palette. Draft overrides the underlying state — a draft of any
// state still renders as the half-circle in the draft tone.
func mrStateGlyph(state models.MRState, draft bool) (glyph, color string) {
	if draft {
		return "◐", theme.FgDraft
	}
	switch state {
	case models.MRStateOpened:
		return "●", theme.FgOK
	case models.MRStateMerged:
		return "✓", theme.FgMerged
	case models.MRStateClosed:
		return "✕", theme.FgErr
	}

	return "?", theme.FgDraft
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
