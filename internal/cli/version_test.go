package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/cli"
	"github.com/niklod/lazylab/internal/version"
)

type VersionCommandSuite struct {
	suite.Suite
	buf     *bytes.Buffer
	restore func()
}

func (s *VersionCommandSuite) SetupTest() {
	s.buf = &bytes.Buffer{}
}

func (s *VersionCommandSuite) TearDownTest() {
	if s.restore != nil {
		s.restore()
		s.restore = nil
	}
}

func (s *VersionCommandSuite) TestVersion_WritesDefaultVersion() {
	s.restore = version.SetForTest("dev")

	err := cli.Version(s.buf)

	s.Require().NoError(err)
	s.Require().Equal("lazylab dev\n", s.buf.String())
}

func (s *VersionCommandSuite) TestVersion_WritesInjectedVersion() {
	s.restore = version.SetForTest("v0.1.0")

	err := cli.Version(s.buf)

	s.Require().NoError(err)
	s.Require().Equal("lazylab v0.1.0\n", s.buf.String())
}

func (s *VersionCommandSuite) TestVersion_WrapsWriteError() {
	err := cli.Version(failingWriter{})

	s.Require().Error(err)
	s.Require().ErrorContains(err, "write version output")
}

func TestVersionCommandSuite(t *testing.T) {
	suite.Run(t, new(VersionCommandSuite))
}
