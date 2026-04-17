package views

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
)

const (
	detailStatusEmpty = "Select a merge request."
	detailDateFormat  = "2006-01-02 15:04"
	detailBranchesSep = " \u2192 "

	// ANSI SGR escapes; gocui was built with OutputTrue (see internal/tui/app.go)
	// and forwards escape sequences in the pane buffer to the terminal.
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiReverse = "\x1b[7m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiCyan    = "\x1b[36m"

	iconOK   = "\u2713" // ✓
	iconBad  = "\u2717" // ✗
	iconWarn = "\u26A0" // ⚠

	detailNoConflicts  = ansiGreen + iconOK + " No conflicts" + ansiReset
	detailHasConflicts = ansiRed + iconBad + " Has conflicts" + ansiReset

	detailTabSeparator = " | "

	detailDiffLoading      = "Loading diff…"
	detailDiffEmpty        = "No files changed in this merge request."
	detailConversationStub = "Conversation tab — not yet implemented."
	detailPipelineStub     = "Pipeline tab — not yet implemented."
)

// DetailTab identifies which sub-tab the detail pane is currently showing.
// The Overview → Diff → Conversation → Pipeline cycle mirrors the Python UI.
type DetailTab int

const (
	DetailTabOverview DetailTab = iota
	DetailTabDiff
	DetailTabConversation
	DetailTabPipeline

	detailTabCount = 4
)

var detailTabLabels = [detailTabCount]string{
	DetailTabOverview:     "Overview",
	DetailTabDiff:         "Diff",
	DetailTabConversation: "Conversation",
	DetailTabPipeline:     "Pipeline",
}

// DetailView renders the Details pane for the selected merge request and
// its sub-tabs (Overview, Diff, stubs for Conversation/Pipeline).
//
// Overview body is cached on SetMR / discussion-stats arrival (MRsView
// pattern) so the layout tick doesn't allocate a fresh builder per redraw.
// The Diff tab owns two child widgets — diffTree + diffContent — rendered
// into ephemeral sub-panes mounted by the tui layout package when
// currentTab == DetailTabDiff. Diff state is reset on SetMR via diffSeq so
// switching MRs mid-fetch cannot cross-pollinate results.
//
// Lock ordering: DetailView.mu → diffTree.mu → diffContent.mu. applyDiff
// is called with DetailView.mu held and calls diffTree.SelectedFile(),
// which acquires diffTree.mu. Never invert this order.
type DetailView struct {
	g   *gocui.Gui
	app *appcontext.AppContext

	mu       sync.Mutex
	mr       *models.MergeRequest
	stats    *models.DiscussionStats
	cached   string
	statsSeq uint64

	currentTab   DetailTab
	pendingFocus string
	tabBar       string

	diffTree    *DiffTreeView
	diffContent *DiffContentView
	diffSeq     uint64
	diffLoading bool
	diffErr     error
}

func NewDetail(g *gocui.Gui, app *appcontext.AppContext) *DetailView {
	return &DetailView{
		g:           g,
		app:         app,
		diffTree:    NewDiffTree(),
		diffContent: NewDiffContent(),
		tabBar:      renderTabBar(DetailTabOverview),
	}
}

// ConsumePendingFocus returns and clears the view name a recent tab change
// wants focus to move to. Layout reads this after mounting sub-panes so
// the focus shift survives the "pane does not exist yet" race — sub-panes
// only exist after manageDiffSubpanes runs, which happens after the tab
// handler fires.
func (d *DetailView) ConsumePendingFocus() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	target := d.pendingFocus
	d.pendingFocus = ""

	return target
}

// SetMR updates the MR driving the pane and kicks off an async discussion
// stats fetch. No fetch happens if project is nil or the view has no
// GitLab client (tests that instantiate DetailView without an app skip it).
func (d *DetailView) SetMR(project *models.Project, mr *models.MergeRequest) {
	seq, projectID, iid, tab := d.commitMR(project, mr)
	if projectID == 0 || d.app == nil || d.app.GitLab == nil || d.g == nil {
		return
	}
	go d.fetchStats(context.Background(), seq, projectID, iid)
	if tab == DetailTabDiff {
		d.fetchDiffAsync(context.Background(), project, mr)
	}
}

// SetMRSync applies an MR and fetches stats inline. Test-only entry point
// that mirrors MRsView.SetProjectSync so tests running without MainLoop
// observe both fields deterministically.
func (d *DetailView) SetMRSync(ctx context.Context, project *models.Project, mr *models.MergeRequest) error {
	seq, projectID, iid, _ := d.commitMR(project, mr)
	if projectID == 0 || d.app == nil || d.app.GitLab == nil {
		return nil
	}
	stats, err := d.app.GitLab.GetMRDiscussionStats(ctx, projectID, iid)
	d.applyStats(seq, stats, err)
	if err != nil {
		return fmt.Errorf("detail: fetch discussion stats: %w", err)
	}

	return nil
}

// CurrentMR returns the MR currently displayed, or nil.
func (d *DetailView) CurrentMR() *models.MergeRequest {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.mr
}

// Stats returns the most recent DiscussionStats, or nil if none loaded.
func (d *DetailView) Stats() *models.DiscussionStats {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.stats
}

// CurrentTab returns the active tab. Callers should not rely on this for
// focus decisions — the tui package derives sub-pane mount state from it.
func (d *DetailView) CurrentTab() DetailTab {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.currentTab
}

// DiffTree returns the Diff-tab's file tree widget. Package-internal use
// for rendering via the layout callback.
func (d *DetailView) DiffTree() *DiffTreeView { return d.diffTree }

// DiffContent returns the Diff-tab's content widget.
func (d *DetailView) DiffContent() *DiffContentView { return d.diffContent }

// SetTab advances the active tab and triggers a diff fetch when switching
// to the Diff tab for the first time on the current MR. Project is the
// owning project (needed for the API call); pass nil to skip the fetch
// (e.g., when the Detail pane holds no MR yet). Focus shift is queued on
// pendingFocus and picked up by the layout once sub-panes exist.
func (d *DetailView) SetTab(tab DetailTab, project *models.Project) {
	if tab < 0 || tab >= detailTabCount {
		return
	}
	d.mu.Lock()
	prev := d.currentTab
	d.currentTab = tab
	d.tabBar = renderTabBar(tab)
	d.pendingFocus = focusTargetForTab(tab)
	mr := d.mr
	d.mu.Unlock()

	if tab == DetailTabDiff && prev != DetailTabDiff && mr != nil {
		d.fetchDiffAsync(context.Background(), project, mr)
	}
}

// SetTabSync is the test entry point. Switches tab + runs the diff fetch
// inline so suites observing post-tab state do not need to spin MainLoop.
func (d *DetailView) SetTabSync(ctx context.Context, tab DetailTab, project *models.Project) error {
	if tab < 0 || tab >= detailTabCount {
		return fmt.Errorf("detail: invalid tab %d", tab)
	}
	d.mu.Lock()
	prev := d.currentTab
	d.currentTab = tab
	d.tabBar = renderTabBar(tab)
	mr := d.mr
	d.mu.Unlock()

	if tab != DetailTabDiff || prev == DetailTabDiff || mr == nil {
		return nil
	}
	if d.app == nil || d.app.GitLab == nil {
		return nil
	}
	projectID := 0
	if project != nil {
		projectID = project.ID
	}
	if projectID == 0 {
		return nil
	}

	seq := d.beginDiffLoad()
	data, err := d.app.GitLab.GetMRChanges(ctx, projectID, mr.IID)
	d.applyDiff(seq, data, err)
	if err != nil {
		return fmt.Errorf("detail: fetch mr changes: %w", err)
	}

	return nil
}

// SelectDiffFile is invoked by the Diff-tab's file-tree on Enter. It pushes
// the selected file into the content widget and resets the scroll origin
// via the gocui view handle (looked up by name — caller doesn't have it).
func (d *DetailView) SelectDiffFile(g *gocui.Gui) {
	if d.diffTree == nil || d.diffContent == nil {
		return
	}
	file := d.diffTree.SelectedFile()
	if file == nil {
		return
	}
	d.diffContent.SetFile(file)
	if g == nil {
		return
	}
	if pv, err := g.View(keymap.ViewDetailDiffContent); err == nil {
		d.diffContent.ScrollToTop(pv)
	}
}

// Render paints the tab bar + (for non-Diff tabs) the active tab's body.
// Must be called from the layout callback.
func (d *DetailView) Render(pane *gocui.View) {
	d.mu.Lock()
	defer d.mu.Unlock()

	pane.Clear()
	pane.WriteString(d.tabBar + "\n")

	if d.mr == nil && d.currentTab != DetailTabOverview {
		pane.WriteString(detailStatusEmpty + "\n")

		return
	}

	switch d.currentTab {
	case DetailTabOverview:
		if d.mr == nil {
			pane.WriteString(detailStatusEmpty + "\n")

			return
		}
		pane.WriteString(d.cached)
	case DetailTabDiff:
	case DetailTabConversation:
		pane.WriteString(detailConversationStub + "\n")
	case DetailTabPipeline:
		pane.WriteString(detailPipelineStub + "\n")
	}
}

func (d *DetailView) commitMR(project *models.Project, mr *models.MergeRequest) (seq uint64, projectID, iid int, tab DetailTab) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.mr = mr
	d.stats = nil
	d.statsSeq++
	d.diffSeq++
	d.diffLoading = false
	d.diffErr = nil
	if d.diffTree != nil {
		d.diffTree.SetFiles(nil)
	}
	if d.diffContent != nil {
		d.diffContent.SetFile(nil)
	}

	if mr == nil {
		d.cached = ""

		return d.statsSeq, 0, 0, d.currentTab
	}
	d.cached = renderOverview(mr, nil)

	pid := 0
	if project != nil {
		pid = project.ID
	}

	return d.statsSeq, pid, mr.IID, d.currentTab
}

func (d *DetailView) fetchStats(ctx context.Context, seq uint64, projectID, iid int) {
	stats, err := d.app.GitLab.GetMRDiscussionStats(ctx, projectID, iid)
	d.g.Update(func(_ *gocui.Gui) error {
		d.applyStats(seq, stats, err)

		return nil
	})
}

func (d *DetailView) applyStats(seq uint64, stats *models.DiscussionStats, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if seq != d.statsSeq || d.mr == nil {
		return
	}
	if err != nil {
		return
	}
	d.stats = stats
	d.cached = renderOverview(d.mr, stats)
}

func (d *DetailView) beginDiffLoad() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.diffSeq++
	d.diffLoading = true
	d.diffErr = nil
	if d.diffContent != nil {
		d.diffContent.ShowLoading()
	}
	if d.diffTree != nil {
		d.diffTree.SetFiles(nil)
	}

	return d.diffSeq
}

func (d *DetailView) fetchDiffAsync(ctx context.Context, project *models.Project, mr *models.MergeRequest) {
	if d.app == nil || d.app.GitLab == nil || d.g == nil || project == nil || mr == nil {
		return
	}
	seq := d.beginDiffLoad()
	projectID := project.ID
	iid := mr.IID
	go func() {
		data, err := d.app.GitLab.GetMRChanges(ctx, projectID, iid)
		d.g.Update(func(_ *gocui.Gui) error {
			d.applyDiff(seq, data, err)

			return nil
		})
	}()
}

func (d *DetailView) applyDiff(seq uint64, data *models.MRDiffData, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if seq != d.diffSeq {
		return
	}
	d.diffLoading = false
	if err != nil {
		d.diffErr = err
		if d.diffTree != nil {
			d.diffTree.ShowError(err.Error())
		}
		if d.diffContent != nil {
			d.diffContent.ShowError(err.Error())
		}

		return
	}
	files := []models.MRDiffFile{}
	if data != nil {
		files = data.Files
	}
	if d.diffTree != nil {
		d.diffTree.SetFiles(files)
	}
	if d.diffContent != nil {
		if len(files) == 0 {
			d.diffContent.SetFile(nil)
			d.diffContent.ShowError(detailDiffEmpty)
		} else {
			selected := d.diffTree.SelectedFile()
			if selected == nil {
				selected = &files[0]
			}
			d.diffContent.SetFile(selected)
		}
	}
}

func focusTargetForTab(tab DetailTab) string {
	if tab == DetailTabDiff {
		return keymap.ViewDetailDiffTree
	}

	return keymap.ViewDetail
}

func renderTabBar(active DetailTab) string {
	var sb strings.Builder
	sb.Grow(64)
	for i, label := range detailTabLabels {
		if i > 0 {
			sb.WriteString(detailTabSeparator)
		}
		if DetailTab(i) == active {
			sb.WriteString(ansiReverse)
			sb.WriteString(label)
			sb.WriteString(ansiReset)
		} else {
			sb.WriteString(label)
		}
	}

	return sb.String()
}

func renderOverview(mr *models.MergeRequest, stats *models.DiscussionStats) string {
	var sb strings.Builder
	sb.Grow(256)

	fmt.Fprintf(&sb, "!%d %s\n\n", mr.IID, mr.Title)
	fmt.Fprintf(&sb, "Author:   @%s\n", mr.Author.Username)
	fmt.Fprintf(&sb, "Created:  %s\n", mr.CreatedAt.Format(detailDateFormat))
	fmt.Fprintf(&sb, "Status:   %s %s\n", mrStateLetter(mr.State), mr.State)
	fmt.Fprintf(&sb, "Branches: %s%s%s\n", mr.SourceBranch, detailBranchesSep, mr.TargetBranch)
	fmt.Fprintf(&sb, "Conflicts: %s\n", conflictText(mr.HasConflicts))
	fmt.Fprintf(&sb, "Comments: %s\n", commentsText(mr.UserNotesCount, stats))

	return sb.String()
}

// commentsText formats the comment count, appending a resolved-thread
// breakdown when stats carry any resolvable discussions. Mirrors Python
// `_comments_text` — `N` on its own when nothing is resolvable,
// `N ✓ (X/X resolved)` in green when every resolvable thread is resolved,
// `N ⚠ (X/Y resolved)` in yellow when some remain unresolved.
func commentsText(notesCount int, stats *models.DiscussionStats) string {
	if stats == nil || stats.TotalResolvable == 0 {
		return fmt.Sprintf("%d", notesCount)
	}

	color, icon := ansiYellow, iconWarn
	if stats.Resolved == stats.TotalResolvable {
		color, icon = ansiGreen, iconOK
	}

	return fmt.Sprintf("%d %s%s (%d/%d resolved)%s",
		notesCount, color, icon, stats.Resolved, stats.TotalResolvable, ansiReset,
	)
}

func conflictText(hasConflicts bool) string {
	if hasConflicts {
		return detailHasConflicts
	}

	return detailNoConflicts
}
