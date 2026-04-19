//go:build e2e

package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
	"github.com/niklod/lazylab/internal/tui"
	"github.com/niklod/lazylab/internal/tui/keymap"
	"github.com/niklod/lazylab/internal/tui/views"
)

const (
	pipelineCfgPath = "/cfg/config.yaml"
	pipelineWidth   = 160
	pipelineHeight  = 48
)

const pipelineProjectsFixture = `[
	{"id":42,"name":"Alpha","path_with_namespace":"grp/alpha","web_url":"https://gl/grp/alpha","last_activity_at":"2026-04-10T10:00:00Z","archived":false}
]`

const pipelineOpenedMRsFixture = `[
	{
		"id":1,"iid":10,"title":"Feature Alpha",
		"state":"opened",
		"author":{"id":1,"username":"alice","name":"A","web_url":"u"},
		"source_branch":"feat/alpha","target_branch":"main",
		"web_url":"u",
		"created_at":"2026-04-10T14:30:00Z",
		"user_notes_count":0
	}
]`

const pipelinesListFixture = `[
	{"id":77,"iid":1,"project_id":42,"status":"failed","ref":"feat/alpha","web_url":"u","updated_at":"2026-04-10T15:00:00Z"}
]`

const pipelineGetFixture = `{
	"id":77,"status":"failed","ref":"feat/alpha","sha":"deadbeef","web_url":"u",
	"created_at":"2026-04-10T14:45:00Z","updated_at":"2026-04-10T15:00:00Z"
}`

// pipelineJobsFixture mirrors the real GitLab response shape — jobs are
// sorted newest-first (descending ID). Client.GetMRPipelineDetail reverses
// this to pipeline execution order so the UI renders build → test → deploy.
const pipelineJobsFixture = `[
	{"id":103,"name":"test:lint","stage":"test","status":"success","web_url":"u3","duration":7.0,"allow_failure":false},
	{"id":102,"name":"test:unit","stage":"test","status":"failed","web_url":"u2","duration":44.0,"allow_failure":false},
	{"id":101,"name":"build:binary","stage":"build","status":"success","web_url":"u1","duration":12.0,"allow_failure":false}
]`

const pipelineJobTraceFixture = "starting test:unit\nstep 1 ok\nstep 2 FAILED: expected ok\n"

type MRPipelineSuite struct {
	suite.Suite
	srv     *httptest.Server
	fs      afero.Fs
	g       *gocui.Gui
	v       *views.Views
	app     *appcontext.AppContext
	manager func(*gocui.Gui) error
}

func (s *MRPipelineSuite) SetupTest() {
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprint(w, pipelineProjectsFixture)
		case strings.Contains(r.URL.Path, "/merge_requests/10/pipelines"):
			_, _ = fmt.Fprint(w, pipelinesListFixture)
		case strings.Contains(r.URL.Path, "/pipelines/77/jobs"):
			_, _ = fmt.Fprint(w, pipelineJobsFixture)
		case strings.Contains(r.URL.Path, "/pipelines/77"):
			_, _ = fmt.Fprint(w, pipelineGetFixture)
		case strings.Contains(r.URL.Path, "/jobs/102/trace"):
			_, _ = fmt.Fprint(w, pipelineJobTraceFixture)
		case strings.Contains(r.URL.Path, "/jobs/") && strings.HasSuffix(r.URL.Path, "/trace"):
			_, _ = fmt.Fprint(w, "")
		case strings.Contains(r.URL.Path, "/discussions"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/approvals"):
			_, _ = fmt.Fprint(w, `{"approved":true,"approvals_required":0,"approvals_left":0}`)
		case strings.Contains(r.URL.Path, "/diffs"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/merge_requests"):
			_, _ = fmt.Fprint(w, pipelineOpenedMRsFixture)
		case strings.HasSuffix(r.URL.Path, "/user"):
			_, _ = fmt.Fprint(w, `{"id":99,"username":"me","name":"Me","web_url":"u"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	s.fs = afero.NewMemMapFs()
	cfg := config.Defaults()
	cfg.GitLab.URL = s.srv.URL
	cfg.GitLab.Token = "e2e-secret"
	s.Require().NoError(cfg.Save(s.fs, pipelineCfgPath))

	client, err := gitlab.New(cfg.GitLab, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	s.app = appcontext.New(cfg, client, cache.New(cfg.Cache, s.fs), s.fs, pipelineCfgPath)

	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: pipelineWidth, Height: pipelineHeight})
	s.Require().NoError(err)
	s.g = g

	s.v = views.New(g, s.app)
	s.manager = tui.NewManager(s.v)
	g.SetManagerFunc(s.manager)
	tui.SetFocusOrderProvider(s.v.FocusOrder)
	s.Require().NoError(tui.Bind(g, s.v.Bindings()...))
	s.Require().NoError(s.layoutTick())
}

func (s *MRPipelineSuite) TearDownTest() {
	tui.SetFocusOrderProvider(nil)
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

func (s *MRPipelineSuite) layoutTick() error { return s.manager(s.g) }

func (s *MRPipelineSuite) buffer(name string) string {
	pane, err := s.g.View(name)
	s.Require().NoError(err)

	return pane.Buffer()
}

func (s *MRPipelineSuite) dispatch(view string, key any) error {
	s.T().Helper()

	v, err := s.g.View(view)
	if err != nil {
		v = nil
	}
	for _, b := range s.v.Bindings() {
		if b.View == view && b.Key == key {
			return b.Handler(s.g, v)
		}
	}
	s.T().Fatalf("no binding for view=%q key=%v", view, key)

	return nil
}

func (s *MRPipelineSuite) loadAndOpenPipelineTab() {
	s.Require().NoError(s.v.Repos.LoadSync(context.Background()))
	s.Require().NoError(s.layoutTick())

	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	s.Require().NoError(s.v.MRs.SetProjectSync(context.Background(), project))
	s.Require().NoError(s.layoutTick())

	_, err := s.g.SetCurrentView(keymap.ViewMRs)
	s.Require().NoError(err)
	s.Require().NoError(s.dispatch(keymap.ViewMRs, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	s.Require().NoError(s.v.Detail.SetTabSync(context.Background(), views.DetailTabPipeline, project))
	s.Require().NoError(s.layoutTick())
}

func (s *MRPipelineSuite) TestPipelineTab_RendersStages() {
	s.loadAndOpenPipelineTab()

	buf := s.buffer(keymap.ViewDetailPipelineStages)
	s.Require().Contains(buf, "build")
	s.Require().Contains(buf, "test")
	s.Require().Contains(buf, "build:binary")
	s.Require().Contains(buf, "test:unit")
	s.Require().Contains(buf, "test:lint")
}

func (s *MRPipelineSuite) TestPipelineTab_EnterOpensJobLogAndEscCloses() {
	s.loadAndOpenPipelineTab()

	_, err := s.g.SetCurrentView(keymap.ViewDetailPipelineStages)
	s.Require().NoError(err)

	// Advance cursor to the failed job (test:unit → second job row).
	s.Require().NoError(s.dispatch(keymap.ViewDetailPipelineStages, 'j'))

	selected := s.v.Detail.PipelineStages().SelectedJob()
	s.Require().NotNil(selected)
	s.Require().Equal("test:unit", selected.Name)

	project := s.v.Repos.SelectedProject()
	s.Require().NoError(s.v.Detail.OpenJobLogSync(context.Background(), project))
	s.Require().NoError(s.layoutTick())

	s.Require().True(s.v.Detail.LogOpen())

	logBuf := s.buffer(keymap.ViewDetailPipelineJobLog)
	s.Require().Contains(logBuf, "Log · ", "pane chrome shows Log title")
	s.Require().Contains(logBuf, "test:unit", "job name in chrome")
	s.Require().Contains(logBuf, "stage test", "meta line shows stage")
	s.Require().Contains(logBuf, "step 1 ok")
	s.Require().Contains(logBuf, "step 2 FAILED")
	s.Require().Contains(logBuf, "end of log", "footer rendered below body")

	// Stages pane must be unmounted while log is open.
	_, err = s.g.View(keymap.ViewDetailPipelineStages)
	s.Require().Error(err, "stages pane is replaced by log while open")

	s.Require().NoError(s.dispatch(keymap.ViewDetailPipelineJobLog, gocui.KeyEsc))
	s.Require().NoError(s.layoutTick())

	s.Require().False(s.v.Detail.LogOpen())
	// Log pane unmounted, stages pane remounted and focused.
	_, err = s.g.View(keymap.ViewDetailPipelineJobLog)
	s.Require().Error(err, "log pane unmounted after Esc")
	_, err = s.g.View(keymap.ViewDetailPipelineStages)
	s.Require().NoError(err, "stages pane remounted after Esc")
	s.Require().Equal(keymap.ViewDetailPipelineStages, s.g.CurrentView().Name(),
		"focus returns to stages pane after closing log")
}

func (s *MRPipelineSuite) TestPipelineTab_RendersStagesInExecutionOrder() {
	s.loadAndOpenPipelineTab()

	buf := s.buffer(keymap.ViewDetailPipelineStages)
	buildIdx := strings.Index(buf, "build")
	testIdx := strings.Index(buf, "test")
	s.Require().NotEqual(-1, buildIdx, "build stage rendered")
	s.Require().NotEqual(-1, testIdx, "test stage rendered")
	s.Require().Less(buildIdx, testIdx,
		"build stage must come before test stage — API order reversed to exec order")
}

func (s *MRPipelineSuite) TestPipelineTab_JobLog_gG_JumpToTopAndBottom() {
	s.loadAndOpenPipelineTab()

	_, err := s.g.SetCurrentView(keymap.ViewDetailPipelineStages)
	s.Require().NoError(err)
	s.Require().NoError(s.dispatch(keymap.ViewDetailPipelineStages, 'j'))

	project := s.v.Repos.SelectedProject()
	s.Require().NoError(s.v.Detail.OpenJobLogSync(context.Background(), project))
	s.Require().NoError(s.layoutTick())

	pane, err := s.g.View(keymap.ViewDetailPipelineJobLog)
	s.Require().NoError(err)

	// Jump to bottom via G, then top via g, ensuring both handlers are wired.
	pane.SetOrigin(0, 0)
	s.Require().NoError(s.dispatch(keymap.ViewDetailPipelineJobLog, 'G'))
	_, oyBottom := pane.Origin()

	s.Require().NoError(s.dispatch(keymap.ViewDetailPipelineJobLog, 'g'))
	_, oyTop := pane.Origin()

	s.Require().Equal(0, oyTop, "g resets origin to top")
	s.Require().GreaterOrEqual(oyBottom, oyTop, "G advances origin past top (clamps if content fits)")
}

func (s *MRPipelineSuite) TestPipelineTab_StagesHighlightSurvivesLogOpenAndClose() {
	s.loadAndOpenPipelineTab()

	_, err := s.g.SetCurrentView(keymap.ViewDetailPipelineStages)
	s.Require().NoError(err)
	s.Require().NoError(s.dispatch(keymap.ViewDetailPipelineStages, 'j'))

	project := s.v.Repos.SelectedProject()
	s.Require().NoError(s.v.Detail.OpenJobLogSync(context.Background(), project))
	s.Require().NoError(s.layoutTick())

	s.Require().NoError(s.dispatch(keymap.ViewDetailPipelineJobLog, gocui.KeyEsc))
	s.Require().NoError(s.layoutTick())

	pane, err := s.g.View(keymap.ViewDetailPipelineStages)
	s.Require().NoError(err)
	s.Require().True(pane.Highlight,
		"highlight property must be re-applied to the remounted stages pane")
}

func (s *MRPipelineSuite) TestPipelineTab_TabCycleFromPipelineFamilyWorks() {
	s.loadAndOpenPipelineTab()

	_, err := s.g.SetCurrentView(keymap.ViewDetailPipelineStages)
	s.Require().NoError(err)

	s.Require().NoError(s.dispatch(keymap.ViewDetailPipelineStages, '['))
	s.Require().NoError(s.layoutTick())

	s.Require().Equal(views.DetailTabConversation, s.v.Detail.CurrentTab(),
		"[ from Pipeline cycles to Conversation")
}

//nolint:paralleltest // gocui tcell simulation screen is global.
func TestMRPipelineSuite(t *testing.T) {
	suite.Run(t, new(MRPipelineSuite))
}
