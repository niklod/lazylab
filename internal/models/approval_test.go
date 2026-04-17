package models_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
)

type ApprovalSuite struct {
	suite.Suite
}

func (s *ApprovalSuite) TestApproval_JSONRoundTrip() {
	a := models.ApprovalStatus{
		Approved:          true,
		ApprovalsRequired: 2,
		ApprovalsLeft:     0,
		ApprovedBy: []models.User{
			{ID: 1, Username: "alice", Name: "Alice", WebURL: "https://example/alice"},
		},
	}

	data, err := json.Marshal(a)
	s.Require().NoError(err)

	var decoded models.ApprovalStatus
	s.Require().NoError(json.Unmarshal(data, &decoded))
	s.Require().Equal(a, decoded)
}

func (s *ApprovalSuite) TestApproval_Unmarshal_EmptyApprovedBy() {
	payload := `{"approved": false, "approvals_required": 1, "approvals_left": 1}`

	var a models.ApprovalStatus
	s.Require().NoError(json.Unmarshal([]byte(payload), &a))

	s.Require().False(a.Approved)
	s.Require().Equal(1, a.ApprovalsRequired)
	s.Require().Nil(a.ApprovedBy)
}

func TestApprovalSuite(t *testing.T) {
	suite.Run(t, new(ApprovalSuite))
}
