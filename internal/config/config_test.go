package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"

	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/models"
)

type ConfigSuite struct {
	suite.Suite
	fs   afero.Fs
	path string
}

func (s *ConfigSuite) SetupTest() {
	s.fs = afero.NewMemMapFs()
	s.path = filepath.Join("/home/user/.config/lazylab", "config.yaml")
	s.T().Setenv(config.EnvGitLabToken, "")
	s.T().Setenv(config.EnvConfigPath, "")
}

func (s *ConfigSuite) TestLoad_MissingFile_WritesDefaults() {
	cfg, err := config.Load(s.fs, s.path)

	s.Require().NoError(err)
	s.Require().Equal("https://gitlab.com", cfg.GitLab.URL)
	s.Require().Equal(models.MRStateFilterOpened, cfg.MergeRequests.StateFilter)
	s.Require().Equal(models.MROwnerFilterAll, cfg.MergeRequests.OwnerFilter)
	s.Require().Equal(600, cfg.Cache.TTL)
	s.Require().Equal(5_000_000, cfg.Core.LogfileMaxBytes)

	info, err := s.fs.Stat(s.path)
	s.Require().NoError(err)
	s.Require().Equal(os.FileMode(0o600), info.Mode().Perm())

	raw, err := afero.ReadFile(s.fs, s.path)
	s.Require().NoError(err)
	s.Require().Contains(string(raw), "url: https://gitlab.com")
}

func (s *ConfigSuite) TestLoad_PartialFile_MergesDefaults() {
	partial := []byte("gitlab:\n  url: https://gitlab.example.com/\n")
	s.Require().NoError(afero.WriteFile(s.fs, s.path, partial, 0o600))

	cfg, err := config.Load(s.fs, s.path)

	s.Require().NoError(err)
	s.Require().Equal("https://gitlab.example.com", cfg.GitLab.URL)
	s.Require().Equal("last_activity", cfg.Repositories.SortBy)
	s.Require().Equal(models.MRStateFilterOpened, cfg.MergeRequests.StateFilter)
	s.Require().Equal(600, cfg.Cache.TTL)
}

func (s *ConfigSuite) TestLoad_FullFile_RoundTrips() {
	full := &config.Config{
		GitLab: config.GitLabConfig{URL: "https://g.example.com", Token: "file-token"},
		Repositories: config.RepositoriesConfig{
			Favorites: []string{"group/a", "group/b"},
			SortBy:    "name",
		},
		MergeRequests: config.MergeRequestsConfig{
			StateFilter: models.MRStateFilterAll,
			OwnerFilter: models.MROwnerFilterMine,
		},
		Cache: config.CacheConfig{Directory: "/tmp/cache", TTL: 120},
		Core:  config.CoreConfig{Logfile: "/tmp/log", LogfileMaxBytes: 42, LogfileCount: 1},
	}
	data, err := yaml.Marshal(full)
	s.Require().NoError(err)
	s.Require().NoError(afero.WriteFile(s.fs, s.path, data, 0o600))

	cfg, err := config.Load(s.fs, s.path)

	s.Require().NoError(err)
	s.Require().Equal(full, cfg)
}

func (s *ConfigSuite) TestLoad_EnvTokenOverridesFile() {
	body := []byte("gitlab:\n  url: https://example.com\n  token: file-token\n")
	s.Require().NoError(afero.WriteFile(s.fs, s.path, body, 0o600))
	s.T().Setenv(config.EnvGitLabToken, "env-token")

	cfg, err := config.Load(s.fs, s.path)

	s.Require().NoError(err)
	s.Require().Equal("env-token", cfg.GitLab.Token)
}

func (s *ConfigSuite) TestLoad_EnvTokenOverridesDefaultsPath() {
	s.T().Setenv(config.EnvGitLabToken, "env-token")

	cfg, err := config.Load(s.fs, s.path)

	s.Require().NoError(err)
	s.Require().Equal("env-token", cfg.GitLab.Token)
}

func (s *ConfigSuite) TestLoad_ReadError_ReturnsWrappedError() {
	osFs := afero.NewOsFs()
	dir := s.T().TempDir()
	path := filepath.Join(dir, "config.yaml")
	s.Require().NoError(osFs.MkdirAll(path, 0o700))

	cfg, err := config.Load(osFs, path)

	s.Require().Error(err)
	s.Require().Nil(cfg)
	s.Require().ErrorContains(err, "load config")
}

func (s *ConfigSuite) TestLoad_InvalidYAML_ReturnsWrappedError() {
	s.Require().NoError(afero.WriteFile(s.fs, s.path, []byte("not: [valid"), 0o600))

	cfg, err := config.Load(s.fs, s.path)

	s.Require().Error(err)
	s.Require().Nil(cfg)
	s.Require().ErrorContains(err, "load config")
}

func (s *ConfigSuite) TestGitLabConfig_URLTrailingSlashStripped() {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no trailing slash", in: "https://gitlab.com", want: "https://gitlab.com"},
		{name: "single trailing slash", in: "https://gitlab.com/", want: "https://gitlab.com"},
		{name: "multiple trailing slashes", in: "https://gitlab.com///", want: "https://gitlab.com"},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			body := []byte("gitlab:\n  url: " + tt.in + "\n")
			s.Require().NoError(afero.WriteFile(s.fs, s.path, body, 0o600))

			cfg, err := config.Load(s.fs, s.path)

			s.Require().NoError(err)
			s.Require().Equal(tt.want, cfg.GitLab.URL)
		})
	}
}

func (s *ConfigSuite) TestSave_RoundTrips() {
	cfg := config.Defaults()
	cfg.GitLab.Token = "secret"
	cfg.Repositories.Favorites = []string{"foo/bar"}

	s.Require().NoError(cfg.Save(s.fs, s.path))

	loaded, err := config.Load(s.fs, s.path)
	s.Require().NoError(err)
	s.Require().Equal(cfg, loaded)
}

func (s *ConfigSuite) TestDefaultConfigPath_EnvOverrideWins() {
	s.T().Setenv(config.EnvConfigPath, "/custom/path/lazylab.yaml")

	s.Require().Equal("/custom/path/lazylab.yaml", config.DefaultConfigPath())
}

func (s *ConfigSuite) TestDefaultConfigPath_UsesXDGConfigHome() {
	s.T().Setenv("XDG_CONFIG_HOME", "/xdg")

	s.Require().Equal(filepath.Join("/xdg", "lazylab", "config.yaml"), config.DefaultConfigPath())
}

func (s *ConfigSuite) TestDefaultConfigPath_FallsBackToUserHome() {
	s.T().Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	s.Require().NoError(err)

	got := config.DefaultConfigPath()

	s.Require().Equal(filepath.Join(home, ".config", "lazylab", "config.yaml"), got)
}

func (s *ConfigSuite) TestSave_ReadOnlyFs_ReturnsWrappedError() {
	ro := afero.NewReadOnlyFs(afero.NewMemMapFs())

	err := config.Defaults().Save(ro, s.path)

	s.Require().Error(err)
}

//nolint:paralleltest // t.Setenv incompatible with parallel ancestors
func TestConfigSuite(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}
