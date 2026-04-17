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

type DiffContentSuite struct {
	suite.Suite
	g *gocui.Gui
	v *DiffContentView
}

func (s *DiffContentSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	s.Require().NoError(err)
	s.g = g

	_, err = s.g.SetView(keymap.ViewDetailDiffContent, 0, 0, 100, 20, 0)
	if err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
		s.T().Fatalf("SetView: %v", err)
	}

	s.v = NewDiffContent()
}

func (s *DiffContentSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
}

func (s *DiffContentSuite) TestRenderDiffMarkup_AppliesPythonParityColors() {
	raw := strings.Join([]string{
		"--- a/foo.go",
		"+++ b/foo.go",
		"@@ -1,3 +1,4 @@",
		" context line",
		"-removed",
		"+added",
	}, "\n")

	out := renderDiffMarkup(raw)

	s.Require().Contains(out, ansiBold+"--- a/foo.go"+ansiReset)
	s.Require().Contains(out, ansiBold+"+++ b/foo.go"+ansiReset)
	s.Require().Contains(out, ansiCyan+"@@ -1,3 +1,4 @@"+ansiReset)
	s.Require().Contains(out, ansiRed+"-removed"+ansiReset)
	s.Require().Contains(out, ansiGreen+"+added"+ansiReset)
	s.Require().Contains(out, " context line")
}

func (s *DiffContentSuite) TestRenderDiffMarkup_EmptyDiffReturnsHint() {
	out := renderDiffMarkup("")

	s.Require().Equal(ansiDim+diffBinaryHint+ansiReset, out)
}

func (s *DiffContentSuite) TestRenderDiffMarkup_WhitespaceOnlyReturnsHint() {
	out := renderDiffMarkup("   \n\t\n")

	s.Require().Equal(ansiDim+diffBinaryHint+ansiReset, out)
}

func (s *DiffContentSuite) TestSetFile_NilShowsEmptyHint() {
	pv, err := s.g.View(keymap.ViewDetailDiffContent)
	s.Require().NoError(err)

	s.v.SetFile(nil)
	s.v.Render(pv)

	s.Require().Contains(pv.Buffer(), diffEmptyHint)
}

func (s *DiffContentSuite) TestSetFile_RendersDiffBody() {
	pv, err := s.g.View(keymap.ViewDetailDiffContent)
	s.Require().NoError(err)

	s.v.SetFile(&models.MRDiffFile{
		NewPath: "src/main.go",
		Diff:    "@@ -1 +1 @@\n-old\n+new\n",
	})
	s.v.Render(pv)

	buf := pv.Buffer()
	s.Require().Contains(buf, "-old")
	s.Require().Contains(buf, "+new")
}

func (s *DiffContentSuite) TestShowLoading_ShowsLoadingHint() {
	pv, err := s.g.View(keymap.ViewDetailDiffContent)
	s.Require().NoError(err)

	s.v.ShowLoading()
	s.v.Render(pv)

	s.Require().Contains(pv.Buffer(), diffLoadingHint)
}

func (s *DiffContentSuite) TestShowError_ShowsRedMessage() {
	pv, err := s.g.View(keymap.ViewDetailDiffContent)
	s.Require().NoError(err)

	s.v.ShowError("boom")
	s.v.Render(pv)

	buf := pv.Buffer()
	s.Require().Contains(buf, "boom")
}

func (s *DiffContentSuite) TestScrollBy_ClampsToContentExtent() {
	pv, err := s.g.View(keymap.ViewDetailDiffContent)
	s.Require().NoError(err)

	lines := make([]string, 80)
	for i := range lines {
		lines[i] = "line"
	}
	s.v.SetFile(&models.MRDiffFile{Diff: strings.Join(lines, "\n")})
	s.v.Render(pv)

	s.v.ScrollBy(pv, 10)
	_, oy := pv.Origin()
	s.Require().Equal(10, oy)

	s.v.ScrollBy(pv, 10_000)
	_, oy = pv.Origin()
	_, innerH := pv.InnerSize()
	s.Require().Equal(80-innerH, oy, "clamped to max origin")

	s.v.ScrollBy(pv, -1_000)
	_, oy = pv.Origin()
	s.Require().Equal(0, oy, "clamped to zero")
}

func (s *DiffContentSuite) TestScrollToTop_ResetsOrigin() {
	pv, err := s.g.View(keymap.ViewDetailDiffContent)
	s.Require().NoError(err)

	pv.SetOrigin(0, 5)

	s.v.ScrollToTop(pv)

	_, oy := pv.Origin()
	s.Require().Equal(0, oy)
}

//nolint:paralleltest // gocui stores tcell simulation screen in a global; parallel runs race.
func TestDiffContentSuite(t *testing.T) {
	suite.Run(t, new(DiffContentSuite))
}
