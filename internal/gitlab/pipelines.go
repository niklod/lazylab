package gitlab

import (
	"context"
	"fmt"
	"io"

	gogitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/niklod/lazylab/internal/models"
)

const (
	mrPipelineCacheNamespace = "mr_pipeline"
	jobTraceCacheNamespace   = "job_trace"

	defaultPipelineJobsPerPage = 100
)

// GetMRPipelineDetail fetches the latest pipeline for an MR together with its
// jobs. Mirrors Python `get_pipeline_detail` — one cache entry per (project,
// iid) carrying pipeline + jobs so invalidation is atomic and the Pipeline
// tab avoids a second round-trip for jobs after discovering the pipeline ID.
// Returns (nil, nil) when the MR has no pipelines attached.
func (c *Client) GetMRPipelineDetail(ctx context.Context, projectID, iid int) (*models.PipelineDetail, error) {
	if projectID <= 0 || iid <= 0 {
		return nil, fmt.Errorf("gitlab: get mr pipeline detail: project id and iid required")
	}

	loader := func(ctx context.Context) (*models.PipelineDetail, error) {
		return c.fetchMRPipelineDetail(ctx, projectID, iid)
	}

	return doCached(ctx, c, mrPipelineCacheNamespace, "get mr pipeline detail", loader, projectID, iid)
}

func (c *Client) fetchMRPipelineDetail(ctx context.Context, projectID, iid int) (*models.PipelineDetail, error) {
	pipelines, _, err := c.api.MergeRequests.ListMergeRequestPipelines(projectID, int64(iid), gogitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("gitlab: list mr pipelines %d!%d: %w", projectID, iid, err)
	}
	if len(pipelines) == 0 {
		// nilnil is intentional: a missing pipeline is a normal
		// state the UI renders as "no pipeline for this MR", not
		// an error. A sentinel error would force every caller to
		// errors.Is against it before inspecting the payload.
		return nil, nil //nolint:nilnil
	}
	latest := pipelines[0]

	full, _, err := c.api.Pipelines.GetPipeline(projectID, latest.ID, gogitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("gitlab: get pipeline %d: %w", latest.ID, err)
	}

	jobs, err := c.listPipelineJobs(ctx, projectID, latest.ID)
	if err != nil {
		return nil, fmt.Errorf("gitlab: fetch mr pipeline detail: %w", err)
	}

	return &models.PipelineDetail{
		Pipeline: toDomainPipeline(full),
		Jobs:     jobs,
	}, nil
}

func (c *Client) listPipelineJobs(ctx context.Context, projectID int, pipelineID int64) ([]models.PipelineJob, error) {
	opts := &gogitlab.ListJobsOptions{
		ListOptions: gogitlab.ListOptions{
			Page:    1,
			PerPage: int64(defaultPipelineJobsPerPage),
		},
	}
	var out []models.PipelineJob
	for {
		page, resp, err := c.api.Jobs.ListPipelineJobs(projectID, pipelineID, opts, gogitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: list pipeline jobs page %d: %w", opts.Page, err)
		}
		for _, j := range page {
			if j == nil {
				continue
			}
			out = append(out, toDomainJob(j))
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// GitLab returns pipeline jobs newest-first (deploy → test → build),
	// which is backwards for a pipeline visualisation where stages should
	// read in execution order (build → test → deploy). Reverse once here
	// so every downstream consumer (stages widget, tests, cache) sees a
	// stable, intuitive ordering.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}

	return out, nil
}

// GetJobTrace fetches a CI job's log. Large — cache TTL keeps the UI snappy
// for revisits while still refreshing on the stale-while-revalidate cycle.
func (c *Client) GetJobTrace(ctx context.Context, projectID, jobID int) (string, error) {
	if projectID <= 0 || jobID <= 0 {
		return "", fmt.Errorf("gitlab: get job trace: project id and job id required")
	}

	loader := func(ctx context.Context) (string, error) {
		reader, _, err := c.api.Jobs.GetTraceFile(projectID, int64(jobID), gogitlab.WithContext(ctx))
		if err != nil {
			return "", fmt.Errorf("gitlab: get job trace %d!%d: %w", projectID, jobID, err)
		}
		if reader == nil {
			return "", nil
		}
		buf, err := io.ReadAll(reader)
		if err != nil {
			return "", fmt.Errorf("gitlab: read job trace %d!%d: %w", projectID, jobID, err)
		}

		return string(buf), nil
	}

	return doCached(ctx, c, jobTraceCacheNamespace, "get job trace", loader, projectID, jobID)
}

func toDomainPipeline(p *gogitlab.Pipeline) models.Pipeline {
	if p == nil {
		return models.Pipeline{}
	}
	m := models.Pipeline{
		ID:     int(p.ID),
		Status: models.PipelineStatus(p.Status),
		Ref:    p.Ref,
		SHA:    p.SHA,
		WebURL: p.WebURL,
	}
	if p.CreatedAt != nil {
		m.CreatedAt = *p.CreatedAt
	}
	if p.UpdatedAt != nil {
		m.UpdatedAt = *p.UpdatedAt
	}

	return m
}

func toDomainJob(j *gogitlab.Job) models.PipelineJob {
	out := models.PipelineJob{
		ID:           int(j.ID),
		Name:         j.Name,
		Stage:        j.Stage,
		Status:       models.PipelineStatus(j.Status),
		WebURL:       j.WebURL,
		AllowFailure: j.AllowFailure,
	}
	// Zero duration means "not started" — omit so the UI renders "" instead
	// of a misleading "0s". client-go uses 0 as the sentinel because the
	// struct field is a non-pointer float64.
	if j.Duration > 0 {
		d := j.Duration
		out.Duration = &d
	}

	return out
}
