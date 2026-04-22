//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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
	mrActionsCfgPath = "/cfg/config.yaml"
	mrActionsWidth   = 160
	mrActionsHeight  = 40
)

const mrActionsProjectsFixture = `[
	{"id":42,"name":"Alpha","path_with_namespace":"grp/alpha","web_url":"https://gl/grp/alpha","last_activity_at":"2026-04-10T10:00:00Z","archived":false}
]`

const mrActionsOpenedMRsFixture = `[
	{
		"id":1,"iid":10,"title":"Feature Alpha",
		"state":"opened",
		"author":{"id":1,"username":"alice","name":"A","web_url":"u"},
		"source_branch":"feat/alpha","target_branch":"main",
		"web_url":"u","created_at":"2026-04-10T14:30:00Z",
		"has_conflicts":false,"user_notes_count":0
	}
]`

const mrActionsMergedMRsFixture = `[
	{
		"id":1,"iid":10,"title":"Feature Alpha",
		"state":"merged",
		"author":{"id":1,"username":"alice","name":"A","web_url":"u"},
		"source_branch":"feat/alpha","target_branch":"main",
		"web_url":"u","created_at":"2026-04-10T14:30:00Z",
		"merged_at":"2026-04-11T14:30:00Z",
		"has_conflicts":false,"user_notes_count":0
	}
]`

// recordedRequest captures the method/path/body of a mutation call so the
// test can assert both that it fired and what it sent.
type recordedRequest struct {
	method string
	path   string
	body   map[string]any
}

type MRActionsSuite struct {
	suite.Suite

	srv     *httptest.Server
	fs      afero.Fs
	g       *gocui.Gui
	v       *views.Views
	app     *appcontext.AppContext
	manager func(*gocui.Gui) error

	mu       sync.Mutex
	recorded []recordedRequest
	mrState  atomic.Pointer[string]
}

func (s *MRActionsSuite) SetupTest() {
	initial := "opened"
	s.mrState.Store(&initial)

	s.mu.Lock()
	s.recorded = nil
	s.mu.Unlock()

	s.srv = httptest.NewServer(http.HandlerFunc(s.handleRequest))

	s.fs = afero.NewMemMapFs()
	cfg := config.Defaults()
	cfg.GitLab.URL = s.srv.URL
	cfg.GitLab.Token = "e2e-secret"
	s.Require().NoError(cfg.Save(s.fs, mrActionsCfgPath))

	client, err := gitlab.New(cfg.GitLab, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	s.app = appcontext.New(cfg, client, cache.New(cfg.Cache, s.fs), s.fs, mrActionsCfgPath)

	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: mrActionsWidth, Height: mrActionsHeight})
	s.Require().NoError(err)
	s.g = g

	s.v = views.New(g, s.app)
	s.manager = tui.NewManager(s.v)
	g.SetManagerFunc(s.manager)
	tui.SetFocusOrderProvider(s.v.FocusOrder)
	s.T().Cleanup(func() { tui.SetFocusOrderProvider(nil) })
	s.Require().NoError(tui.Bind(g, s.v.Bindings()...))
	s.Require().NoError(s.layoutTick())
}

func (s *MRActionsSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

func (s *MRActionsSuite) handleRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	state := *s.mrState.Load()
	switch {
	case strings.HasSuffix(r.URL.Path, "/projects"):
		_, _ = fmt.Fprint(w, mrActionsProjectsFixture)

		return
	case strings.Contains(r.URL.Path, "/discussions"),
		strings.Contains(r.URL.Path, "/diffs"):
		_, _ = fmt.Fprint(w, `[]`)

		return
	case strings.Contains(r.URL.Path, "/approvals"):
		_, _ = fmt.Fprint(w, `{"approved":true,"approvals_required":0,"approvals_left":0}`)

		return
	case strings.Contains(r.URL.Path, "/merge_requests/") && strings.HasSuffix(r.URL.Path, "/pipelines"):
		_, _ = fmt.Fprint(w, `[]`)

		return
	case strings.HasSuffix(r.URL.Path, "/user"):
		_, _ = fmt.Fprint(w, `{"id":99,"username":"me","name":"Me","web_url":"u"}`)

		return
	}

	// Mutation endpoints. Order matters: /merge is a sub-path of /merge_requests/:iid.
	if r.Method == http.MethodPut {
		body := s.decodeBody(r)
		s.record(r, body)

		if strings.HasSuffix(r.URL.Path, "/merge") {
			merged := "merged"
			s.mrState.Store(&merged)
			s.writeMRResponse(w, "merged")

			return
		}
		if ev, _ := body["state_event"].(string); ev == "close" {
			closed := "closed"
			s.mrState.Store(&closed)
			s.writeMRResponse(w, "closed")

			return
		}
	}

	if strings.Contains(r.URL.Path, "/merge_requests") {
		switch state {
		case "closed":
			_, _ = fmt.Fprint(w, `[]`)

			return
		case "merged":
			_, _ = fmt.Fprint(w, mrActionsMergedMRsFixture)

			return
		}
		_, _ = fmt.Fprint(w, mrActionsOpenedMRsFixture)

		return
	}

	http.NotFound(w, r)
}

func (s *MRActionsSuite) writeMRResponse(w http.ResponseWriter, state string) {
	_, _ = fmt.Fprintf(w, `{"id":1,"iid":10,"title":"Feature Alpha","state":%q,"author":{"id":1,"username":"alice","name":"A","web_url":"u"},"source_branch":"feat/alpha","target_branch":"main","web_url":"u"}`, state)
}

func (s *MRActionsSuite) decodeBody(r *http.Request) map[string]any {
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	_ = json.Unmarshal(body, &out)

	return out
}

func (s *MRActionsSuite) record(r *http.Request, body map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.recorded = append(s.recorded, recordedRequest{
		method: r.Method,
		path:   r.URL.Path,
		body:   body,
	})
}

func (s *MRActionsSuite) mutations() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]recordedRequest, len(s.recorded))
	copy(out, s.recorded)

	return out
}

func (s *MRActionsSuite) layoutTick() error { return s.manager(s.g) }

func (s *MRActionsSuite) dispatch(view string, key any) error {
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

func (s *MRActionsSuite) modalBuffer() string {
	pane, err := s.g.View(keymap.ViewMRActionsModal)
	if err != nil {
		return ""
	}

	return pane.Buffer()
}

func (s *MRActionsSuite) footerBuffer() string {
	pane, err := s.g.View(keymap.ViewFooter)
	if err != nil {
		return ""
	}

	return pane.Buffer()
}

// stripANSI drops SGR escape sequences so assertions don't need to know the
// exact theme bytes around the content they're checking for.
func stripANSI(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b || i+1 >= len(s) || s[i+1] != '[' {
			out.WriteByte(s[i])

			continue
		}
		// Skip through the 'm' terminator.
		j := i + 2
		for j < len(s) && s[j] != 'm' {
			j++
		}
		i = j
	}

	return out.String()
}

func (s *MRActionsSuite) loadProjectAndMRs() {
	s.Require().NoError(s.v.Repos.LoadSync(context.Background()))
	s.Require().NoError(s.layoutTick())

	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	s.Require().NoError(s.v.MRs.SetProjectSync(context.Background(), project))
	s.Require().NoError(s.layoutTick())
}

// confirmInlineClose drives the close path synchronously. Reuses the real
// gitlab client (which hits the test server) but calls its methods from the
// test goroutine so the layout tick observes completed state without timing
// coupling. Mirrors Views.runCloseAction's mutation-then-finish sequence.
func (s *MRActionsSuite) confirmInlineClose() {
	modal := s.v.ActionsModal
	snap := modal.Snapshot()
	s.Require().True(snap.Active)

	project := s.v.MRs.CurrentProject()
	s.Require().NotNil(project)

	modal.SetBusy(true)
	updated, err := s.app.GitLab.CloseMergeRequest(context.Background(), project.ID, snap.MR.IID, project.PathWithNamespace)
	if err != nil {
		modal.SetErr(err.Error())

		return
	}
	modal.Close()
	s.Require().NoError(s.v.MRs.SetProjectSync(context.Background(), project))
	if updated != nil {
		s.v.Detail.SetMR(project, updated)
	}
	s.Require().NoError(s.layoutTick())
}

func (s *MRActionsSuite) confirmInlineMerge() {
	modal := s.v.ActionsModal
	snap := modal.Snapshot()
	s.Require().True(snap.Active)

	project := s.v.MRs.CurrentProject()
	s.Require().NotNil(project)

	modal.SetBusy(true)
	updated, err := s.app.GitLab.AcceptMergeRequest(context.Background(), project.ID, snap.MR.IID, project.PathWithNamespace, gitlab.AcceptOptions{
		Squash:                   snap.Squash,
		ShouldRemoveSourceBranch: snap.DeleteBranch,
	})
	if err != nil {
		modal.SetErr(err.Error())

		return
	}
	modal.Close()
	s.Require().NoError(s.v.MRs.SetProjectSync(context.Background(), project))
	if updated != nil {
		s.v.Detail.SetMR(project, updated)
	}
	s.Require().NoError(s.layoutTick())
}

func (s *MRActionsSuite) TestCloseConfirm_SendsStateEventAndRefreshesList() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'x'))
	s.Require().NoError(s.layoutTick())

	s.Require().True(s.v.ActionsModal.IsActive(), "x on opened MR opens the modal")
	s.Require().Contains(s.modalBuffer(), "Feature Alpha")

	s.confirmInlineClose()

	muts := s.mutations()
	s.Require().Len(muts, 1)
	s.Require().Equal(http.MethodPut, muts[0].method)
	s.Require().True(strings.HasSuffix(muts[0].path, "/merge_requests/10"), "close uses Update endpoint, not /merge")
	s.Require().Equal("close", muts[0].body["state_event"])

	s.Require().False(s.v.ActionsModal.IsActive(), "success closes the modal")
	_, total := s.v.MRs.CursorInfo()
	s.Require().Zero(total, "refetch drops the closed MR from the opened filter")
}

func (s *MRActionsSuite) TestCloseCancel_NoMutation() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'x'))
	s.Require().NoError(s.layoutTick())
	s.Require().True(s.v.ActionsModal.IsActive())

	s.Require().NoError(s.dispatch(keymap.ViewMRActionsModal, gocui.KeyEsc))
	s.Require().NoError(s.layoutTick())

	s.Require().False(s.v.ActionsModal.IsActive())
	s.Require().Empty(s.mutations())
}

func (s *MRActionsSuite) TestCloseOnMergedMR_OpensGuardedModal() {
	merged := "merged"
	s.mrState.Store(&merged)

	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'x'))
	s.Require().NoError(s.layoutTick())

	snap := s.v.ActionsModal.Snapshot()
	s.Require().True(snap.Active, "guard must open the modal locked instead of skipping it")
	s.Require().True(snap.Locked)
	s.Require().Contains(snap.ErrMsg, "Cannot close")
	s.Require().Contains(snap.ErrMsg, "!10")
	s.Require().Empty(s.mutations(), "guard must not reach the API")

	buf := stripANSI(s.modalBuffer())
	s.Require().Contains(buf, "Cannot close", "guard reason must render inside the modal body")
}

func (s *MRActionsSuite) TestFooter_CloseModalActive_ShowsConfirmCancelOnly() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'x'))
	s.Require().NoError(s.layoutTick())

	footer := s.footerBuffer()
	s.Require().Contains(footer, "confirm")
	s.Require().Contains(footer, "cancel")
	s.Require().NotContains(footer, "delete-branch", "close modal must not show merge-specific hints")
	s.Require().NotContains(footer, "squash")
}

func (s *MRActionsSuite) TestFooter_SearchActive_ShowsSearchHints() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, '/'))
	s.Require().NoError(s.layoutTick())

	footer := s.footerBuffer()
	s.Require().Contains(footer, "apply")
	s.Require().Contains(footer, "cancel")
	s.Require().NotContains(footer, "merge", "search input must override the MRs-pane hint set")
}

func (s *MRActionsSuite) TestFooter_MergeModalActive_ShowsModalHints() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'M'))
	s.Require().NoError(s.layoutTick())

	footer := s.footerBuffer()
	s.Require().Contains(footer, "confirm")
	s.Require().Contains(footer, "cancel")
	s.Require().Contains(footer, "delete-branch")
	s.Require().Contains(footer, "squash")
}

func (s *MRActionsSuite) TestFooter_MetaLineShowsMRAndLastSync() {
	s.loadProjectAndMRs()
	s.Require().NoError(s.layoutTick())

	footer := s.footerBuffer()
	s.Require().Contains(footer, "lazylab")
	s.Require().Contains(footer, "grp/alpha")
	s.Require().Contains(footer, "!10")
	s.Require().Contains(footer, "last sync")
}

func (s *MRActionsSuite) TestMergeConfirm_DefaultOptionsSendDeleteBranchOnly() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'M'))
	s.Require().NoError(s.layoutTick())
	s.Require().True(s.v.ActionsModal.IsActive())
	s.Require().Equal(views.ModalMerge, s.v.ActionsModal.Kind())
	s.Require().True(s.v.ActionsModal.DeleteBranch(), "delete-branch defaults on")
	s.Require().False(s.v.ActionsModal.Squash())

	s.confirmInlineMerge()

	muts := s.mutations()
	s.Require().Len(muts, 1)
	s.Require().Contains(muts[0].path, "/merge_requests/10/merge")
	s.Require().Equal(true, muts[0].body["should_remove_source_branch"])
	s.Require().Equal(false, muts[0].body["squash"])
	s.Require().False(s.v.ActionsModal.IsActive(), "merge success closes the modal")
	// Test handler returns the merged MR fixture unconditionally (it does
	// not honour ?state=), so we only assert the mutation succeeded and the
	// modal closed — the "MR disappears from opened filter" contract is
	// exercised in TestCloseConfirm_SendsStateEventAndRefreshesList where
	// the handler returns an empty list for state=closed.
}

func (s *MRActionsSuite) TestMergeToggleSquash_SendsBothFlags() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'M'))
	s.Require().NoError(s.dispatch(keymap.ViewMRActionsModal, 's'))
	s.Require().NoError(s.layoutTick())
	s.Require().True(s.v.ActionsModal.Squash())

	s.confirmInlineMerge()

	muts := s.mutations()
	s.Require().Len(muts, 1)
	s.Require().Equal(true, muts[0].body["squash"])
	s.Require().Equal(true, muts[0].body["should_remove_source_branch"])
}

func (s *MRActionsSuite) TestMergeToggleDeleteBranch_SendsDeleteBranchOff() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'M'))
	s.Require().NoError(s.dispatch(keymap.ViewMRActionsModal, 'd'))
	s.Require().NoError(s.layoutTick())
	s.Require().False(s.v.ActionsModal.DeleteBranch(), "toggling from default-on flips off")

	s.confirmInlineMerge()

	muts := s.mutations()
	s.Require().Len(muts, 1)
	s.Require().Equal(false, muts[0].body["should_remove_source_branch"])
	s.Require().Equal(false, muts[0].body["squash"])
}

func (s *MRActionsSuite) TestMergeOnMergedMR_OpensGuardedModal() {
	merged := "merged"
	s.mrState.Store(&merged)
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'M'))
	s.Require().NoError(s.layoutTick())

	snap := s.v.ActionsModal.Snapshot()
	s.Require().True(snap.Active)
	s.Require().True(snap.Locked)
	s.Require().Contains(snap.ErrMsg, "Cannot merge")
	s.Require().Empty(s.mutations())
}

func (s *MRActionsSuite) TestEscWhileBusy_IgnoredByCancelGuard() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'x'))
	s.Require().NoError(s.layoutTick())
	s.Require().True(s.v.ActionsModal.IsActive())

	s.v.ActionsModal.SetBusy(true)
	s.Require().NoError(s.dispatch(keymap.ViewMRActionsModal, gocui.KeyEsc))

	s.Require().True(s.v.ActionsModal.IsActive(), "Esc is ignored while the modal is busy")
	s.Require().True(s.v.ActionsModal.Busy())
}

func (s *MRActionsSuite) TestEnterWhileBusy_DoubleConfirmGuard() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'x'))
	s.Require().NoError(s.layoutTick())

	s.v.ActionsModal.SetBusy(true)
	s.Require().NoError(s.dispatch(keymap.ViewMRActionsModal, gocui.KeyEnter))

	s.Require().Empty(s.mutations(), "Enter while busy must not fire a second mutation")
}

func (s *MRActionsSuite) selectMRIntoDetail() {
	_, err := s.g.SetCurrentView(keymap.ViewMRs)
	s.Require().NoError(err)
	s.Require().NoError(s.dispatch(keymap.ViewMRs, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())
}

func (s *MRActionsSuite) TestCloseFromOverviewPane_OpensModalWithCorrectMR() {
	s.loadProjectAndMRs()
	s.selectMRIntoDetail()

	s.Require().NoError(s.dispatch(keymap.ViewDetail, 'x'))
	s.Require().NoError(s.layoutTick())

	s.Require().True(s.v.ActionsModal.IsActive(), "x from Overview must open the modal")
	s.Require().Equal(views.ModalClose, s.v.ActionsModal.Kind())
	s.Require().Contains(s.modalBuffer(), "Feature Alpha")
	s.Require().Empty(s.mutations(), "opening the modal must not mutate")
}

func (s *MRActionsSuite) TestMergeFromPipelineTab_OpensModalWithBranches() {
	s.loadProjectAndMRs()
	s.selectMRIntoDetail()
	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	s.Require().NoError(s.v.Detail.SetTabSync(context.Background(), views.DetailTabPipeline, project))
	s.Require().NoError(s.layoutTick())

	s.Require().NoError(s.dispatch(keymap.ViewDetailPipelineStages, 'M'))
	s.Require().NoError(s.layoutTick())

	s.Require().True(s.v.ActionsModal.IsActive(), "M from Pipeline must open the modal")
	s.Require().Equal(views.ModalMerge, s.v.ActionsModal.Kind())
	buf := s.modalBuffer()
	s.Require().Contains(buf, "feat/alpha", "merge modal must render source branch")
	s.Require().Contains(buf, "main", "merge modal must render target branch")
}

func (s *MRActionsSuite) TestMergeFromConversationTab_OpensModal() {
	s.loadProjectAndMRs()
	s.selectMRIntoDetail()
	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	s.Require().NoError(s.v.Detail.SetTabSync(context.Background(), views.DetailTabConversation, project))
	s.Require().NoError(s.layoutTick())

	s.Require().NoError(s.dispatch(keymap.ViewDetailConversation, 'M'))
	s.Require().NoError(s.layoutTick())

	s.Require().True(s.v.ActionsModal.IsActive(), "M from Conversation must open the modal")
	s.Require().Equal(views.ModalMerge, s.v.ActionsModal.Kind())
}

func (s *MRActionsSuite) TestCloseFromDetailPane_MergedMR_OpensGuardedModal() {
	merged := "merged"
	s.mrState.Store(&merged)

	s.loadProjectAndMRs()
	s.selectMRIntoDetail()

	s.Require().NoError(s.dispatch(keymap.ViewDetail, 'x'))
	s.Require().NoError(s.layoutTick())

	snap := s.v.ActionsModal.Snapshot()
	s.Require().True(snap.Active, "guard opens a locked modal so the user sees the reason")
	s.Require().True(snap.Locked)
	s.Require().Contains(snap.ErrMsg, "Cannot close")
	s.Require().Contains(snap.ErrMsg, "!10")
	s.Require().Empty(s.mutations())
}

func (s *MRActionsSuite) TestCloseFromPipelinePane_ConfirmMutatesAndClosesModal() {
	s.loadProjectAndMRs()
	s.selectMRIntoDetail()
	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	s.Require().NoError(s.v.Detail.SetTabSync(context.Background(), views.DetailTabPipeline, project))
	s.Require().NoError(s.layoutTick())

	s.Require().NoError(s.dispatch(keymap.ViewDetailPipelineStages, 'x'))
	s.Require().NoError(s.layoutTick())
	s.Require().True(s.v.ActionsModal.IsActive())

	s.confirmInlineClose()

	muts := s.mutations()
	s.Require().Len(muts, 1)
	s.Require().Equal(http.MethodPut, muts[0].method)
	s.Require().Equal("close", muts[0].body["state_event"])
	s.Require().False(s.v.ActionsModal.IsActive())
	_, total := s.v.MRs.CursorInfo()
	s.Require().Zero(total, "closed MR leaves the opened-filter list")
}

func (s *MRActionsSuite) TestMutationError_KeepsModalWithErrMsg() {
	s.loadProjectAndMRs()

	s.srv.Close()
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = fmt.Fprint(w, `{"message":"Branch cannot be merged"}`)
	}))
	newClient, err := gitlab.New(config.GitLabConfig{URL: s.srv.URL, Token: "e2e-secret"}, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	s.app.GitLab = newClient

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'M'))
	s.Require().NoError(s.layoutTick())
	s.Require().True(s.v.ActionsModal.IsActive())

	s.confirmInlineMerge()

	s.Require().True(s.v.ActionsModal.IsActive(), "error keeps modal open")
	s.Require().NotEmpty(s.v.ActionsModal.ErrMsg())
}

func (s *MRActionsSuite) TestMutationError_WrapsLongErrorWithinModal() {
	s.loadProjectAndMRs()

	s.srv.Close()
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		long := strings.Repeat("the merge request is not in a mergeable state yet ", 10)
		_, _ = fmt.Fprintf(w, `{"message":%q}`, long)
	}))
	newClient, err := gitlab.New(config.GitLabConfig{URL: s.srv.URL, Token: "e2e-secret"}, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	s.app.GitLab = newClient

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'M'))
	s.Require().NoError(s.layoutTick())

	s.confirmInlineMerge()
	s.Require().NoError(s.layoutTick())

	s.Require().True(s.v.ActionsModal.IsActive(), "error keeps modal open")
	snap := s.v.ActionsModal.Snapshot()
	s.Require().GreaterOrEqual(len(snap.ErrLines), 2, "long upstream error must wrap into multiple lines")

	// Every wrapped err line must fit the inner width — otherwise the pane
	// frame is crossed just like the bug we're fixing.
	for _, line := range snap.ErrLines {
		s.Require().LessOrEqualf(
			len([]rune(line)), views.ModalWidth-4,
			"wrapped err line %q exceeds inner width", line,
		)
	}
}

func (s *MRActionsSuite) TestCloseModal_ShowsActionButtons() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'x'))
	s.Require().NoError(s.layoutTick())

	buf := stripANSI(s.modalBuffer())
	s.Require().Contains(buf, "[ Close (Enter) ]", "close modal must render its primary button")
	s.Require().Contains(buf, "[ Cancel (Esc) ]")
}

func (s *MRActionsSuite) TestMergeModal_ShowsActionButtons() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'M'))
	s.Require().NoError(s.layoutTick())

	buf := stripANSI(s.modalBuffer())
	s.Require().Contains(buf, "[ Merge (Enter) ]", "merge modal must render its primary button")
	s.Require().Contains(buf, "[ Cancel (Esc) ]")
}

func (s *MRActionsSuite) TestGuardedModal_EnterIsNoOp() {
	merged := "merged"
	s.mrState.Store(&merged)
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'x'))
	s.Require().NoError(s.layoutTick())
	s.Require().True(s.v.ActionsModal.Locked())

	s.Require().NoError(s.dispatch(keymap.ViewMRActionsModal, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	s.Require().True(s.v.ActionsModal.IsActive(), "Enter on a locked guard modal must keep it open")
	s.Require().Empty(s.mutations(), "locked modal must never fire a mutation")
}

func (s *MRActionsSuite) TestGuardedModal_EscClosesModal() {
	merged := "merged"
	s.mrState.Store(&merged)
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'x'))
	s.Require().NoError(s.layoutTick())
	s.Require().True(s.v.ActionsModal.Locked())

	s.Require().NoError(s.dispatch(keymap.ViewMRActionsModal, gocui.KeyEsc))
	s.Require().NoError(s.layoutTick())

	s.Require().False(s.v.ActionsModal.IsActive(), "Esc must dismiss a guarded modal")
}

func TestMRActionsSuite(t *testing.T) {
	suite.Run(t, new(MRActionsSuite))
}
