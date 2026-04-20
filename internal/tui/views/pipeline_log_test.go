package views

import (
	"strings"
	"testing"

	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
	"github.com/niklod/lazylab/internal/tui/theme"
)

type JobLogSuite struct {
	suite.Suite
	g *gocui.Gui
	v *JobLogView
}

func (s *JobLogSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 30})
	s.Require().NoError(err)
	s.g = g

	_, err = s.g.SetView(keymap.ViewDetailPipelineJobLog, 0, 0, 80, 25, 0)
	if err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
		s.T().Fatalf("SetView: %v", err)
	}

	s.v = NewJobLog()
}

func (s *JobLogSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
}

func (s *JobLogSuite) pane() *gocui.View {
	pane, err := s.g.View(keymap.ViewDetailPipelineJobLog)
	s.Require().NoError(err)

	return pane
}

func (s *JobLogSuite) job() *models.PipelineJob {
	d := 12.0

	return &models.PipelineJob{
		ID:       42,
		Name:     "test:unit",
		Stage:    "test",
		Status:   models.PipelineStatusFailed,
		Duration: &d,
	}
}

func (s *JobLogSuite) TestSetJob_RendersBodyAndFooter() {
	s.v.SetJob(s.job(), "line 1\nline 2\n")

	s.v.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, "line 1")
	s.Require().Contains(buf, "line 2")
	s.Require().Contains(buf, "end of log · 2 lines", "footer reflects trimmed body line count")
	s.Require().Contains(buf, "press y to copy", "footer advertises copy hint")
	s.Require().Contains(buf, "Esc to close", "footer advertises close hint")
	// Keybind strip moved to the global FooterView.
	s.Require().NotContains(buf, "j/k")
	s.Require().NotContains(buf, "test/test:unit",
		"job header lives on the pane chrome, not in the scrollable body")
	s.Require().Equal(s.job(), s.v.CurrentJob())
}

func (s *JobLogSuite) TestRenderJobLogFooter_Singular() {
	got := renderJobLogFooter(1)

	s.Require().Contains(got, "1 line ")
	s.Require().NotContains(got, "1 lines")
}

func (s *JobLogSuite) TestSetJob_EmptyTraceShowsPlaceholder() {
	s.v.SetJob(s.job(), "   \n\n")

	s.v.Render(s.pane())

	s.Require().Contains(s.pane().Buffer(), jobLogEmptyTrace)
}

func (s *JobLogSuite) TestSetJob_PreservesAnsiPassthrough() {
	s.v.SetJob(s.job(), ansiGreen+"build ok"+ansiReset)

	s.v.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, "build ok", "ANSI sequences passed through unchanged")
}

func (s *JobLogSuite) TestSetJob_NilClearsBackToEmpty() {
	s.v.SetJob(s.job(), "log")
	s.v.SetJob(nil, "")

	s.Require().Nil(s.v.CurrentJob())
	s.v.Render(s.pane())
	s.Require().Contains(s.pane().Buffer(), jobLogEmptyHint)
}

func (s *JobLogSuite) TestShowLoadingAndError() {
	s.v.ShowLoading()
	s.v.Render(s.pane())
	s.Require().Contains(s.pane().Buffer(), jobLogLoadingHint)

	s.v.ShowError("boom")
	s.v.Render(s.pane())
	s.Require().Contains(s.pane().Buffer(), "boom")
	s.Require().Contains(s.v.statusSnapshot(), theme.FgErr, "error status carries err color until render")
}

func (s *JobLogSuite) TestScrollBy_ClampsToContent() {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "line " + string(rune('A'+i%26))
	}
	s.v.SetJob(s.job(), strings.Join(lines, "\n"))
	pane := s.pane()

	pane.SetOrigin(0, 0)
	s.v.ScrollBy(pane, 100)
	_, oy := pane.Origin()
	s.Require().GreaterOrEqual(oy, 0, "scroll clamped")

	s.v.ScrollBy(pane, -100)
	_, oy = pane.Origin()
	s.Require().Equal(0, oy)
}

func (s *JobLogSuite) TestScrollToTop_ResetsOrigin() {
	pane := s.pane()
	pane.SetOrigin(0, 5)

	s.v.ScrollToTop(pane)

	_, oy := pane.Origin()
	s.Require().Equal(0, oy)
}

func (s *JobLogSuite) TestSanitizeTraceBody_StripsDangerousEscapes() {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "SGR passes through", in: "\x1b[31mred\x1b[0m", want: "\x1b[31mred\x1b[0m"},
		{name: "cursor move stripped", in: "before\x1b[2Jafter", want: "beforeafter"},
		{name: "cursor reposition stripped", in: "x\x1b[10;20Hy", want: "xy"},
		{name: "OSC hyperlink stripped", in: "a\x1b]8;;https://e.vil\x07b\x1b]8;;\x07c", want: "abc"},
		{name: "OSC with ST terminator stripped", in: "pre\x1b]0;title\x1b\\post", want: "prepost"},
		{name: "NUL and BS dropped", in: "a\x00b\x08c", want: "abc"},
		{name: "bare CR becomes LF", in: "line1\rline2", want: "line1\nline2"},
		{name: "CRLF normalizes to LF", in: "line1\r\nline2", want: "line1\nline2"},
		{name: "two-byte ESC sequence dropped", in: "a\x1b=b", want: "ab"},
		{name: "plain text untouched", in: "hello\tworld\n", want: "hello\tworld\n"},
		{name: "unterminated OSC drops rest", in: "keep\x1b]evil_never_closed", want: "keep"},
		{name: "unterminated CSI drops rest", in: "keep\x1b[01234", want: "keep"},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Require().Equal(tt.want, sanitizeTraceBody(tt.in))
		})
	}
}

func (s *JobLogSuite) TestSetJob_StripsCursorMoveFromMaliciousTrace() {
	malicious := "safe output\n\x1b[2J\x1b[Hclear-screen injected"
	s.v.SetJob(s.job(), malicious)

	s.v.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, "safe output")
	s.Require().Contains(buf, "clear-screen injected")
	s.Require().NotContains(buf, "\x1b[2J", "screen-clear escape must be stripped before render")
}

// statusSnapshot lets tests assert the cached status string without driving
// a pane render (pane.Buffer() strips ANSI colour codes).
func (l *JobLogView) statusSnapshot() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.status
}

//nolint:paralleltest // gocui tcell sim screen is global.
func TestJobLogSuite(t *testing.T) {
	suite.Run(t, new(JobLogSuite))
}
