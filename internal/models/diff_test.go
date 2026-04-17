package models_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
)

type DiffSuite struct {
	suite.Suite
}

func (s *DiffSuite) TestMRDiffFile_JSONRoundTrip() {
	f := models.MRDiffFile{
		OldPath:     "a.go",
		NewPath:     "b.go",
		Diff:        "@@ -1 +1 @@\n-a\n+b",
		NewFile:     false,
		RenamedFile: true,
		DeletedFile: false,
	}

	data, err := json.Marshal(f)
	s.Require().NoError(err)

	var decoded models.MRDiffFile
	s.Require().NoError(json.Unmarshal(data, &decoded))
	s.Require().Equal(f, decoded)
}

func (s *DiffSuite) TestMRDiffData_EmptyFiles_RoundTrip() {
	d := models.MRDiffData{}

	data, err := json.Marshal(d)
	s.Require().NoError(err)

	var decoded models.MRDiffData
	s.Require().NoError(json.Unmarshal(data, &decoded))
	s.Require().Empty(decoded.Files)
}

func TestDiffSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(DiffSuite))
}
