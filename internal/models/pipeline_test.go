package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
)

type PipelineSuite struct {
	suite.Suite
}

func (s *PipelineSuite) TestPipeline_JSONRoundTrip() {
	p := models.Pipeline{
		ID:        5,
		Status:    models.PipelineStatusSuccess,
		Ref:       "main",
		SHA:       "abc123",
		WebURL:    "https://example/pipelines/5",
		CreatedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 1, 1, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(p)
	s.Require().NoError(err)

	var decoded models.Pipeline
	s.Require().NoError(json.Unmarshal(data, &decoded))
	s.Require().True(p.CreatedAt.Equal(decoded.CreatedAt))
	s.Require().True(p.UpdatedAt.Equal(decoded.UpdatedAt))
	decoded.CreatedAt, decoded.UpdatedAt = p.CreatedAt, p.UpdatedAt
	s.Require().Equal(p, decoded)
}

func (s *PipelineSuite) TestPipelineJob_Unmarshal_WithDuration() {
	payload := `{
        "id": 10,
        "name": "test",
        "stage": "test",
        "status": "running",
        "web_url": "https://example/jobs/10",
        "duration": 42.5,
        "allow_failure": true
    }`

	var j models.PipelineJob
	s.Require().NoError(json.Unmarshal([]byte(payload), &j))

	s.Require().Equal(10, j.ID)
	s.Require().Equal(models.PipelineStatusRunning, j.Status)
	s.Require().NotNil(j.Duration)
	s.Require().InDelta(42.5, *j.Duration, 0.0001)
	s.Require().True(j.AllowFailure)
}

func (s *PipelineSuite) TestPipelineJob_Unmarshal_NilDuration() {
	payload := `{"id": 11, "name": "lint", "stage": "static", "status": "pending", "web_url": "https://example/jobs/11"}`

	var j models.PipelineJob
	s.Require().NoError(json.Unmarshal([]byte(payload), &j))

	s.Require().Nil(j.Duration)
	s.Require().False(j.AllowFailure)
}

func (s *PipelineSuite) TestPipelineDetail_RoundTrip() {
	pd := models.PipelineDetail{
		Pipeline: models.Pipeline{ID: 1, Status: models.PipelineStatusFailed, Ref: "branch", SHA: "deadbeef"},
		Jobs: []models.PipelineJob{
			{ID: 2, Name: "a", Stage: "build", Status: models.PipelineStatusFailed},
		},
	}

	data, err := json.Marshal(pd)
	s.Require().NoError(err)

	var decoded models.PipelineDetail
	s.Require().NoError(json.Unmarshal(data, &decoded))
	s.Require().Equal(pd, decoded)
}

func TestPipelineSuite(t *testing.T) {
	suite.Run(t, new(PipelineSuite))
}
