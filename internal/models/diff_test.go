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

func (s *DiffSuite) TestStats_CountsAddedAndRemoved() {
	d := &models.MRDiffData{
		Files: []models.MRDiffFile{
			{Diff: "--- a/foo\n+++ b/foo\n@@ -1,2 +1,3 @@\n context\n-old\n+new1\n+new2\n+new3\n"},
			{Diff: "--- a/bar\n+++ b/bar\n@@ -1 +0,0 @@\n-removed\n-again\n"},
		},
	}

	got := d.Stats()

	s.Require().Equal(3, got.Added, "three +lines excluding the +++ header")
	s.Require().Equal(3, got.Removed, "three -lines excluding the --- header")
}

func (s *DiffSuite) TestStats_NilReceiverReturnsZero() {
	var d *models.MRDiffData

	got := d.Stats()

	s.Require().Equal(models.DiffStats{}, got)
}

func (s *DiffSuite) TestStats_EmptyDiffReturnsZero() {
	d := &models.MRDiffData{Files: []models.MRDiffFile{{Diff: ""}}}

	got := d.Stats()

	s.Require().Equal(models.DiffStats{}, got)
}

func (s *DiffSuite) TestStats_IgnoresHunkAndContextLines() {
	d := &models.MRDiffData{
		Files: []models.MRDiffFile{
			{Diff: "@@ -1 +1 @@\n context\n+added\n-removed\n"},
		},
	}

	got := d.Stats()

	s.Require().Equal(models.DiffStats{Added: 1, Removed: 1}, got)
}

func TestDiffSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(DiffSuite))
}
