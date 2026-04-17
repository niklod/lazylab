package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
)

type ProjectSuite struct {
	suite.Suite
}

func (s *ProjectSuite) TestProject_JSONRoundTrip() {
	p := models.Project{
		ID:                1,
		Name:              "lazylab",
		PathWithNamespace: "niklod/lazylab",
		DefaultBranch:     "master",
		WebURL:            "https://gitlab.example/niklod/lazylab",
		LastActivityAt:    time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC),
		Archived:          false,
	}

	data, err := json.Marshal(p)
	s.Require().NoError(err)

	var decoded models.Project
	s.Require().NoError(json.Unmarshal(data, &decoded))
	s.Require().True(p.LastActivityAt.Equal(decoded.LastActivityAt))
	decoded.LastActivityAt = p.LastActivityAt
	s.Require().Equal(p, decoded)
}

func (s *ProjectSuite) TestProject_Unmarshal_FromFixture() {
	payload := `{
        "id": 100,
        "name": "demo",
        "path_with_namespace": "group/demo",
        "default_branch": "main",
        "web_url": "https://example/group/demo",
        "last_activity_at": "2026-01-02T03:04:05Z",
        "archived": true
    }`

	var p models.Project
	s.Require().NoError(json.Unmarshal([]byte(payload), &p))

	s.Require().Equal(100, p.ID)
	s.Require().Equal("group/demo", p.PathWithNamespace)
	s.Require().Equal("main", p.DefaultBranch)
	s.Require().True(p.Archived)
	s.Require().Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), p.LastActivityAt)
}

func TestProjectSuite(t *testing.T) {
	suite.Run(t, new(ProjectSuite))
}
