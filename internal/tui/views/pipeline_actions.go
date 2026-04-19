package views

import (
	"context"
	"fmt"

	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/pkg/browser"
)

// currentJob returns the job under the stages cursor, or the job currently
// shown in the log pane when the log is open. Nil when neither pane has a
// selection — every action handler no-ops on nil.
func (d *DetailView) currentJob() *models.PipelineJob {
	d.mu.Lock()
	logOpen := d.logOpen
	logJob := d.logJob
	d.mu.Unlock()

	if logOpen && logJob != nil {
		return logJob
	}
	if d.pipelineStages == nil {
		return nil
	}

	return d.pipelineStages.SelectedJob()
}

// RetryCurrentJob retries the job under the cursor (or the job shown in the
// log pane). No-op when the job's status is not retryable, avoiding a 409
// round-trip. On success the pipeline cache is invalidated so the next fetch
// surfaces the re-queued state; the data-tick goroutine picks it up on its
// next fire.
func (d *DetailView) RetryCurrentJob(ctx context.Context, project *models.Project) error {
	if project == nil || d.app == nil || d.app.GitLab == nil {
		return nil
	}
	job := d.currentJob()
	if job == nil || !job.Status.IsRetryable() {
		return nil
	}

	d.setTransientStatus(fmt.Sprintf("retrying %s…", job.Name))
	if _, err := d.app.GitLab.RetryJob(ctx, project.ID, job.ID); err != nil {
		d.setTransientStatus("retry failed: " + err.Error())

		return fmt.Errorf("detail: retry job: %w", err)
	}
	d.app.GitLab.InvalidateMR(project.ID, d.mrIID())
	d.setTransientStatus(fmt.Sprintf("retried %s", job.Name))

	return nil
}

// OpenCurrentJobInBrowser opens the job's GitLab web URL in the user's
// default browser. No-op when the job or URL is missing.
func (d *DetailView) OpenCurrentJobInBrowser() error {
	job := d.currentJob()
	if job == nil || job.WebURL == "" {
		return nil
	}
	if err := browser.Open(job.WebURL); err != nil {
		d.setTransientStatus("open failed: " + err.Error())

		return fmt.Errorf("detail: open job in browser: %w", err)
	}

	return nil
}

// CopyLogBody writes the currently-displayed job trace to the OS clipboard,
// stripped of SGR escapes. Surfaces success/failure via the transient status
// line; the status auto-clears on the next pipeline-pane render.
func (d *DetailView) CopyLogBody() error {
	if d.jobLog == nil {
		return nil
	}
	body := d.jobLog.CopyBody()
	if body == "" {
		return nil
	}
	d.mu.Lock()
	clip := d.clip
	d.mu.Unlock()

	if clip == nil {
		return nil
	}
	if err := clip.WriteAll(body); err != nil {
		d.setTransientStatus("copy failed: " + err.Error())

		return fmt.Errorf("detail: copy log body: %w", err)
	}
	d.setTransientStatus(fmt.Sprintf("copied %d chars", len(body)))

	return nil
}

// setTransientStatus writes a short ephemeral message into the stages pane's
// status slot. The next pipeline render clears it implicitly via SetDetail.
// Kept cheap — no timers, no goroutines.
func (d *DetailView) setTransientStatus(msg string) {
	if d.pipelineStages == nil {
		return
	}
	d.pipelineStages.SetTransientStatus(msg)
}

func (d *DetailView) mrIID() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.mr == nil {
		return 0
	}

	return d.mr.IID
}
