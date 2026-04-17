package cli_test

import (
	"bytes"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/cli"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
)

type RunCommandSuite struct {
	suite.Suite
	buf *bytes.Buffer
	fs  afero.Fs
}

func (s *RunCommandSuite) SetupTest() {
	s.buf = &bytes.Buffer{}
	s.fs = afero.NewMemMapFs()
}

func (s *RunCommandSuite) writeConfig(path, yaml string) {
	s.Require().NoError(afero.WriteFile(s.fs, path, []byte(yaml), 0o600))
}

func (s *RunCommandSuite) TestRun_LoadsConfigAndReportsReady() {
	path := "/cfg/config.yaml"
	s.writeConfig(path, "gitlab:\n  url: https://gitlab.example.com\n  token: secret\n")

	err := cli.Run(s.buf, cli.WithFS(s.fs), cli.WithConfigPath(path))

	s.Require().NoError(err)
	s.Require().Contains(s.buf.String(), "config loaded")
	s.Require().Contains(s.buf.String(), "Phase G2")
}

func (s *RunCommandSuite) TestRun_SeedsDefaultsWhenConfigMissing() {
	s.T().Setenv(config.EnvGitLabToken, "env-token")
	path := "/cfg/config.yaml"

	err := cli.Run(s.buf, cli.WithFS(s.fs), cli.WithConfigPath(path))

	s.Require().NoError(err)
	exists, statErr := afero.Exists(s.fs, path)
	s.Require().NoError(statErr)
	s.Require().True(exists)
}

func (s *RunCommandSuite) TestRun_WrapsConfigLoadError() {
	path := "/cfg/config.yaml"
	s.writeConfig(path, "not: [valid")

	err := cli.Run(s.buf, cli.WithFS(s.fs), cli.WithConfigPath(path))

	s.Require().Error(err)
	s.Require().ErrorContains(err, "run: load config")
}

func (s *RunCommandSuite) TestRun_WrapsClientBuildError() {
	s.T().Setenv(config.EnvGitLabToken, "")
	path := "/cfg/config.yaml"
	s.writeConfig(path, "gitlab:\n  url: \"\"\n  token: \"\"\n")

	err := cli.Run(s.buf, cli.WithFS(s.fs), cli.WithConfigPath(path))

	s.Require().Error(err)
	s.Require().ErrorContains(err, "run: build gitlab client")
	s.Require().ErrorIs(err, gitlab.ErrMissingToken)
}

func (s *RunCommandSuite) TestRun_WrapsWriteError() {
	path := "/cfg/config.yaml"
	s.writeConfig(path, "gitlab:\n  url: https://gitlab.example.com\n  token: secret\n")

	err := cli.Run(failingWriter{}, cli.WithFS(s.fs), cli.WithConfigPath(path))

	s.Require().Error(err)
	s.Require().ErrorContains(err, "run: write output")
}

func TestRunCommandSuite(t *testing.T) {
	suite.Run(t, new(RunCommandSuite))
}
