package views

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
	"github.com/niklod/lazylab/internal/tui/theme"
)

const (
	detailStatusEmpty = "Select a merge request."
	detailBranchesSep = "  \u2192  "

	iconOK  = "\u2713" // ✓
	iconBad = "\u2717" // ✗

	// detailKeyWidth is the padding applied to the fixed left column in the
	// Overview table. Design (design/project/wireframes/overview.js) uses a
	// 12-char key column so the eye can scan values top to bottom.
	detailKeyWidth = 12
	detailDescRule = "─────────────────────────────────────────────────────────────────────────"

	detailDiffLoading      = "Loading diff…"
	detailDiffEmpty        = "No files changed in this merge request."
	detailConversationStub = "Conversation tab — not yet implemented."
)

var (
	detailNoConflicts  = theme.Wrap(theme.FgOK, "none")
	detailHasConflicts = theme.Wrap(theme.FgErr, "has conflicts")
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
	pipelineLoaded  bool
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
	statsSeq, approvalsSeq, _, projectID, iid := d.commitMR(project, mr)
	if projectID == 0 || d.app == nil || d.app.GitLab == nil || d.g == nil {
		return
	}
	go d.fetchStats(context.Background(), statsSeq, projectID, iid)
	go d.fetchApprovals(context.Background(), approvalsSeq, projectID, iid)
	// Always prefetch the diff — Overview shows a +N -M line derived from
	// the same payload the Diff tab consumes, so the fetch doubles as the
	// stats source. cache.Do dedupes when the user actually opens Diff.
	d.fetchDiffAsync(context.Background(), project, mr)
	// Overview also surfaces the pipeline status, so prefetch here and let
	// cache.Do dedupe when the user later opens the Pipeline tab.
	d.fetchPipelineAsync(context.Background(), project, mr)
}

// SetMRSync applies an MR and fetches stats inline. Test-only entry point
// that mirrors MRsView.SetProjectSync so tests running without MainLoop
// observe both fields deterministically.
func (d *DetailView) SetMRSync(ctx context.Context, project *models.Project, mr *models.MergeRequest) error {
	statsSeq, approvalsSeq, pipelineSeq, projectID, iid := d.commitMR(project, mr)
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

	// commitMR set pipelineLoaded=false on the assumption a fetch would follow.
	// SetMRSync must honour that contract so the Overview Pipeline row
	// resolves off "loading…" in tests that observe the sync path.
	pipeline, err := d.app.GitLab.GetMRPipelineDetail(ctx, projectID, iid)
	d.applyPipeline(pipelineSeq, pipeline, err)
	if err != nil {
		return fmt.Errorf("detail: fetch mr pipeline: %w", err)
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

func (d *DetailView) commitMR(project *models.Project, mr *models.MergeRequest) (statsSeq, approvalsSeq, pipelineSeq uint64, projectID, iid int) {
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
	// Overview distinguishes "loading…" (fetch in flight) from "no pipeline"
	// (fetch completed, MR has none). When no fetch can fire — no project, no
	// GitLab client — flip loaded=true so the row shows a final value instead
	// of an indefinite spinner.
	willFetchPipeline := project != nil && project.ID != 0 && d.app != nil && d.app.GitLab != nil
	d.pipelineLoaded = !willFetchPipeline
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
		return d.statsSeq, d.approvalsSeq, d.pipelineSeq, 0, 0
	}

	return d.statsSeq, d.approvalsSeq, d.pipelineSeq, pid, mr.IID
}

// renderOverviewLocked rebuilds the cached Overview body from the current
// state fields. Caller must hold d.mu. Returns "" when no MR is selected —
// the Render path surfaces detailStatusEmpty in that case.
func (d *DetailView) renderOverviewLocked() string {
	if d.mr == nil {
		return ""
	}

	return renderOverview(overviewState{
		mr:             d.mr,
		stats:          d.stats,
		diffStats:      d.diffStats,
		approvals:      d.approvals,
		pipelineLoaded: d.pipelineLoaded,
		pipelineDetail: d.pipelineDetail,
		pipelineErr:    d.pipelineErr,
		now:            time.Now(),
	})
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
		d.pipelineLoaded = true
		d.cached = d.renderOverviewLocked()
		if d.pipelineStages != nil {
			d.pipelineStages.ShowError(err.Error())
		}

		return
	}
	d.pipelineDetail = detail
	d.pipelineLoaded = true
	d.cached = d.renderOverviewLocked()
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

// renderTabBar paints the Overview|Diff|Conversation|Pipeline bar at the
// top of the detail pane. Design: dim brackets and separators, active tab
// in accent bold. Source: design/project/wireframes/overview.js.
func renderTabBar(active DetailTab) string {
	var sb strings.Builder
	sb.Grow(96)
	sb.WriteString(theme.Dim)
	sb.WriteString("[ ")
	sb.WriteString(theme.Reset)
	for i, label := range detailTabLabels {
		if i > 0 {
			sb.WriteString(theme.Dim)
			sb.WriteString(" | ")
			sb.WriteString(theme.Reset)
		}
		if DetailTab(i) == active {
			sb.WriteString(theme.FgAccent)
			sb.WriteString(theme.Bold)
			sb.WriteString(label)
			sb.WriteString(theme.Reset)
		} else {
			sb.WriteString(label)
		}
	}
	sb.WriteString(theme.Dim)
	sb.WriteString(" ]")
	sb.WriteString(theme.Reset)

	return sb.String()
}

// overviewState bundles the inputs renderOverview needs. Exposed as a
// struct (vs. a long positional list) so tests construct deterministic
// snapshots via named fields.
type overviewState struct {
	mr             *models.MergeRequest
	stats          *models.DiscussionStats
	diffStats      *models.DiffStats
	approvals      *models.ApprovalStatus
	pipelineLoaded bool
	pipelineDetail *models.PipelineDetail
	pipelineErr    error
	now            time.Time
}

// renderOverview produces the body of the Overview tab. Layout mirrors
// design/project/wireframes/overview.js: bold title, dim subtitle, a fixed
// 12-char key column for labeled rows, a dashed rule, then the description
// block when one is set.
func renderOverview(st overviewState) string {
	if st.mr == nil {
		return ""
	}
	mr := st.mr

	var sb strings.Builder
	sb.Grow(512)

	fmt.Fprintf(&sb, " %s%s%s\n", theme.Bold, mr.Title, theme.Reset)
	sb.WriteString(" ")
	sb.WriteString(theme.Dim)
	sb.WriteString(overviewSubtitle(mr, st.now))
	sb.WriteString(theme.Reset)
	sb.WriteString("\n\n")

	writeRow(&sb, "Author", theme.Wrap(theme.FgAccent, "@"+mr.Author.Username))
	if reviewers := reviewersLine(mr.Reviewers); reviewers != "" {
		writeRow(&sb, "Reviewers", reviewers)
	}
	writeRow(&sb, "Branch", branchLine(mr.SourceBranch, mr.TargetBranch))
	writeRow(&sb, "State", stateLine(mr.State))
	writeRow(&sb, "Conflicts", conflictText(mr.HasConflicts))
	writeRow(&sb, "Changes", diffStatsText(st.diffStats))
	writeRow(&sb, "Approvals", approvalsText(st.approvals))
	writeRow(&sb, "Pipeline", pipelineSummary(st.pipelineLoaded, st.pipelineDetail, st.pipelineErr))
	writeRow(&sb, "Comments", commentsText(mr.UserNotesCount, st.stats))
	writeRow(&sb, "Updated", updatedLine(mr.UpdatedAt, st.now))

	desc := strings.TrimSpace(mr.Description)
	if desc != "" {
		sb.WriteString("\n ")
		sb.WriteString(theme.Dim)
		sb.WriteString(detailDescRule)
		sb.WriteString(theme.Reset)
		sb.WriteString("\n\n ")
		sb.WriteString(theme.Bold)
		sb.WriteString("Description")
		sb.WriteString(theme.Reset)
		sb.WriteString("\n\n")
		writeDescription(&sb, desc)
	}

	return sb.String()
}

// writeRow appends one labeled row (" key<pad> value\n") with the fixed
// key column width the design spec calls for. Hand-rolled padding beats
// fmt.Fprintf — the latter allocates for the variadic args on every call,
// and writeRow is invoked nine times per renderOverview.
func writeRow(sb *strings.Builder, key, value string) {
	sb.WriteByte(' ')
	sb.WriteString(key)
	for i := len(key); i < detailKeyWidth; i++ {
		sb.WriteByte(' ')
	}
	sb.WriteByte(' ')
	sb.WriteString(value)
	sb.WriteByte('\n')
}

// writeDescription indents each line of the description body by 3 spaces so
// it visually sits under the "Description" header. Trailing whitespace on
// each line is trimmed to keep copy-paste clean.
func writeDescription(sb *strings.Builder, desc string) {
	for _, line := range strings.Split(desc, "\n") {
		line = strings.TrimRight(line, " \t\r")
		sb.WriteString("   ")
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
}

func overviewSubtitle(mr *models.MergeRequest, now time.Time) string {
	verb := "opened"
	switch {
	case mr.State.IsMerged():
		verb = "merged"
	case mr.State.IsClosed():
		verb = "closed"
	}
	reference := mr.CreatedAt
	if mr.State.IsMerged() && mr.MergedAt != nil {
		reference = *mr.MergedAt
	}
	rel := theme.Relative(reference, now)

	var sb strings.Builder
	sb.Grow(64)
	fmt.Fprintf(&sb, "!%d", mr.IID)
	if mr.ProjectPath != "" {
		sb.WriteString(" · ")
		sb.WriteString(mr.ProjectPath)
	}
	if rel != "" {
		sb.WriteString(" · ")
		sb.WriteString(verb)
		sb.WriteByte(' ')
		sb.WriteString(rel)
	}

	return sb.String()
}

// reviewersLine joins reviewer usernames into a comma-separated list, each
// handle wrapped in accent colour. Returns "" when the MR has no reviewers
// — caller skips the row.
func reviewersLine(reviewers []models.User) string {
	if len(reviewers) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(reviewers) * 20)
	for i, u := range reviewers {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(theme.FgAccent)
		sb.WriteByte('@')
		sb.WriteString(u.Username)
		sb.WriteString(theme.Reset)
	}

	return sb.String()
}

func branchLine(source, target string) string {
	return source + detailBranchesSep + target
}

func stateLine(s models.MRState) string {
	var color, word string
	switch {
	case s.IsOpen():
		color, word = theme.FgOK, "opened"
	case s.IsMerged():
		color, word = theme.FgMerged, "merged"
	case s.IsClosed():
		color, word = theme.FgErr, "closed"
	default:
		color, word = theme.FgDraft, strings.ToLower(string(s))
	}

	return theme.Dot(color) + " " + theme.Wrap(color, word)
}

func conflictText(hasConflicts bool) string {
	if hasConflicts {
		return detailHasConflicts
	}

	return detailNoConflicts
}

// diffStatsText renders the "Changes" overview line. Returns a dim
// "loading…" hint while the fetch is in flight (stats == nil) and the
// coloured `+N -M` pair once the diff has been counted.
func diffStatsText(stats *models.DiffStats) string {
	if stats == nil {
		return theme.Wrap(theme.Dim, "loading…")
	}

	return fmt.Sprintf("%s %s",
		theme.Wrap(theme.FgOK, fmt.Sprintf("+%d", stats.Added)),
		theme.Wrap(theme.FgErr, fmt.Sprintf("-%d", stats.Removed)),
	)
}

// approvalsText renders the "Approvals" overview line. The `required == 0`
// dim branch avoids misleading `0/0 ✗` on projects with no approval rules.
func approvalsText(a *models.ApprovalStatus) string {
	if a == nil {
		return theme.Wrap(theme.Dim, "loading…")
	}
	if a.ApprovalsRequired == 0 {
		return theme.Wrap(theme.Dim, "no approvals required")
	}

	received := a.ApprovalsRequired - a.ApprovalsLeft
	color, icon := theme.FgErr, iconBad
	if a.Approved {
		color, icon = theme.FgOK, iconOK
	}

	return fmt.Sprintf("%s%s %d/%d approvals received%s",
		color, icon, received, a.ApprovalsRequired, theme.Reset,
	)
}

// pipelineSummary renders the "Pipeline" overview row. Loaded=false means
// "fetch in flight" (dim loading); loaded=true + nil detail means "MR has
// no pipeline"; err != nil surfaces the error in red.
func pipelineSummary(loaded bool, pd *models.PipelineDetail, err error) string {
	if err != nil {
		return theme.Wrap(theme.FgErr, "error: "+err.Error())
	}
	if !loaded {
		return theme.Wrap(theme.Dim, "loading…")
	}
	if pd == nil {
		return theme.Wrap(theme.Dim, "no pipeline")
	}

	p := pd.Pipeline
	color := pipelineStatusColor(p.Status)
	label := pipelineStatusLabel(p.Status)
	head := theme.Dot(color) + " " + color + label + theme.Reset

	meta := fmt.Sprintf("#%d", p.ID)
	if dur := pipelineDurationText(p); dur != "" {
		meta += " · " + dur
	}

	return head + "     " + theme.Wrap(theme.Dim, meta)
}

// pipelineDurationText returns the pipeline's wall-clock runtime, computed
// from UpdatedAt-CreatedAt when the status is terminal. Non-terminal
// pipelines return "" so the row does not display a misleading duration
// during an in-flight run.
func pipelineDurationText(p models.Pipeline) string {
	if !p.Status.IsTerminal() {
		return ""
	}
	if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		return ""
	}
	d := p.UpdatedAt.Sub(p.CreatedAt)
	if d <= 0 {
		return ""
	}
	seconds := int(d / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}

	return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
}

// pipelineStatusLabel maps GitLab's wire status to the user-facing verb
// used in the Overview row. Unknown statuses fall through as the lowercase
// wire name so the row still shows something meaningful.
func pipelineStatusLabel(s models.PipelineStatus) string {
	if s == models.PipelineStatusSuccess {
		return "passed"
	}

	return strings.ToLower(string(s))
}

// pipelineStatusColor maps a PipelineStatus to its semantic palette token.
// Shared with pipeline_stages.go's per-job icon colouring.
func pipelineStatusColor(s models.PipelineStatus) string {
	switch s {
	case models.PipelineStatusSuccess:
		return theme.FgOK
	case models.PipelineStatusFailed:
		return theme.FgErr
	case models.PipelineStatusRunning,
		models.PipelineStatusPending,
		models.PipelineStatusPreparing,
		models.PipelineStatusWaitingForResource:
		return theme.FgWarn
	case models.PipelineStatusCanceled,
		models.PipelineStatusSkipped,
		models.PipelineStatusCreated:
		return theme.FgDraft
	case models.PipelineStatusManual,
		models.PipelineStatusScheduled:
		return theme.FgInfo
	default:
		return theme.FgDraft
	}
}

// commentsText formats the comment count, appending a resolved-thread
// breakdown in dim when stats carry any resolvable discussions.
func commentsText(notesCount int, stats *models.DiscussionStats) string {
	if stats == nil || stats.TotalResolvable == 0 {
		return fmt.Sprintf("%d", notesCount)
	}

	return fmt.Sprintf("%d    %s(%d/%d resolved)%s",
		notesCount, theme.Dim, stats.Resolved, stats.TotalResolvable, theme.Reset,
	)
}

func updatedLine(t time.Time, now time.Time) string {
	rel := theme.Relative(t, now)
	if rel == "" {
		return theme.Wrap(theme.Dim, "—")
	}

	return theme.Wrap(theme.Dim, rel)
}
