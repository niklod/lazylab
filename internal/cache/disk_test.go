// White-box tests — exercise unexported disk helpers.
package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
)

type DiskSuite struct {
	suite.Suite
	fs  afero.Fs
	dir string
}

func (s *DiskSuite) SetupTest() {
	s.fs = afero.NewMemMapFs()
	s.dir = "/cache"
	s.Require().NoError(ensureCacheDir(s.fs, s.dir))
}

func (s *DiskSuite) TestSaveThenLoad_DomainModel_RoundTrips() {
	project := models.Project{ID: 42, Name: "demo", PathWithNamespace: "group/demo"}
	createdAt := time.Unix(1_715_000_000, 0)

	s.Require().NoError(saveDisk(s.fs, s.dir, "project:42", createdAt, project))

	got, gotTime, ok := loadDisk[models.Project](s.fs, s.dir, "project:42")
	s.Require().True(ok)
	s.Require().Equal(project, got)
	s.Require().Equal(createdAt.Unix(), gotTime.Unix())
}

func (s *DiskSuite) TestSaveThenLoad_Slice_RoundTrips() {
	projects := []models.Project{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}
	createdAt := time.Unix(1_700_000_000, 500_000_000)

	s.Require().NoError(saveDisk(s.fs, s.dir, "projects", createdAt, projects))

	got, gotTime, ok := loadDisk[[]models.Project](s.fs, s.dir, "projects")
	s.Require().True(ok)
	s.Require().Equal(projects, got)
	s.Require().InDelta(float64(createdAt.UnixNano())/1e9, float64(gotTime.UnixNano())/1e9, 1e-6)
}

func (s *DiskSuite) TestLoad_MissingFile_ReturnsNotOk() {
	_, _, ok := loadDisk[models.Project](s.fs, s.dir, "absent")
	s.Require().False(ok)
}

func (s *DiskSuite) TestLoad_CorruptFile_ReturnsNotOk() {
	path := diskPath(s.dir, "broken")
	s.Require().NoError(afero.WriteFile(s.fs, path, []byte("not json"), 0o600))

	_, _, ok := loadDisk[models.Project](s.fs, s.dir, "broken")

	s.Require().False(ok)
}

func (s *DiskSuite) TestLoad_WrongSchema_ReturnsNotOk() {
	path := diskPath(s.dir, "wrong-schema")
	s.Require().NoError(afero.WriteFile(s.fs, path, []byte(`{"created_at":"not-a-number","data":{}}`), 0o600))

	_, _, ok := loadDisk[models.Project](s.fs, s.dir, "wrong-schema")

	s.Require().False(ok)
}

func (s *DiskSuite) TestSanitizeKey_Cases() {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "simple alphanumeric", in: "project42", want: "project42"},
		{name: "colon", in: "a:b", want: "a_b"},
		{name: "slash", in: "a/b", want: "a_b"},
		{name: "equals", in: "a=b", want: "a_b"},
		{name: "backslash path traversal collapses", in: `..\..\evil`, want: "_.._evil"},
		{name: "forward-slash path traversal collapses", in: "../../evil", want: "_.._evil"},
		{name: "leading dots stripped", in: "...secret", want: "secret"},
		{name: "control chars collapsed", in: "a\x00b\x01c", want: "a_b_c"},
		{name: "only dots becomes underscore", in: "....", want: "_"},
		{name: "runs collapse to single underscore", in: "a:::b", want: "a_b"},
		{name: "allowed punct retained", in: "mr-list_v1.0", want: "mr-list_v1.0"},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Require().Equal(tt.want, sanitizeKey(tt.in))
		})
	}
}

func (s *DiskSuite) TestDiskPath_UsesSanitizer() {
	got := diskPath(s.dir, "mr:10/99=x")
	s.Require().Equal(filepath.Join(s.dir, "api_mr_10_99_x.json"), got)
}

func (s *DiskSuite) TestRemoveDiskFile_MissingFile_IsNoError() {
	err := removeDiskFile(s.fs, s.dir, "never-written")
	s.Require().NoError(err)
}

func (s *DiskSuite) TestRemoveDiskFile_RemovesExisting() {
	s.Require().NoError(saveDisk(s.fs, s.dir, "tmp", time.Now(), "hello"))

	s.Require().NoError(removeDiskFile(s.fs, s.dir, "tmp"))

	_, _, ok := loadDisk[string](s.fs, s.dir, "tmp")
	s.Require().False(ok)
}

func (s *DiskSuite) TestEnsureCacheDir_OnOsFs_SetsDirPerm0700() {
	tmp := s.T().TempDir()
	target := filepath.Join(tmp, "nested")

	s.Require().NoError(ensureCacheDir(afero.NewOsFs(), target))

	info, err := os.Stat(target)
	s.Require().NoError(err)
	s.Require().Equal(os.FileMode(0o700), info.Mode().Perm())
}

func (s *DiskSuite) TestSaveDisk_OnOsFs_SetsFilePerm0600() {
	osFs := afero.NewOsFs()
	tmp := s.T().TempDir()
	s.Require().NoError(ensureCacheDir(osFs, tmp))

	s.Require().NoError(saveDisk(osFs, tmp, "perm", time.Now(), "data"))

	info, err := os.Stat(diskPath(tmp, "perm"))
	s.Require().NoError(err)
	s.Require().Equal(os.FileMode(0o600), info.Mode().Perm())
}

func (s *DiskSuite) TestSaveDisk_ReadOnlyFs_ReturnsError() {
	ro := afero.NewReadOnlyFs(afero.NewMemMapFs())

	err := saveDisk(ro, s.dir, "k", time.Now(), "v")

	s.Require().Error(err)
}

func (s *DiskSuite) TestUnixFloat_RoundTrips() {
	tests := []time.Time{
		time.Unix(0, 0),
		time.Unix(1_700_000_000, 0),
		time.Unix(1_700_000_000, 500_000_000),
		time.Unix(-10, 0),
	}
	for _, t := range tests {
		got := fromUnixFloat(toUnixFloat(t))
		s.Require().InDelta(t.UnixNano(), got.UnixNano(), float64(time.Millisecond))
	}
}

func (s *DiskSuite) TestSaveDisk_MarshalFailure_ReturnsError() {
	err := saveDisk(s.fs, s.dir, "k", time.Now(), unmarshalableValue{})

	s.Require().Error(err)
}

type unmarshalableValue struct{}

func (unmarshalableValue) MarshalJSON() ([]byte, error) {
	return nil, context.Canceled
}

func TestDiskSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(DiskSuite))
}
