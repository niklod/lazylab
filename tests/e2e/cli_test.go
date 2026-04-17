//go:build e2e

package e2e_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
)

const testVersion = "e2e-test"

var (
	buildOnce sync.Once
	buildPath string
	buildDir  string
	buildErr  error
)

type CLISuite struct {
	suite.Suite
	binary string
}

func (s *CLISuite) SetupSuite() {
	buildOnce.Do(buildBinary)
	s.Require().NoError(buildErr)
	s.binary = buildPath
}

func (s *CLISuite) TearDownSuite() {
	if buildDir != "" {
		_ = os.RemoveAll(buildDir)
	}
}

func buildBinary() {
	tmp, err := os.MkdirTemp("", "lazylab-e2e-*")
	if err != nil {
		buildErr = fmt.Errorf("mkdir temp: %w", err)
		return
	}
	buildDir = tmp
	binary := filepath.Join(tmp, "lazylab")

	wd, err := os.Getwd()
	if err != nil {
		buildErr = fmt.Errorf("getwd: %w", err)
		return
	}
	root := filepath.Join(wd, "..", "..")

	cmd := exec.Command(
		"go", "build",
		"-ldflags", "-X github.com/niklod/lazylab/internal/version.version="+testVersion,
		"-o", binary,
		"./cmd/lazylab",
	)
	cmd.Dir = root

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		buildErr = fmt.Errorf("build failed: %w\nstderr: %s", err, stderr.String())
		return
	}
	buildPath = binary
}

func (s *CLISuite) run(args ...string) (stdout, stderr string, exitCode int) {
	return s.runWithEnv(nil, args...)
}

func (s *CLISuite) runWithEnv(extraEnv []string, args ...string) (stdout, stderr string, exitCode int) {
	s.T().Helper()

	cmd := exec.Command(s.binary, args...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		exitCode = 0
	case errors.As(err, &exitErr):
		exitCode = exitErr.ExitCode()
	default:
		s.T().Fatalf("exec lazylab %v: %v", args, err)
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func (s *CLISuite) TestVersion_PrintsInjectedVersion() {
	stdout, stderr, code := s.run("version")

	s.Require().Equal(0, code)
	s.Require().Equal("lazylab "+testVersion+"\n", stdout)
	s.Require().Empty(stderr)
}

func (s *CLISuite) TestRun_LoadsConfigThenFailsWithoutTTY() {
	dir := s.T().TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	s.Require().NoError(os.WriteFile(configPath, []byte(
		"gitlab:\n  url: https://gitlab.example.com\n  token: e2e-secret\n",
	), 0o600))

	_, stderr, code := s.runWithEnv(
		[]string{"LAZYLAB_CONFIG=" + configPath, "LAZYLAB_GITLAB_TOKEN="},
		"run",
	)

	s.Require().NotEqual(0, code)
	s.Require().Contains(stderr, "requires an interactive terminal")
}

func (s *CLISuite) TestRun_MissingTokenExitsNonZero() {
	dir := s.T().TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	s.Require().NoError(os.WriteFile(configPath, []byte(
		"gitlab:\n  url: https://gitlab.example.com\n  token: \"\"\n",
	), 0o600))

	_, stderr, code := s.runWithEnv(
		[]string{"LAZYLAB_CONFIG=" + configPath, "LAZYLAB_GITLAB_TOKEN="},
		"run",
	)

	s.Require().NotEqual(0, code)
	s.Require().Contains(stderr, "token")
}

func (s *CLISuite) TestNoSubcommand_ExitsNonZeroAndPrintsHelp() {
	stdout, stderr, code := s.run()

	s.Require().NotEqual(0, code)
	combined := stdout + stderr
	s.Require().Contains(combined, "Terminal UI for GitLab")
	s.Require().Contains(combined, "version")
	s.Require().Contains(combined, "run")
}

func (s *CLISuite) TestUnknownSubcommand_ExitsNonZeroAndWritesStderr() {
	_, stderr, code := s.run("bogus-subcommand-xyz")

	s.Require().NotEqual(0, code)
	s.Require().NotEmpty(stderr)
}

func (s *CLISuite) TestHelpFlag_PrintsUsage() {
	stdout, stderr, code := s.run("--help")

	s.Require().Equal(0, code)
	combined := stdout + stderr
	s.Require().Contains(combined, "Terminal UI for GitLab")
	s.Require().Contains(combined, "version")
	s.Require().Contains(combined, "run")
}

func TestCLISuite(t *testing.T) {
	suite.Run(t, new(CLISuite))
}
