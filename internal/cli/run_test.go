package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/cli"
)

type RunCommandSuite struct {
	suite.Suite
	buf *bytes.Buffer
}

func (s *RunCommandSuite) SetupTest() {
	s.buf = &bytes.Buffer{}
}

func (s *RunCommandSuite) TestRun_WritesStubMessage() {
	err := cli.Run(s.buf)

	s.Require().NoError(err)
	out := s.buf.String()
	s.Require().Contains(out, "not yet implemented")
	s.Require().Contains(out, "Phase G2")
}

func (s *RunCommandSuite) TestRun_WrapsWriteError() {
	err := cli.Run(failingWriter{})

	s.Require().Error(err)
	s.Require().ErrorContains(err, "write run stub")
}

func TestRunCommandSuite(t *testing.T) {
	suite.Run(t, new(RunCommandSuite))
}
