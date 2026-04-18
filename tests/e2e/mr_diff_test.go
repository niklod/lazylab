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
	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui"
	"github.com/niklod/lazylab/internal/tui/keymap"
	"github.com/niklod/lazylab/internal/tui/views"
)

const (
	diffCfgPath = "/cfg/config.yaml"
	diffWidth   = 160
	diffHeight  = 48
)

const diffProjectsFixture = `[
	{"id":42,"name":"Alpha","path_with_namespace":"grp/alpha","web_url":"https://gl/grp/alpha","last_activity_at":"2026-04-10T10:00:00Z","archived":false}
]`

const diffOpenedMRsFixture = `[
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

const diffChangesFixture = `[
	{"old_path":"README.md","new_path":"README.md","diff":"@@ -1 +1 @@\n-old title\n+new title\n"},
	{"old_path":"src/main.go","new_path":"src/main.go","diff":"@@ -1 +1 @@\n-x\n+y\n"},
	{"old_path":"src/util.go","new_path":"src/util.go","diff":"@@ -0,0 +1,3 @@\n+package util\n+\n+func Do() {}\n","new_file":true}
]`

type MRDiffSuite struct {
	suite.Suite
	srv     *httptest.Server
	fs      afero.Fs
	g       *gocui.Gui
	v       *views.Views
	app     *appcontext.AppContext
	manager func(*gocui.Gui) error
}

func (s *MRDiffSuite) SetupTest() {
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprint(w, diffProjectsFixture)
		case strings.Contains(r.URL.Path, "/diffs"):
			_, _ = fmt.Fprint(w, diffChangesFixture)
		case strings.Contains(r.URL.Path, "/discussions"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/merge_requests"):
			_, _ = fmt.Fprint(w, diffOpenedMRsFixture)
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
	s.Require().NoError(cfg.Save(s.fs, diffCfgPath))

	client, err := gitlab.New(cfg.GitLab, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	s.app = appcontext.New(cfg, client, cache.New(cfg.Cache, s.fs), s.fs, diffCfgPath)

	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: diffWidth, Height: diffHeight})
	s.Require().NoError(err)
	s.g = g

	s.v = views.New(g, s.app)
	s.manager = tui.NewManager(s.v)
	g.SetManagerFunc(s.manager)
	tui.SetFocusOrderProvider(s.v.FocusOrder)
	s.Require().NoError(tui.Bind(g, s.v.Bindings()...))
	s.Require().NoError(s.layoutTick())
}

func (s *MRDiffSuite) TearDownTest() {
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

func (s *MRDiffSuite) layoutTick() error { return s.manager(s.g) }

func (s *MRDiffSuite) buffer(name string) string {
	pane, err := s.g.View(name)
	s.Require().NoError(err)

	return pane.Buffer()
}

func (s *MRDiffSuite) dispatch(view string, key any) error {
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

func (s *MRDiffSuite) loadAndSelectMR() {
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

	s.Require().NoError(s.v.Detail.SetTabSync(context.Background(), views.DetailTabDiff, project))
	s.Require().NoError(s.layoutTick())
}

func (s *MRDiffSuite) TestDiffTab_RendersTreeAndContent() {
	s.loadAndSelectMR()

	treeBuf := s.buffer(keymap.ViewDetailDiffTree)
	s.Require().Contains(treeBuf, "README.md")
	s.Require().Contains(treeBuf, "main.go")
	s.Require().Contains(treeBuf, "util.go")

	contentBuf := s.buffer(keymap.ViewDetailDiffContent)
	s.Require().Contains(contentBuf, "-old title")
	s.Require().Contains(contentBuf, "+new title")
}

func (s *MRDiffSuite) TestDiffTab_EnterOnTreeLeafSwapsContent() {
	s.loadAndSelectMR()

	_, err := s.g.SetCurrentView(keymap.ViewDetailDiffTree)
	s.Require().NoError(err)

	s.Require().NoError(s.dispatch(keymap.ViewDetailDiffTree, 'j'))
	s.Require().NoError(s.layoutTick())

	selected := s.v.Detail.DiffTree().SelectedFile()
	s.Require().NotNil(selected)
	s.Require().Equal("src/main.go", selected.NewPath)

	s.Require().NoError(s.dispatch(keymap.ViewDetailDiffTree, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	contentBuf := s.buffer(keymap.ViewDetailDiffContent)
	s.Require().Contains(contentBuf, "-x")
	s.Require().Contains(contentBuf, "+y")
}

func (s *MRDiffSuite) TestDiffTab_CtrlDScrollsContentPane() {
	s.loadAndSelectMR()

	s.v.Detail.DiffTree().MoveCursorToEnd()
	s.v.Detail.SelectDiffFile(s.g)
	s.Require().NoError(s.layoutTick())

	content, err := s.g.View(keymap.ViewDetailDiffContent)
	s.Require().NoError(err)

	_, oyBefore := content.Origin()
	s.Require().NoError(s.dispatch(keymap.ViewDetailDiffContent, gocui.KeyCtrlD))
	s.Require().NoError(s.layoutTick())

	_, oyAfter := content.Origin()
	s.Require().GreaterOrEqual(oyAfter, oyBefore, "ctrl+d is clamped to content extent but never goes backwards")
}

func (s *MRDiffSuite) TestDiffTab_AllTreeKeys_MoveCursor() {
	s.loadAndSelectMR()

	_, err := s.g.SetCurrentView(keymap.ViewDetailDiffTree)
	s.Require().NoError(err)

	tree := s.v.Detail.DiffTree()
	s.Require().GreaterOrEqual(tree.RowCount(), 3)

	start := tree.Cursor()

	downKeys := []any{'j', gocui.KeyArrowDown}
	for _, k := range downKeys {
		tree.MoveCursorToStart()
		s.Require().NoError(s.dispatch(keymap.ViewDetailDiffTree, k))
		s.Require().Greater(tree.Cursor(), start, "key %v must advance cursor", k)
	}

	upKeys := []any{'k', gocui.KeyArrowUp}
	for _, k := range upKeys {
		tree.MoveCursorToEnd()
		before := tree.Cursor()
		s.Require().NoError(s.dispatch(keymap.ViewDetailDiffTree, k))
		s.Require().Less(tree.Cursor(), before, "key %v must retreat cursor", k)
	}

	tree.MoveCursorToEnd()
	last := tree.Cursor()
	s.Require().NoError(s.dispatch(keymap.ViewDetailDiffTree, 'g'))
	s.Require().Less(tree.Cursor(), last, "g jumps to first leaf")

	s.Require().NoError(s.dispatch(keymap.ViewDetailDiffTree, 'G'))
	s.Require().Equal(last, tree.Cursor(), "G jumps to last leaf")
}

func (s *MRDiffSuite) TestDiffTab_TreeCtrlDScrollsContent() {
	s.loadAndSelectMR()

	lines := make([]string, 80)
	for i := range lines {
		lines[i] = "+line" + fmt.Sprintf("%d", i)
	}
	s.v.Detail.DiffContent().SetFile(&models.MRDiffFile{Diff: strings.Join(lines, "\n")})
	s.Require().NoError(s.layoutTick())

	_, err := s.g.SetCurrentView(keymap.ViewDetailDiffTree)
	s.Require().NoError(err)

	content, err := s.g.View(keymap.ViewDetailDiffContent)
	s.Require().NoError(err)
	content.SetOrigin(0, 0)

	tree := s.v.Detail.DiffTree()
	cursorBefore := tree.Cursor()

	s.Require().NoError(s.dispatch(keymap.ViewDetailDiffTree, gocui.KeyCtrlD))

	_, oy := content.Origin()
	s.Require().Greater(oy, 0, "ctrl+d on tree must scroll the diff content, not tree")
	s.Require().Equal(cursorBefore, tree.Cursor(), "tree cursor must not move on ctrl+d")

	s.Require().NoError(s.dispatch(keymap.ViewDetailDiffTree, gocui.KeyCtrlU))
	_, oyAfterUp := content.Origin()
	s.Require().Less(oyAfterUp, oy, "ctrl+u on tree must scroll content back up")
}

func (s *MRDiffSuite) TestDiffTab_AllContentKeys_ScrollOrigin() {
	s.loadAndSelectMR()

	lines := make([]string, 80)
	for i := range lines {
		lines[i] = "+line" + fmt.Sprintf("%d", i)
	}
	s.v.Detail.DiffContent().SetFile(&models.MRDiffFile{Diff: strings.Join(lines, "\n")})
	s.Require().NoError(s.layoutTick())

	content, err := s.g.View(keymap.ViewDetailDiffContent)
	s.Require().NoError(err)

	_, err = s.g.SetCurrentView(keymap.ViewDetailDiffContent)
	s.Require().NoError(err)

	scrollKeys := []any{'j', gocui.KeyArrowDown, gocui.KeyCtrlD}
	for _, k := range scrollKeys {
		content.SetOrigin(0, 0)
		s.Require().NoError(s.dispatch(keymap.ViewDetailDiffContent, k))
		_, oy := content.Origin()
		s.Require().Greater(oy, 0, "key %v must scroll down", k)
	}

	retreatKeys := []any{'k', gocui.KeyArrowUp, gocui.KeyCtrlU}
	for _, k := range retreatKeys {
		content.SetOrigin(0, 10)
		s.Require().NoError(s.dispatch(keymap.ViewDetailDiffContent, k))
		_, oy := content.Origin()
		s.Require().Less(oy, 10, "key %v must scroll up", k)
	}
}

func (s *MRDiffSuite) TestDiffTab_LeavingTabRemovesSubPanes() {
	s.loadAndSelectMR()

	_, err := s.g.View(keymap.ViewDetailDiffTree)
	s.Require().NoError(err, "tree mounted while Diff tab active")

	s.v.Detail.SetTab(views.DetailTabOverview, nil)
	s.Require().NoError(s.layoutTick())

	_, err = s.g.View(keymap.ViewDetailDiffTree)
	s.Require().Error(err, "tree unmounted after leaving Diff tab")
	_, err = s.g.View(keymap.ViewDetailDiffContent)
	s.Require().Error(err, "content unmounted after leaving Diff tab")
}

func (s *MRDiffSuite) TestTabCycle_BracketAdvancesToDiffThenOverview() {
	s.v.Detail.SetTab(views.DetailTabOverview, nil)
	s.Require().NoError(s.layoutTick())

	_, err := s.g.SetCurrentView(keymap.ViewDetail)
	s.Require().NoError(err)

	s.Require().NoError(s.dispatch(keymap.ViewDetail, ']'))
	s.Require().NoError(s.layoutTick())

	s.Require().Equal(views.DetailTabDiff, s.v.Detail.CurrentTab())
	s.Require().Equal(keymap.ViewDetailDiffTree, s.g.CurrentView().Name(),
		"cycling into Diff auto-focuses the tree pane")

	s.Require().NoError(s.dispatch(keymap.ViewDetailDiffTree, '['))
	s.Require().NoError(s.layoutTick())

	s.Require().Equal(views.DetailTabOverview, s.v.Detail.CurrentTab())
	s.Require().Equal(keymap.ViewDetail, s.g.CurrentView().Name())
}

func (s *MRDiffSuite) TestDiffTab_TreeHighlightSurvivesTabCycleAway() {
	s.loadAndSelectMR()

	pane, err := s.g.View(keymap.ViewDetailDiffTree)
	s.Require().NoError(err)
	s.Require().True(pane.Highlight, "tree highlight on initial mount")

	// Cycle away to Overview, then back to Diff. Sub-panes unmount and
	// remount; Render must re-apply Highlight to the fresh pane.
	s.v.Detail.SetTab(views.DetailTabOverview, nil)
	s.Require().NoError(s.layoutTick())
	s.v.Detail.SetTab(views.DetailTabDiff, s.v.Repos.SelectedProject())
	s.Require().NoError(s.layoutTick())

	pane, err = s.g.View(keymap.ViewDetailDiffTree)
	s.Require().NoError(err)
	s.Require().True(pane.Highlight,
		"highlight property must be re-applied to the remounted diff tree pane")
}

//nolint:paralleltest // gocui stores tcell simulation screen in a global; parallel runs race.
func TestMRDiffSuite(t *testing.T) {
	suite.Run(t, new(MRDiffSuite))
}
