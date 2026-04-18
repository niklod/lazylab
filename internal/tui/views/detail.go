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
// Lock ordering: DetailView.mu → {diffTree, diffContent, pipelineStages,
// jobLog}.mu. Child widgets own their own mutex and never call back into
// DetailView, so the parent can hold d.mu while invoking their methods
// (applyDiff / applyPipeline / applyTrace do this). Never invert the
// order: a child widget must not acquire DetailView.mu.
type DetailView struct {
	g   *gocui.Gui
	app *appcontext.AppContext

	mu       sync.Mutex
	mr       *models.MergeRequest
	stats    *models.DiscussionStats
	cached   string
	statsSeq uint64

	approvals    *models.ApprovalStatus
	approvalsSeq uint64

	currentTab   DetailTab
	pendingFocus string
	tabBar       string

	diffTree    *DiffTreeView
	diffContent *DiffContentView
	diffSeq     uint64
	diffLoading bool
	diffErr     error
	diffStats   *models.DiffStats

	pipelineStages  *PipelineStagesView
	jobLog          *JobLogView
	pipelineDetail  *models.PipelineDetail
	pipelineSeq     uint64
	pipelineLoading bool
	pipelineErr     error

	logOpen    bool
	logJob     *models.PipelineJob
	logSeq     uint64
	logLoading bool
	logErr     error
}

func NewDetail(g *gocui.Gui, app *appcontext.AppContext) *DetailView {
	return &DetailView{
		g:              g,
		app:            app,
		diffTree:       NewDiffTree(),
		diffContent:    NewDiffContent(),
		pipelineStages: NewPipelineStages(),
		jobLog:         NewJobLog(),
		tabBar:         renderTabBar(DetailTabOverview),
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
	statsSeq, approvalsSeq, projectID, iid := d.commitMR(project, mr)
	if projectID == 0 || d.app == nil || d.app.GitLab == nil || d.g == nil {
		return
	}
	go d.fetchStats(context.Background(), statsSeq, projectID, iid)
	go d.fetchApprovals(context.Background(), approvalsSeq, projectID, iid)
	// Always prefetch the diff — Overview shows a +N -M line derived from
	// the same payload the Diff tab consumes, so the fetch doubles as the
	// stats source. cache.Do dedupes when the user actually opens Diff.
	d.fetchDiffAsync(context.Background(), project, mr)
}

// SetMRSync applies an MR and fetches stats inline. Test-only entry point
// that mirrors MRsView.SetProjectSync so tests running without MainLoop
// observe both fields deterministically.
func (d *DetailView) SetMRSync(ctx context.Context, project *models.Project, mr *models.MergeRequest) error {
	statsSeq, approvalsSeq, projectID, iid := d.commitMR(project, mr)
	if projectID == 0 || d.app == nil || d.app.GitLab == nil {
		return nil
	}
	stats, err := d.app.GitLab.GetMRDiscussionStats(ctx, projectID, iid)
	d.applyStats(statsSeq, stats, err)
	if err != nil {
		return fmt.Errorf("detail: fetch discussion stats: %w", err)
	}

	approvals, err := d.app.GitLab.GetMRApprovals(ctx, projectID, iid)
	d.applyApprovals(approvalsSeq, approvals, err)
	if err != nil {
		return fmt.Errorf("detail: fetch mr approvals: %w", err)
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

// Approvals returns the most recent ApprovalStatus, or nil if none loaded.
func (d *DetailView) Approvals() *models.ApprovalStatus {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.approvals
}

// DiffStatsSnapshot returns the most recent added/removed line counts, or
// nil if the diff has not been fetched yet.
func (d *DetailView) DiffStatsSnapshot() *models.DiffStats {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.diffStats
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

// PipelineStages returns the Pipeline-tab stages widget.
func (d *DetailView) PipelineStages() *PipelineStagesView { return d.pipelineStages }

// JobLog returns the Pipeline-tab job log widget.
func (d *DetailView) JobLog() *JobLogView { return d.jobLog }

// LogOpen reports whether the inline job log is currently mounted.
// Layout reads this to decide whether the stages pane or the log pane
// owns the Pipeline-tab rect.
func (d *DetailView) LogOpen() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.logOpen
}

// PipelineDetailSnapshot returns the most recent pipeline detail fetched
// for the current MR, or nil. Exposed for tests.
func (d *DetailView) PipelineDetailSnapshot() *models.PipelineDetail {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.pipelineDetail
}

// SetTab advances the active tab and triggers a diff fetch when switching
// to the Diff tab for the first time on the current MR. Project is the
// owning project (needed for the API call); pass nil to skip the fetch
// (e.g., when the Detail pane holds no MR yet). Focus shift is queued on
// pendingFocus and picked up by the layout once sub-panes exist.
//
// Leaving the Pipeline tab with the inline job log open resets the log
// state so re-entering Pipeline always starts on the stages pane. Without
// this reset, focusTargetForTab(Pipeline) would point to the stages pane
// while managePipelineSubpanes still mounts the log pane — resulting in
// a wrapped ErrUnknownView from SetCurrentView on the next layout tick.
func (d *DetailView) SetTab(tab DetailTab, project *models.Project) {
	if tab < 0 || tab >= detailTabCount {
		return
	}
	d.mu.Lock()
	prev := d.currentTab
	d.currentTab = tab
	d.tabBar = renderTabBar(tab)
	if prev == DetailTabPipeline && tab != DetailTabPipeline && d.logOpen {
		d.resetJobLogLocked()
	}
	d.pendingFocus = focusTargetForTab(tab)
	mr := d.mr
	d.mu.Unlock()

	if tab == DetailTabDiff && prev != DetailTabDiff && mr != nil {
		d.fetchDiffAsync(context.Background(), project, mr)
	}
	if tab == DetailTabPipeline && prev != DetailTabPipeline && mr != nil {
		d.fetchPipelineAsync(context.Background(), project, mr)
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
	if prev == DetailTabPipeline && tab != DetailTabPipeline && d.logOpen {
		d.resetJobLogLocked()
	}
	mr := d.mr
	d.mu.Unlock()

	if mr == nil || d.app == nil || d.app.GitLab == nil {
		return nil
	}
	projectID := 0
	if project != nil {
		projectID = project.ID
	}
	if projectID == 0 {
		return nil
	}

	if tab == DetailTabDiff && prev != DetailTabDiff {
		seq := d.beginDiffLoad()
		data, err := d.app.GitLab.GetMRChanges(ctx, projectID, mr.IID)
		d.applyDiff(seq, data, err)
		if err != nil {
			return fmt.Errorf("detail: fetch mr changes: %w", err)
		}
	}

	if tab == DetailTabPipeline && prev != DetailTabPipeline {
		seq := d.beginPipelineLoad()
		detail, err := d.app.GitLab.GetMRPipelineDetail(ctx, projectID, mr.IID)
		d.applyPipeline(seq, detail, err)
		if err != nil {
			return fmt.Errorf("detail: fetch mr pipeline: %w", err)
		}
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
	}
}

func (d *DetailView) commitMR(project *models.Project, mr *models.MergeRequest) (statsSeq, approvalsSeq uint64, projectID, iid int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.mr = mr
	d.stats = nil
	d.approvals = nil
	d.statsSeq++
	d.approvalsSeq++
	d.diffSeq++
	d.diffLoading = false
	d.diffErr = nil
	d.diffStats = nil
	if d.diffTree != nil {
		d.diffTree.SetFiles(nil)
	}
	if d.diffContent != nil {
		d.diffContent.SetFile(nil)
	}

	d.pipelineSeq++
	d.pipelineDetail = nil
	d.pipelineLoading = false
	d.pipelineErr = nil
	if d.pipelineStages != nil {
		d.pipelineStages.ShowLoading()
	}

	d.logSeq++
	d.logOpen = false
	d.logJob = nil
	d.logLoading = false
	d.logErr = nil
	if d.jobLog != nil {
		d.jobLog.SetJob(nil, "")
	}

	d.cached = d.renderOverviewLocked()

	pid := 0
	if project != nil {
		pid = project.ID
	}
	if mr == nil {
		return d.statsSeq, d.approvalsSeq, 0, 0
	}

	return d.statsSeq, d.approvalsSeq, pid, mr.IID
}

// renderOverviewLocked rebuilds the cached Overview body from the current
// state fields. Caller must hold d.mu. Returns "" when no MR is selected —
// the Render path surfaces detailStatusEmpty in that case.
func (d *DetailView) renderOverviewLocked() string {
	if d.mr == nil {
		return ""
	}

	return renderOverview(d.mr, d.stats, d.diffStats, d.approvals)
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

	if err != nil || seq != d.statsSeq || d.mr == nil {
		return
	}
	d.stats = stats
	d.cached = d.renderOverviewLocked()
}

func (d *DetailView) fetchApprovals(ctx context.Context, seq uint64, projectID, iid int) {
	approvals, err := d.app.GitLab.GetMRApprovals(ctx, projectID, iid)
	d.g.Update(func(_ *gocui.Gui) error {
		d.applyApprovals(seq, approvals, err)

		return nil
	})
}

func (d *DetailView) applyApprovals(seq uint64, approvals *models.ApprovalStatus, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err != nil || seq != d.approvalsSeq || d.mr == nil {
		return
	}
	d.approvals = approvals
	d.cached = d.renderOverviewLocked()
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
	stats := data.Stats()
	d.diffStats = &stats
	d.cached = d.renderOverviewLocked()
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

func (d *DetailView) beginPipelineLoad() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.pipelineSeq++
	d.pipelineLoading = true
	d.pipelineErr = nil
	if d.pipelineStages != nil {
		d.pipelineStages.ShowLoading()
	}

	return d.pipelineSeq
}

func (d *DetailView) fetchPipelineAsync(ctx context.Context, project *models.Project, mr *models.MergeRequest) {
	if d.app == nil || d.app.GitLab == nil || d.g == nil || project == nil || mr == nil {
		return
	}
	seq := d.beginPipelineLoad()
	projectID := project.ID
	iid := mr.IID
	go func() {
		detail, err := d.app.GitLab.GetMRPipelineDetail(ctx, projectID, iid)
		d.g.Update(func(_ *gocui.Gui) error {
			d.applyPipeline(seq, detail, err)

			return nil
		})
	}()
}

func (d *DetailView) applyPipeline(seq uint64, detail *models.PipelineDetail, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if seq != d.pipelineSeq {
		return
	}
	d.pipelineLoading = false
	if err != nil {
		d.pipelineErr = err
		if d.pipelineStages != nil {
			d.pipelineStages.ShowError(err.Error())
		}

		return
	}
	d.pipelineDetail = detail
	if d.pipelineStages != nil {
		d.pipelineStages.SetDetail(detail)
	}
}

// OpenJobLog mounts the inline log pane for the job under the stages
// cursor, fetches the trace asynchronously, and queues focus to shift to
// the log pane. No-op if no job is selected (cursor on a header, which
// should not occur after SetDetail).
func (d *DetailView) OpenJobLog(project *models.Project) {
	if d.pipelineStages == nil {
		return
	}
	job := d.pipelineStages.SelectedJob()
	if job == nil {
		return
	}

	if d.app == nil || d.app.GitLab == nil || d.g == nil || project == nil {
		return
	}
	seq := d.beginJobLogLoad(job)
	projectID := project.ID
	jobID := job.ID
	go func() {
		trace, err := d.app.GitLab.GetJobTrace(context.Background(), projectID, jobID)
		d.g.Update(func(_ *gocui.Gui) error {
			d.applyJobTrace(seq, job, trace, err)

			return nil
		})
	}()
}

// OpenJobLogSync is the test entry point — fetches the trace inline so
// tests running without MainLoop observe the populated log deterministically.
func (d *DetailView) OpenJobLogSync(ctx context.Context, project *models.Project) error {
	if d.pipelineStages == nil {
		return nil
	}
	job := d.pipelineStages.SelectedJob()
	if job == nil {
		return nil
	}

	if d.app == nil || d.app.GitLab == nil || project == nil {
		return nil
	}
	seq := d.beginJobLogLoad(job)
	trace, err := d.app.GitLab.GetJobTrace(ctx, project.ID, job.ID)
	d.applyJobTrace(seq, job, trace, err)
	if err != nil {
		return fmt.Errorf("detail: fetch job trace: %w", err)
	}

	return nil
}

// CloseJobLog unmounts the log pane and queues focus back to the stages
// pane. Idempotent — no-op if the log is not open.
func (d *DetailView) CloseJobLog() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.logOpen {
		return
	}
	d.resetJobLogLocked()
	d.pendingFocus = keymap.ViewDetailPipelineStages
}

// resetJobLogLocked clears the inline-log state. Caller must hold d.mu.
// Does NOT touch pendingFocus — callers that need to hand focus somewhere
// set it themselves (CloseJobLog → stages; SetTab → whatever the new tab
// requests).
func (d *DetailView) resetJobLogLocked() {
	d.logOpen = false
	d.logJob = nil
	d.logLoading = false
	d.logErr = nil
	d.logSeq++
	if d.jobLog != nil {
		d.jobLog.SetJob(nil, "")
	}
}

func (d *DetailView) beginJobLogLoad(job *models.PipelineJob) uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logSeq++
	d.logOpen = true
	d.logJob = job
	d.logLoading = true
	d.logErr = nil
	d.pendingFocus = keymap.ViewDetailPipelineJobLog
	if d.jobLog != nil {
		d.jobLog.ShowLoading()
	}

	return d.logSeq
}

func (d *DetailView) applyJobTrace(seq uint64, job *models.PipelineJob, trace string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if seq != d.logSeq || !d.logOpen {
		return
	}
	d.logLoading = false
	if err != nil {
		d.logErr = err
		if d.jobLog != nil {
			d.jobLog.ShowError(err.Error())
		}

		return
	}
	if d.jobLog != nil {
		d.jobLog.SetJob(job, trace)
	}
}

func focusTargetForTab(tab DetailTab) string {
	switch tab {
	case DetailTabDiff:
		return keymap.ViewDetailDiffTree
	case DetailTabPipeline:
		return keymap.ViewDetailPipelineStages
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

func renderOverview(mr *models.MergeRequest, stats *models.DiscussionStats, diffStats *models.DiffStats, approvals *models.ApprovalStatus) string {
	var sb strings.Builder
	sb.Grow(256)

	fmt.Fprintf(&sb, "!%d %s\n\n", mr.IID, mr.Title)
	fmt.Fprintf(&sb, "Author:   @%s\n", mr.Author.Username)
	if reviewers := reviewersText(mr.Reviewers); reviewers != "" {
		fmt.Fprintf(&sb, "Reviewers: %s\n", reviewers)
	}
	fmt.Fprintf(&sb, "Created:  %s\n", mr.CreatedAt.Format(detailDateFormat))
	fmt.Fprintf(&sb, "Status:   %s %s\n", mrStateLetter(mr.State), mr.State)
	fmt.Fprintf(&sb, "Branches: %s%s%s\n", mr.SourceBranch, detailBranchesSep, mr.TargetBranch)
	fmt.Fprintf(&sb, "Conflicts: %s\n", conflictText(mr.HasConflicts))
	fmt.Fprintf(&sb, "Changes:  %s\n", diffStatsText(diffStats))
	fmt.Fprintf(&sb, "Comments: %s\n", commentsText(mr.UserNotesCount, stats))
	fmt.Fprintf(&sb, "Approvals: %s\n", approvalsText(approvals))

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

// diffStatsText renders the "Changes:" overview line. Returns a dim
// "loading…" hint while the fetch is in flight (stats == nil) and the
// coloured `+N -M` pair once the diff has been counted.
func diffStatsText(stats *models.DiffStats) string {
	if stats == nil {
		return ansiDim + "loading…" + ansiReset
	}

	return fmt.Sprintf("%s+%d%s %s-%d%s",
		ansiGreen, stats.Added, ansiReset,
		ansiRed, stats.Removed, ansiReset,
	)
}

// reviewersText joins reviewer usernames into a comma-separated list.
// Returns "" when the MR has no reviewers — caller skips the line.
func reviewersText(reviewers []models.User) string {
	if len(reviewers) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(reviewers) * 16)
	for i, u := range reviewers {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteByte('@')
		sb.WriteString(u.Username)
	}

	return sb.String()
}

// approvalsText renders the "Approvals:" overview line. The `required == 0`
// dim branch avoids misleading `0/0 ✗` on projects with no approval rules.
func approvalsText(a *models.ApprovalStatus) string {
	if a == nil {
		return ansiDim + "loading…" + ansiReset
	}
	if a.ApprovalsRequired == 0 {
		return ansiDim + "no approvals required" + ansiReset
	}

	received := a.ApprovalsRequired - a.ApprovalsLeft
	color, icon := ansiRed, iconBad
	if a.Approved {
		color, icon = ansiGreen, iconOK
	}

	return fmt.Sprintf("%s%s %d/%d approvals received%s",
		color, icon, received, a.ApprovalsRequired, ansiReset,
	)
}
