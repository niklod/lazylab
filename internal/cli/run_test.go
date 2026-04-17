package cli_test

import (
	"bytes"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/cli"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
	"github.com/niklod/lazylab/internal/tui"
)

const runTestConfigPath = "/cfg/config.yaml"

type RunCommandSuite struct {
	suite.Suite
	buf *bytes.Buffer
	fs  afero.Fs
}

func (s *RunCommandSuite) SetupTest() {
	s.buf = &bytes.Buffer{}
	s.fs = afero.NewMemMapFs()
}

func (s *RunCommandSuite) writeConfig(yaml string) {
	s.Require().NoError(afero.WriteFile(s.fs, runTestConfigPath, []byte(yaml), 0o600))
}

func (s *RunCommandSuite) TestRun_LoadsConfigThenRequiresTTY() {
	s.writeConfig("gitlab:\n  url: https://gitlab.example.com\n  token: secret\n")

	err := cli.Run(s.buf, cli.WithFS(s.fs), cli.WithConfigPath(runTestConfigPath))

	s.Require().ErrorIs(err, tui.ErrRequiresTTY)
	s.Require().ErrorContains(err, "run: tui")
}

func (s *RunCommandSuite) TestRun_SeedsDefaultsWhenConfigMissing() {
	s.T().Setenv(config.EnvGitLabToken, "env-token")

	err := cli.Run(s.buf, cli.WithFS(s.fs), cli.WithConfigPath(runTestConfigPath))

	s.Require().ErrorIs(err, tui.ErrRequiresTTY)
	exists, statErr := afero.Exists(s.fs, runTestConfigPath)
	s.Require().NoError(statErr)
	s.Require().True(exists)
}

func (s *RunCommandSuite) TestRun_WrapsConfigLoadError() {
	s.writeConfig("not: [valid")

	err := cli.Run(s.buf, cli.WithFS(s.fs), cli.WithConfigPath(runTestConfigPath))

	s.Require().Error(err)
	s.Require().ErrorContains(err, "run: load config")
}

func (s *RunCommandSuite) TestRun_WrapsClientBuildError() {
	s.T().Setenv(config.EnvGitLabToken, "")
	s.writeConfig("gitlab:\n  url: \"\"\n  token: \"\"\n")

	err := cli.Run(s.buf, cli.WithFS(s.fs), cli.WithConfigPath(runTestConfigPath))

	s.Require().Error(err)
	s.Require().ErrorContains(err, "run: build gitlab client")
	s.Require().ErrorIs(err, gitlab.ErrMissingToken)
}

//nolint:paralleltest // t.Setenv incompatible with parallel ancestors
func TestRunCommandSuite(t *testing.T) {
	suite.Run(t, new(RunCommandSuite))
}
