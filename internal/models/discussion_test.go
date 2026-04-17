package models_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
)

type DiscussionSuite struct {
	suite.Suite
}

func (s *DiscussionSuite) TestDiscussionStats_JSONRoundTrip() {
	d := models.DiscussionStats{TotalResolvable: 5, Resolved: 3}

	data, err := json.Marshal(d)
	s.Require().NoError(err)

	var decoded models.DiscussionStats
	s.Require().NoError(json.Unmarshal(data, &decoded))
	s.Require().Equal(d, decoded)
}

func TestDiscussionSuite(t *testing.T) {
	suite.Run(t, new(DiscussionSuite))
}
