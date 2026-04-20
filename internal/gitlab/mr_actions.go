package gitlab

import (
	"context"
	"fmt"

	gogitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/niklod/lazylab/internal/models"
)

// AcceptOptions captures the subset of GitLab merge options the TUI exposes.
// Wireframe-parity (see ADR 020): squash + delete-source-branch only; the
// deprecated merge-when-pipeline-succeeds knob has been dropped. Future
// auto-merge wiring should set AcceptMergeRequestOptions.AutoMerge, not the
// MergeWhenPipelineSucceeds field.
type AcceptOptions struct {
	Squash                   bool
	ShouldRemoveSourceBranch bool
}

// CloseMergeRequest transitions the MR to state=closed via the Update
// endpoint (state_event=close). The mutation self-invalidates the MR cache
// so callers cannot forget — every cached namespace (list/get/approvals/
// discussions/conversation/changes/pipeline…) is dropped before return.
// Returns the refreshed domain model mapped through toDomainMergeRequestFromFull.
func (c *Client) CloseMergeRequest(ctx context.Context, projectID, iid int, projectPath string) (*models.MergeRequest, error) {
	return c.mutateMR(projectID, iid, projectPath, "close", func() (*gogitlab.MergeRequest, error) {
		mr, _, err := c.api.MergeRequests.UpdateMergeRequest(
			projectID,
			int64(iid),
			&gogitlab.UpdateMergeRequestOptions{StateEvent: gogitlab.Ptr("close")},
			gogitlab.WithContext(ctx),
		)

		return mr, err //nolint:wrapcheck // wrapped in mutateMR with op+project+iid context
	})
}

// AcceptMergeRequest merges the MR with the given options and returns the
// refreshed domain model. Self-invalidates the MR cache on success (see
// CloseMergeRequest for rationale).
//
// GitLab returns 405 when the MR is already merged/closed or cannot be merged
// (conflicts, failed pipeline). The error is wrapped so the TUI can surface it
// to the user without special-casing the HTTP code.
func (c *Client) AcceptMergeRequest(ctx context.Context, projectID, iid int, projectPath string, opts AcceptOptions) (*models.MergeRequest, error) {
	return c.mutateMR(projectID, iid, projectPath, "accept", func() (*gogitlab.MergeRequest, error) {
		mr, _, err := c.api.MergeRequests.AcceptMergeRequest(
			projectID,
			int64(iid),
			&gogitlab.AcceptMergeRequestOptions{
				Squash:                   gogitlab.Ptr(opts.Squash),
				ShouldRemoveSourceBranch: gogitlab.Ptr(opts.ShouldRemoveSourceBranch),
			},
			gogitlab.WithContext(ctx),
		)

		return mr, err //nolint:wrapcheck // wrapped in mutateMR with op+project+iid context
	})
}

// mutateMR centralises validate → call → wrap → nil-guard → invalidate → map
// so both public mutations share a single invariant chain. The op string
// ("close"/"accept") is spliced into the error prefix and the initial
// validation message so call sites stay diagnosable.
func (c *Client) mutateMR(
	projectID, iid int,
	projectPath, op string,
	call func() (*gogitlab.MergeRequest, error),
) (*models.MergeRequest, error) {
	if projectID <= 0 || iid <= 0 {
		return nil, fmt.Errorf("gitlab: %s merge request: project id and iid required", op)
	}
	updated, err := call()
	if err != nil {
		return nil, fmt.Errorf("gitlab: %s merge request %d!%d: %w", op, projectID, iid, err)
	}
	if updated == nil {
		return nil, fmt.Errorf("gitlab: %s merge request %d!%d: empty response", op, projectID, iid)
	}

	c.InvalidateMR(projectID, iid)

	return toDomainMergeRequestFromFull(updated, projectPath), nil
}
