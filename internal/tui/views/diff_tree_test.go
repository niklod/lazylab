package views

import (
	"strings"
	"testing"

	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
)

type DiffTreeSuite struct {
	suite.Suite
	g *gocui.Gui
	v *DiffTreeView
}

func (s *DiffTreeSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	s.Require().NoError(err)
	s.g = g

	_, err = s.g.SetView(keymap.ViewDetailDiffTree, 0, 0, 60, 30, 0)
	if err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
		s.T().Fatalf("SetView: %v", err)
	}

	s.v = NewDiffTree()
}

func (s *DiffTreeSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
}

func (s *DiffTreeSuite) files() []models.MRDiffFile {
	return []models.MRDiffFile{
		{OldPath: "README.md", NewPath: "README.md"},
		{OldPath: "src/a.go", NewPath: "src/a.go"},
		{OldPath: "src/nested/b.go", NewPath: "src/nested/b.go", NewFile: true},
		{OldPath: "src/nested/c.go", NewPath: "src/nested/c.go", DeletedFile: true},
	}
}

func (s *DiffTreeSuite) TestSetFiles_GroupsByDirectory() {
	s.v.SetFiles(s.files())

	rows := s.v.rowsSnapshot()
	s.Require().NotEmpty(rows)

	dirs := 0
	leaves := 0
	for _, r := range rows {
		if r.file == nil {
			dirs++
		} else {
			leaves++
		}
	}
	s.Require().Equal(4, leaves, "one row per file")
	s.Require().Equal(2, dirs, "src/ and src/nested/ headers printed once each")
}

func (s *DiffTreeSuite) TestSetFiles_CursorStartsOnFirstLeaf() {
	s.v.SetFiles(s.files())

	cursor := s.v.Cursor()
	rows := s.v.rowsSnapshot()

	s.Require().GreaterOrEqual(cursor, 0)
	s.Require().Less(cursor, len(rows))
	s.Require().NotNil(rows[cursor].file)
}

func (s *DiffTreeSuite) TestMoveCursor_NegativeDoesNotLandOnDirectoryHeader() {
	// Regression: k on first leaf above a dir header used to commit a
	// directory row as the new cursor. See logic-correctness review.
	files := []models.MRDiffFile{
		{NewPath: "src/a.go"},
		{NewPath: "src/nested/b.go"},
	}
	s.v.SetFiles(files)

	rows := s.v.rowsSnapshot()
	firstLeaf := firstLeafRow(rows)
	for firstLeaf > 0 && rows[firstLeaf].file != nil && rows[firstLeaf-1].file != nil {
		firstLeaf--
	}

	s.v.cursor = firstLeafRow(rows)

	moved := s.v.MoveCursor(-1)

	s.Require().False(moved, "no leaf above first → cursor stays put")
	s.Require().NotNil(rows[s.v.Cursor()].file, "cursor remains on a leaf")
}

func (s *DiffTreeSuite) TestMoveCursor_SkipsDirectoryRows() {
	s.v.SetFiles(s.files())

	start := s.v.Cursor()

	moved := s.v.MoveCursor(1)
	s.Require().True(moved)
	next := s.v.Cursor()
	s.Require().Greater(next, start)

	rows := s.v.rowsSnapshot()
	s.Require().NotNil(rows[next].file, "cursor must always land on a leaf")

	for i := 0; i < 10; i++ {
		s.v.MoveCursor(1)
	}
	last := s.v.Cursor()
	s.Require().NotNil(rows[last].file)
}

func (s *DiffTreeSuite) TestMoveCursorToEnd_LandsOnLastLeaf() {
	s.v.SetFiles(s.files())

	s.v.MoveCursorToEnd()
	rows := s.v.rowsSnapshot()
	cursor := s.v.Cursor()

	s.Require().NotNil(rows[cursor].file)
	for i := cursor + 1; i < len(rows); i++ {
		s.Require().Nil(rows[i].file, "no leaf after MoveCursorToEnd")
	}
}

func (s *DiffTreeSuite) TestSelectedFile_ReturnsCurrentCursorFile() {
	s.v.SetFiles(s.files())

	got := s.v.SelectedFile()

	s.Require().NotNil(got)
	s.Require().Equal("README.md", got.NewPath, "first leaf is README at root")
}

func (s *DiffTreeSuite) TestFileStatusLabel_CoversEveryBranch() {
	tests := []struct {
		name string
		file models.MRDiffFile
		want string
	}{
		{"new", models.MRDiffFile{NewFile: true}, diffTreeStatusAdd},
		{"deleted", models.MRDiffFile{DeletedFile: true}, diffTreeStatusDel},
		{"renamed", models.MRDiffFile{RenamedFile: true}, diffTreeStatusRen},
		{"modified", models.MRDiffFile{}, diffTreeStatusMod},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got := fileStatusLabel(&tt.file)
			s.Require().Contains(got, tt.want)
		})
	}
}

func (s *DiffTreeSuite) TestSetFiles_NilReturnsLoadingState() {
	s.v.SetFiles(nil)

	rows := s.v.rowsSnapshot()
	s.Require().Empty(rows)
	s.Require().Equal(diffTreeLoadingStr, s.v.statusSnapshot())
}

func (s *DiffTreeSuite) TestSetFiles_EmptyShowsEmptyHint() {
	s.v.SetFiles([]models.MRDiffFile{})

	s.Require().Equal(diffTreeEmptyHint, s.v.statusSnapshot())
}

func (s *DiffTreeSuite) TestRender_WritesLeavesAndDirs() {
	pv, err := s.g.View(keymap.ViewDetailDiffTree)
	s.Require().NoError(err)

	s.v.SetFiles(s.files())
	s.v.Render(pv)

	buf := pv.Buffer()
	s.Require().Contains(buf, "README.md")
	s.Require().Contains(buf, "a.go")
	s.Require().Contains(buf, "nested")
	s.Require().Contains(buf, "b.go")

	s.Require().GreaterOrEqual(strings.Count(buf, "src/"), 1, "src/ dir header present")
}

// rowsSnapshot is a tiny test helper that copies the internal rows slice
// under lock for inspection. Lives in this file to keep the production
// view surface lean.
func (v *DiffTreeView) rowsSnapshot() []diffTreeRow {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]diffTreeRow, len(v.rows))
	copy(out, v.rows)

	return out
}

func (v *DiffTreeView) statusSnapshot() string {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.status
}

//nolint:paralleltest // gocui stores tcell simulation screen in a global; parallel runs race.
func TestDiffTreeSuite(t *testing.T) {
	suite.Run(t, new(DiffTreeSuite))
}
