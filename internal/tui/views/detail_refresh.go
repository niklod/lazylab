package views

import (
	"context"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/models"
)

// OnCacheRefresh is invoked by the TUI refresh dispatcher when the cache
// reports that a background refresh changed data for (namespace, key).
// The handler re-issues the existing async fetch path for the namespace —
// the entry is now fresh in memory so cache.Do returns immediately and the
// standard apply*Seq plumbing carries the new data into the widget.
//
// Only events whose key matches the MR currently displayed in this pane
// cause a re-fetch. This keeps an mr_pipeline event for some other MR from
// forcing a repaint of the currently-displayed one. ctx derives from the
// cache's rootCtx so in-flight fetches cancel on app quit.
func (d *DetailView) OnCacheRefresh(ctx context.Context, namespace, key string) {
	d.mu.Lock()
	project := d.project
	mr := d.mr
	logJob := d.logJob
	logOpen := d.logOpen
	d.mu.Unlock()

	if project == nil || mr == nil {
		return
	}

	if namespace == cacheNamespaceJobTrace {
		if !logOpen || logJob == nil {
			return
		}
		if key != cache.MakeKey(cacheNamespaceJobTrace, project.ID, logJob.ID) {
			return
		}
		d.refetchJobTrace(ctx, project, logJob)

		return
	}

	if key != cache.MakeKey(namespace, project.ID, mr.IID) {
		return
	}

	switch namespace {
	case cacheNamespaceMR, cacheNamespaceMRApprovals, cacheNamespaceMRDiscussions:
		d.refetchOverviewPart(ctx, namespace, project.ID, mr.IID)
	case cacheNamespaceMRChanges:
		d.refetchDiffSilent(ctx, project, mr)
	case cacheNamespaceMRPipeline:
		d.refetchPipelineSilent(ctx, project, mr)
	case cacheNamespaceMRConversation:
		d.refetchConversationSilent(ctx, project, mr)
	}
}

// refetchDiffSilent re-issues the diff fetch without flipping the widget
// into a Loading state — the existing tree + content stay rendered until
// the fresh payload lands. Transient loader errors are swallowed so a flaky
// upstream can't blank out the content the user is reading.
func (d *DetailView) refetchDiffSilent(ctx context.Context, project *models.Project, mr *models.MergeRequest) {
	if d.app == nil || d.app.GitLab == nil || d.g == nil || project == nil || mr == nil {
		return
	}
	d.mu.Lock()
	d.diffSeq++
	seq := d.diffSeq
	d.mu.Unlock()

	projectID := project.ID
	iid := mr.IID
	go func() {
		data, err := d.app.GitLab.GetMRChanges(ctx, projectID, iid)
		if err != nil {
			return
		}
		d.g.Update(func(_ *gocui.Gui) error {
			d.applyDiff(seq, data, nil)

			return nil
		})
	}()
}

// refetchPipelineSilent mirrors refetchDiffSilent for the pipeline tab.
// Keeps existing stages / logs rendered, swaps in the fresh payload, drops
// transient errors so a blip doesn't wipe the pane.
func (d *DetailView) refetchPipelineSilent(ctx context.Context, project *models.Project, mr *models.MergeRequest) {
	if d.app == nil || d.app.GitLab == nil || d.g == nil || project == nil || mr == nil {
		return
	}
	d.mu.Lock()
	d.pipelineSeq++
	seq := d.pipelineSeq
	d.mu.Unlock()

	projectID := project.ID
	iid := mr.IID
	go func() {
		detail, err := d.app.GitLab.GetMRPipelineDetail(ctx, projectID, iid)
		if err != nil {
			return
		}
		d.g.Update(func(_ *gocui.Gui) error {
			d.applyPipeline(seq, detail, nil)

			return nil
		})
	}()
}

// refetchConversationSilent mirrors refetchDiffSilent for the conversation
// tab. ApplyConversation already tolerates being handed fresh data without
// having been primed via ShowLoading.
func (d *DetailView) refetchConversationSilent(ctx context.Context, project *models.Project, mr *models.MergeRequest) {
	if d.app == nil || d.app.GitLab == nil || d.g == nil || project == nil || mr == nil {
		return
	}
	d.mu.Lock()
	d.conversationSeq++
	seq := d.conversationSeq
	d.mu.Unlock()

	projectID := project.ID
	iid := mr.IID
	go func() {
		data, err := d.app.GitLab.ListMRDiscussions(ctx, projectID, iid)
		if err != nil {
			return
		}
		d.g.Update(func(_ *gocui.Gui) error {
			d.applyConversation(seq, data, nil)

			return nil
		})
	}()
}

// refetchOverviewPart re-issues a single overview-dependent fetch (stats
// or approvals). Only the matching sub-fetch is re-issued so a refresh
// event for one namespace does not pull the other.
func (d *DetailView) refetchOverviewPart(ctx context.Context, namespace string, projectID, iid int) {
	if d.app == nil || d.app.GitLab == nil {
		return
	}
	d.mu.Lock()
	switch namespace {
	case cacheNamespaceMRDiscussions, cacheNamespaceMR:
		d.statsSeq++
		seq := d.statsSeq
		d.mu.Unlock()
		go d.fetchStats(ctx, seq, projectID, iid)
	case cacheNamespaceMRApprovals:
		d.approvalsSeq++
		seq := d.approvalsSeq
		d.mu.Unlock()
		go d.fetchApprovals(ctx, seq, projectID, iid)
	default:
		d.mu.Unlock()
	}
}

// refetchJobTrace re-issues the log fetch for the currently-open job.
// Mirrors OpenJobLog's inner goroutine but reuses the job already being
// displayed instead of the stages cursor.
func (d *DetailView) refetchJobTrace(ctx context.Context, project *models.Project, job *models.PipelineJob) {
	if d.app == nil || d.app.GitLab == nil || d.g == nil || project == nil || job == nil {
		return
	}
	d.mu.Lock()
	if !d.logOpen || d.logJob == nil || d.logJob.ID != job.ID {
		d.mu.Unlock()

		return
	}
	d.logSeq++
	seq := d.logSeq
	d.mu.Unlock()

	projectID := project.ID
	jobID := job.ID
	go func() {
		trace, err := d.app.GitLab.GetJobTrace(ctx, projectID, jobID)
		if err != nil {
			return
		}
		d.g.Update(func(_ *gocui.Gui) error {
			d.applyJobTrace(seq, job, trace, nil)

			return nil
		})
	}()
}
