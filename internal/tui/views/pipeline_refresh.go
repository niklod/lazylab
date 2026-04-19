package views

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/models"
)

// pipelineRefresh cadence — user preference overrides the design's 30s-for-
// terminal suggestion: we poll every 5 seconds regardless of pipeline state
// (retries, manual kicks, and mid-flight stage transitions all surface within
// one refresh window). The 1s UI tick re-renders the "updated Ns ago"
// fragment between network calls without issuing any requests.
const (
	pipelineRefreshFast = 5 * time.Second
	pipelineRefreshUI   = 1 * time.Second
)

// pipelineRefreshCtl owns the cancel handle and live state for the Pipeline
// tab's auto-refresh goroutines. Created on tab entry, torn down on leave.
//
// Two goroutines fire under one context: a data-tick that bumps the cache and
// refetches the pipeline, and a ui-tick that only requests a gocui redraw so
// the "updated Ns ago" fragment ticks forward while no network call is
// needed. Both share the same cancel so there's only one lifecycle to reason
// about.
type pipelineRefreshCtl struct {
	cancel    context.CancelFunc
	project   *models.Project
	mr        *models.MergeRequest
	interval  atomic.Int64 // time.Duration; atomic so the ticker can reset without d.mu
	dataReset chan time.Duration
}

// startPipelineRefresh spawns the refresh goroutines for (project, mr). Stops
// any previously-running controller first. Safe to call when d.g or GitLab
// client is missing — no-op in that case.
func (d *DetailView) startPipelineRefresh(project *models.Project, mr *models.MergeRequest) {
	d.mu.Lock()
	d.stopPipelineRefreshLocked()
	if project == nil || mr == nil {
		d.mu.Unlock()

		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	ctl := &pipelineRefreshCtl{
		cancel:    cancel,
		project:   project,
		mr:        mr,
		dataReset: make(chan time.Duration, 1),
	}
	ctl.interval.Store(int64(pipelineRefreshFast))
	d.pipelineRefresh.interval = pipelineRefreshFast
	d.pipelineRefreshCtl = ctl
	d.mu.Unlock()

	go d.runPipelineDataTick(ctx, ctl)
	go d.runPipelineUITick(ctx)
}

// stopPipelineRefreshLocked cancels the live refresh controller if any.
// Caller holds d.mu.
func (d *DetailView) stopPipelineRefreshLocked() {
	if d.pipelineRefreshCtl == nil {
		return
	}
	d.pipelineRefreshCtl.cancel()
	d.pipelineRefreshCtl = nil
}

// ToggleAutoRefresh flips the auto-refresh enabled state. Keeps the
// goroutines alive so re-enabling doesn't pay for a goroutine restart; the
// data-tick simply skips its fetch while disabled.
func (d *DetailView) ToggleAutoRefresh() {
	d.mu.Lock()
	d.pipelineRefresh.enabled = !d.pipelineRefresh.enabled
	d.mu.Unlock()

	if d.g != nil {
		d.g.Update(func(_ *gocui.Gui) error { return nil })
	}
}

// AutoRefreshEnabled exposes the current auto-refresh toggle state. Used by
// tests and by the `a` keybind's optional feedback.
func (d *DetailView) AutoRefreshEnabled() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.pipelineRefresh.enabled
}

// ForceRefreshPipeline invalidates the cached pipeline for the current MR
// and triggers a fresh fetch. Hooked up to the `R` keybind and to the retry
// flow after a successful job retry.
func (d *DetailView) ForceRefreshPipeline(ctx context.Context) {
	d.mu.Lock()
	project, mr := d.refreshTargetsLocked()
	d.mu.Unlock()

	if project == nil || mr == nil {
		return
	}
	d.invalidateAndRefetchPipeline(ctx, project, mr)
}

func (d *DetailView) refreshTargetsLocked() (*models.Project, *models.MergeRequest) {
	if d.pipelineRefreshCtl == nil {
		return nil, d.mr
	}

	return d.pipelineRefreshCtl.project, d.pipelineRefreshCtl.mr
}

func (d *DetailView) runPipelineDataTick(ctx context.Context, ctl *pipelineRefreshCtl) {
	timer := time.NewTimer(time.Duration(ctl.interval.Load()))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case next := <-ctl.dataReset:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			ctl.interval.Store(int64(next))
			timer.Reset(next)
		case <-timer.C:
			d.mu.Lock()
			enabled := d.pipelineRefresh.enabled
			project, mr := ctl.project, ctl.mr
			d.mu.Unlock()

			if enabled && project != nil && mr != nil {
				d.invalidateAndRefetchPipeline(ctx, project, mr)
			}
			timer.Reset(time.Duration(ctl.interval.Load()))
		}
	}
}

func (d *DetailView) runPipelineUITick(ctx context.Context) {
	ticker := time.NewTicker(pipelineRefreshUI)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if d.g == nil {
				continue
			}
			d.g.Update(func(_ *gocui.Gui) error { return nil })
		}
	}
}

// ForceRefreshPipelineSync is the test entry that invalidates + refetches
// inline (no goroutine, no gocui update). Useful for e2e suites that need a
// deterministic post-refresh state without running MainLoop.
func (d *DetailView) ForceRefreshPipelineSync(ctx context.Context) error {
	d.mu.Lock()
	project, mr := d.refreshTargetsLocked()
	seq := d.pipelineSeq
	d.mu.Unlock()

	if project == nil || mr == nil || d.app == nil || d.app.GitLab == nil {
		return nil
	}
	d.app.GitLab.InvalidateMR(project.ID, mr.IID)
	detail, err := d.app.GitLab.GetMRPipelineDetail(ctx, project.ID, mr.IID)
	d.applyPipelineRefresh(seq, detail, err)
	if err != nil {
		return fmt.Errorf("detail: refresh pipeline: %w", err)
	}

	return nil
}

// invalidateAndRefetchPipeline drops cached state for this MR and fetches
// fresh pipeline data. Does NOT flip the stages pane into the loading state —
// background refresh should be invisible to the user; the stale data stays
// on-screen until the new data arrives (stale-while-revalidate).
func (d *DetailView) invalidateAndRefetchPipeline(ctx context.Context, project *models.Project, mr *models.MergeRequest) {
	if d.app == nil || d.app.GitLab == nil || d.g == nil {
		return
	}
	d.app.GitLab.InvalidateMR(project.ID, mr.IID)

	d.mu.Lock()
	seq := d.pipelineSeq
	d.mu.Unlock()

	detail, err := d.app.GitLab.GetMRPipelineDetail(ctx, project.ID, mr.IID)
	if ctx.Err() != nil {
		return
	}
	d.g.Update(func(_ *gocui.Gui) error {
		d.applyPipelineRefresh(seq, detail, err)

		return nil
	})
}

func (d *DetailView) applyPipelineRefresh(seq uint64, detail *models.PipelineDetail, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if seq != d.pipelineSeq {
		return
	}
	if err != nil {
		// Background refresh failures don't clobber the stale view —
		// surface the message in the stages pane status (Stage 6's
		// transient status slot would be cleaner; for now, dim the
		// last-refresh indicator by leaving lastRefresh unchanged).
		return
	}
	d.pipelineDetail = detail
	d.pipelineLoaded = true
	d.pipelineErr = nil
	d.cached = d.renderOverviewLocked()
	if d.pipelineStages != nil {
		d.pipelineStages.SetDetail(detail)
	}
	d.pipelineRefresh.lastRefresh = time.Now()
}
