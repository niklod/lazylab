package version_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/version"
)

type VersionSuite struct {
	suite.Suite
	restore func()
}

func (s *VersionSuite) TearDownTest() {
	if s.restore != nil {
		s.restore()
		s.restore = nil
	}
}

func (s *VersionSuite) TestString_ReturnsDefault() {
	s.restore = version.SetForTest("dev")
	s.Require().Equal("dev", version.String())
}

func (s *VersionSuite) TestString_ReturnsInjectedValue() {
	s.restore = version.SetForTest("v1.2.3")
	s.Require().Equal("v1.2.3", version.String())
}

func TestVersionSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(VersionSuite))
}
