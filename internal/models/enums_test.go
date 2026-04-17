package models_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
)

type EnumsSuite struct {
	suite.Suite
}

func (s *EnumsSuite) TestMRState_IsValid() {
	tests := []struct {
		name  string
		state models.MRState
		want  bool
	}{
		{name: "opened", state: models.MRStateOpened, want: true},
		{name: "closed", state: models.MRStateClosed, want: true},
		{name: "merged", state: models.MRStateMerged, want: true},
		{name: "empty", state: "", want: false},
		{name: "garbage", state: "nope", want: false},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Require().Equal(tt.want, tt.state.IsValid())
		})
	}
}

func (s *EnumsSuite) TestMRState_Predicates() {
	tests := []struct {
		state                models.MRState
		open, closed, merged bool
	}{
		{state: models.MRStateOpened, open: true},
		{state: models.MRStateClosed, closed: true},
		{state: models.MRStateMerged, merged: true},
	}
	for _, tt := range tests {
		s.Run(string(tt.state), func() {
			s.Require().Equal(tt.open, tt.state.IsOpen())
			s.Require().Equal(tt.closed, tt.state.IsClosed())
			s.Require().Equal(tt.merged, tt.state.IsMerged())
		})
	}
}

func (s *EnumsSuite) TestMRStateFilter_IsValid() {
	tests := []struct {
		filter models.MRStateFilter
		want   bool
	}{
		{filter: models.MRStateFilterAll, want: true},
		{filter: models.MRStateFilterOpened, want: true},
		{filter: models.MRStateFilterClosed, want: true},
		{filter: models.MRStateFilterMerged, want: true},
		{filter: "nope", want: false},
	}
	for _, tt := range tests {
		s.Run(string(tt.filter), func() {
			s.Require().Equal(tt.want, tt.filter.IsValid())
		})
	}
}

func (s *EnumsSuite) TestMROwnerFilter_IsValid() {
	tests := []struct {
		filter models.MROwnerFilter
		want   bool
	}{
		{filter: models.MROwnerFilterAll, want: true},
		{filter: models.MROwnerFilterMine, want: true},
		{filter: models.MROwnerFilterReviewer, want: true},
		{filter: "nope", want: false},
	}
	for _, tt := range tests {
		s.Run(string(tt.filter), func() {
			s.Require().Equal(tt.want, tt.filter.IsValid())
		})
	}
}

func (s *EnumsSuite) TestPipelineStatus_IsValid() {
	valid := []models.PipelineStatus{
		models.PipelineStatusCreated,
		models.PipelineStatusWaitingForResource,
		models.PipelineStatusPreparing,
		models.PipelineStatusPending,
		models.PipelineStatusRunning,
		models.PipelineStatusSuccess,
		models.PipelineStatusFailed,
		models.PipelineStatusCanceled,
		models.PipelineStatusSkipped,
		models.PipelineStatusManual,
		models.PipelineStatusScheduled,
	}
	for _, st := range valid {
		s.Run(string(st), func() {
			s.Require().True(st.IsValid())
		})
	}
	s.Require().False(models.PipelineStatus("bogus").IsValid())
}

func (s *EnumsSuite) TestPipelineStatus_IsTerminal() {
	tests := []struct {
		status models.PipelineStatus
		want   bool
	}{
		{status: models.PipelineStatusSuccess, want: true},
		{status: models.PipelineStatusFailed, want: true},
		{status: models.PipelineStatusCanceled, want: true},
		{status: models.PipelineStatusSkipped, want: true},
		{status: models.PipelineStatusRunning, want: false},
		{status: models.PipelineStatusPending, want: false},
		{status: models.PipelineStatusCreated, want: false},
		{status: models.PipelineStatusManual, want: false},
		{status: models.PipelineStatusScheduled, want: false},
	}
	for _, tt := range tests {
		s.Run(string(tt.status), func() {
			s.Require().Equal(tt.want, tt.status.IsTerminal())
		})
	}
}

func TestEnumsSuite(t *testing.T) {
	suite.Run(t, new(EnumsSuite))
}
