package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
)

type MergeRequestSuite struct {
	suite.Suite
}

func (s *MergeRequestSuite) TestMergeRequest_JSONRoundTrip() {
	merged := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	mr := models.MergeRequest{
		ID:             100,
		IID:            7,
		Title:          "feat: cache",
		Description:    "body",
		State:          models.MRStateMerged,
		Author:         models.User{ID: 1, Username: "alice", Name: "Alice", WebURL: "https://example/alice"},
		SourceBranch:   "feat/cache",
		TargetBranch:   "main",
		WebURL:         "https://example/mrs/7",
		CreatedAt:      time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		MergedAt:       &merged,
		HasConflicts:   false,
		MergeStatus:    "can_be_merged",
		UserNotesCount: 4,
		ProjectPath:    "group/demo",
	}

	data, err := json.Marshal(mr)
	s.Require().NoError(err)

	var decoded models.MergeRequest
	s.Require().NoError(json.Unmarshal(data, &decoded))
	s.Require().True(mr.CreatedAt.Equal(decoded.CreatedAt))
	s.Require().True(mr.UpdatedAt.Equal(decoded.UpdatedAt))
	s.Require().NotNil(decoded.MergedAt)
	s.Require().True(mr.MergedAt.Equal(*decoded.MergedAt))
	decoded.CreatedAt, decoded.UpdatedAt = mr.CreatedAt, mr.UpdatedAt
	decoded.MergedAt = mr.MergedAt
	s.Require().Equal(mr, decoded)
}

func (s *MergeRequestSuite) TestMergeRequest_Unmarshal_NilMergedAt() {
	payload := `{
        "id": 1, "iid": 1, "title": "open", "state": "opened",
        "author": {"id": 1, "username": "a", "name": "A", "web_url": "x"},
        "source_branch": "f", "target_branch": "main", "web_url": "x",
        "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-02T00:00:00Z"
    }`

	var mr models.MergeRequest
	s.Require().NoError(json.Unmarshal([]byte(payload), &mr))

	s.Require().Nil(mr.MergedAt)
	s.Require().Equal(models.MRStateOpened, mr.State)
	s.Require().True(mr.IsOpen())
	s.Require().False(mr.IsMerged())
	s.Require().False(mr.IsClosed())
}

func (s *MergeRequestSuite) TestMergeRequest_Predicates_DelegateToState() {
	tests := []struct {
		state  models.MRState
		open   bool
		closed bool
		merged bool
	}{
		{state: models.MRStateOpened, open: true},
		{state: models.MRStateClosed, closed: true},
		{state: models.MRStateMerged, merged: true},
	}
	for _, tt := range tests {
		s.Run(string(tt.state), func() {
			mr := models.MergeRequest{State: tt.state}
			s.Require().Equal(tt.open, mr.IsOpen())
			s.Require().Equal(tt.closed, mr.IsClosed())
			s.Require().Equal(tt.merged, mr.IsMerged())
		})
	}
}

func TestMergeRequestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(MergeRequestSuite))
}
