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
	conversationCfgPath = "/cfg/config.yaml"
	conversationWidth   = 160
	conversationHeight  = 48
)

const conversationProjectsFixture = `[
	{"id":42,"name":"Alpha","path_with_namespace":"grp/alpha","web_url":"https://gl/grp/alpha","last_activity_at":"2026-04-10T10:00:00Z","archived":false}
]`

const conversationOpenedMRsFixture = `[
	{
		"id":1,"iid":10,"title":"Feature Alpha",
		"state":"opened",
		"author":{"id":1,"username":"alice","name":"A","web_url":"u"},
		"source_branch":"feat/alpha","target_branch":"main",
		"web_url":"u",
		"created_at":"2026-04-10T14:30:00Z",
		"user_notes_count":4
	}
]`

const conversationDiscussionsFixture = `[
	{
		"id": "d-unresolved-1",
		"notes": [
			{
				"id":1,"body":"Is the bounds check needed here?",
				"author":{"id":10,"username":"jay","name":"Jay","web_url":"u"},
				"created_at":"2026-04-10T12:00:00Z",
				"resolvable":true,"resolved":false,"system":false,
				"position":{"new_path":"internal/tui/views/diff_tree.go","new_line":146}
			},
			{
				"id":2,"body":"Different path — placeCursor runs earlier.",
				"author":{"id":11,"username":"mira","name":"Mira","web_url":"u"},
				"created_at":"2026-04-10T13:00:00Z",
				"resolvable":true,"resolved":false,"system":false
			}
		]
	},
	{
		"id": "d-resolved-1",
		"notes": [
			{
				"id":10,"body":"check this",
				"author":{"id":12,"username":"priya","name":"Priya","web_url":"u"},
				"created_at":"2026-04-09T09:00:00Z",
				"resolvable":true,"resolved":true,
				"resolved_by":{"id":10,"username":"jay","name":"Jay","web_url":"u"},
				"system":false
			},
			{
				"id":11,"body":"fixed",
				"author":{"id":10,"username":"jay","name":"Jay","web_url":"u"},
				"created_at":"2026-04-09T10:00:00Z",
				"resolvable":true,"resolved":true,
				"resolved_by":{"id":10,"username":"jay","name":"Jay","web_url":"u"},
				"system":false
			}
		]
	},
	{
		"id": "d-general-1",
		"notes": [
			{
				"id":20,"body":"LGTM once the test is in.",
				"author":{"id":13,"username":"devon","name":"Devon","web_url":"u"},
				"created_at":"2026-04-10T08:00:00Z",
				"resolvable":false,"resolved":false,"system":false
			}
		]
	}
]`

type MRConversationSuite struct {
	suite.Suite
	srv     *httptest.Server
	fs      afero.Fs
	g       *gocui.Gui
	v       *views.Views
	app     *appcontext.AppContext
	manager func(*gocui.Gui) error
}

func (s *MRConversationSuite) SetupTest() {
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprint(w, conversationProjectsFixture)
		case strings.Contains(r.URL.Path, "/discussions"):
			_, _ = fmt.Fprint(w, conversationDiscussionsFixture)
		case strings.Contains(r.URL.Path, "/approvals"):
			_, _ = fmt.Fprint(w, `{"approved":true,"approvals_required":0,"approvals_left":0}`)
		case strings.Contains(r.URL.Path, "/diffs"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/pipelines"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/merge_requests"):
			_, _ = fmt.Fprint(w, conversationOpenedMRsFixture)
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
	s.Require().NoError(cfg.Save(s.fs, conversationCfgPath))

	client, err := gitlab.New(cfg.GitLab, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	s.app = appcontext.New(cfg, client, cache.New(cfg.Cache, s.fs), s.fs, conversationCfgPath)

	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: conversationWidth, Height: conversationHeight})
	s.Require().NoError(err)
	s.g = g

	s.v = views.New(g, s.app)
	s.manager = tui.NewManager(s.v)
	g.SetManagerFunc(s.manager)
	tui.SetFocusOrderProvider(s.v.FocusOrder)
	s.Require().NoError(tui.Bind(g, s.v.Bindings()...))
	s.Require().NoError(s.layoutTick())
}

func (s *MRConversationSuite) TearDownTest() {
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

func (s *MRConversationSuite) layoutTick() error { return s.manager(s.g) }

func (s *MRConversationSuite) buffer(name string) string {
	pane, err := s.g.View(name)
	s.Require().NoError(err)

	return pane.Buffer()
}

func (s *MRConversationSuite) dispatch(view string, key any) error {
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

func (s *MRConversationSuite) loadAndOpenConversationTab() {
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

	s.Require().NoError(s.v.Detail.SetTabSync(context.Background(), views.DetailTabConversation, project))
	s.Require().NoError(s.layoutTick())
}

func (s *MRConversationSuite) TestConversationTab_RendersUnresolvedThreadAndGeneralComment() {
	s.loadAndOpenConversationTab()

	buf := s.buffer(keymap.ViewDetailConversation)
	s.Require().Contains(buf, "Conversation · 2 threads (1 unresolved)",
		"chrome meta counts resolvable threads only")
	s.Require().Contains(buf, "Thread · d-unreso", "short-ID slug visible on unresolved thread")
	s.Require().Contains(buf, "unresolved")
	s.Require().Contains(buf, "@jay")
	s.Require().Contains(buf, "Is the bounds check needed here?")
	s.Require().Contains(buf, "@mira")
	s.Require().Contains(buf, "Different path")
	s.Require().Contains(buf, "General comments")
	s.Require().Contains(buf, "@devon")
	s.Require().Contains(buf, "LGTM once the test is in.")
}

func (s *MRConversationSuite) TestConversationTab_ResolvedThreadCollapsedByDefault() {
	s.loadAndOpenConversationTab()

	buf := s.buffer(keymap.ViewDetailConversation)
	s.Require().Contains(buf, "resolved by @jay",
		"collapsed resolved line shows resolver")
	s.Require().NotContains(buf, "check this",
		"collapsed resolved head body hidden until expand")
}

func (s *MRConversationSuite) TestConversationTab_ShiftE_ExpandsEveryResolvedThread() {
	s.loadAndOpenConversationTab()

	_, err := s.g.SetCurrentView(keymap.ViewDetailConversation)
	s.Require().NoError(err)
	s.Require().NoError(s.dispatch(keymap.ViewDetailConversation, 'E'))
	s.Require().NoError(s.layoutTick())

	buf := s.buffer(keymap.ViewDetailConversation)
	s.Require().Contains(buf, "check this",
		"E expands every resolved thread; head-note body now visible")
	s.Require().Contains(buf, "fixed")
}

func (s *MRConversationSuite) TestConversationTab_E_ExpandsOnlyTheThreadUnderCursor() {
	s.loadAndOpenConversationTab()

	_, err := s.g.SetCurrentView(keymap.ViewDetailConversation)
	s.Require().NoError(err)
	// Cursor is on the first unresolved thread; step past it to reach the
	// resolved (collapsed) thread.
	s.Require().NoError(s.dispatch(keymap.ViewDetailConversation, 'j'))
	s.Require().NoError(s.dispatch(keymap.ViewDetailConversation, 'e'))
	s.Require().NoError(s.layoutTick())

	s.Require().True(s.v.Detail.Conversation().IsResolvedThreadExpanded(0),
		"e expands the single thread under the cursor")

	buf := s.buffer(keymap.ViewDetailConversation)
	s.Require().Contains(buf, "check this")
}

func (s *MRConversationSuite) TestConversationTab_JCyclesTabBackToPipelineAndBackForwardToOverview() {
	s.loadAndOpenConversationTab()

	_, err := s.g.SetCurrentView(keymap.ViewDetailConversation)
	s.Require().NoError(err)

	s.Require().NoError(s.dispatch(keymap.ViewDetailConversation, ']'))
	s.Require().NoError(s.layoutTick())

	s.Require().Equal(views.DetailTabPipeline, s.v.Detail.CurrentTab(),
		"] from Conversation advances to Pipeline")
}

//nolint:paralleltest // gocui tcell simulation screen is global.
func TestMRConversationSuite(t *testing.T) {
	suite.Run(t, new(MRConversationSuite))
}
