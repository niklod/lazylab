package views

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/models"
)

const (
	detailStatusEmpty = "Select a merge request."
	detailDateFormat  = "2006-01-02 15:04"
	detailBranchesSep = " \u2192 "

	// ANSI SGR escapes; gocui was built with OutputTrue (see internal/tui/app.go)
	// and forwards escape sequences in the pane buffer to the terminal.
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"

	iconOK   = "\u2713" // ✓
	iconBad  = "\u2717" // ✗
	iconWarn = "\u26A0" // ⚠

	detailNoConflicts  = ansiGreen + iconOK + " No conflicts" + ansiReset
	detailHasConflicts = ansiRed + iconBad + " Has conflicts" + ansiReset
)

// DetailView renders the Details pane for the selected merge request.
// Phase G4 first sub-task: overview only (title, author, date, state,
// branches, conflicts, comment count + resolved-thread breakdown).
// Subsequent G4 sub-tasks introduce Diff/Conversation/Pipeline tabs — when
// the second tab lands, this view grows a tab bar + per-tab content dispatcher.
//
// cached holds the rendered overview string and is invalidated on SetMR or
// when discussion stats land. Mirrors MRsView.bannerLine: Render is called
// on every layout tick and the content only changes when the MR or its
// stats change, so formatting once per update instead of per tick drops a
// strings.Builder allocation from the hot path.
type DetailView struct {
	g   *gocui.Gui
	app *appcontext.AppContext

	mu       sync.Mutex
	mr       *models.MergeRequest
	stats    *models.DiscussionStats
	cached   string
	statsSeq uint64
}

func NewDetail(g *gocui.Gui, app *appcontext.AppContext) *DetailView {
	return &DetailView{g: g, app: app}
}

// SetMR updates the MR driving the pane and kicks off an async discussion
// stats fetch. No fetch happens if project is nil or the view has no
// GitLab client (tests that instantiate DetailView without an app skip it).
func (d *DetailView) SetMR(project *models.Project, mr *models.MergeRequest) {
	seq, projectID, iid := d.commitMR(project, mr)
	if projectID == 0 || d.app == nil || d.app.GitLab == nil || d.g == nil {
		return
	}
	go d.fetchStats(context.Background(), seq, projectID, iid)
}

// SetMRSync applies an MR and fetches stats inline. Test-only entry point
// that mirrors MRsView.SetProjectSync so tests running without MainLoop
// observe both fields deterministically.
func (d *DetailView) SetMRSync(ctx context.Context, project *models.Project, mr *models.MergeRequest) error {
	seq, projectID, iid := d.commitMR(project, mr)
	if projectID == 0 || d.app == nil || d.app.GitLab == nil {
		return nil
	}
	stats, err := d.app.GitLab.GetMRDiscussionStats(ctx, projectID, iid)
	d.applyStats(seq, stats, err)
	if err != nil {
		return fmt.Errorf("detail: fetch discussion stats: %w", err)
	}

	return nil
}

// CurrentMR returns the MR currently displayed, or nil.
func (d *DetailView) CurrentMR() *models.MergeRequest {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.mr
}

// Stats returns the most recent DiscussionStats, or nil if none loaded.
func (d *DetailView) Stats() *models.DiscussionStats {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.stats
}

// Render paints the detail pane. Must be called from the layout callback.
func (d *DetailView) Render(pane *gocui.View) {
	d.mu.Lock()
	defer d.mu.Unlock()

	pane.Clear()

	if d.mr == nil {
		pane.WriteString(detailStatusEmpty + "\n")

		return
	}

	pane.WriteString(d.cached)
}

func (d *DetailView) commitMR(project *models.Project, mr *models.MergeRequest) (seq uint64, projectID, iid int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.mr = mr
	d.stats = nil
	d.statsSeq++
	if mr == nil {
		d.cached = ""

		return d.statsSeq, 0, 0
	}
	d.cached = renderOverview(mr, nil)

	pid := 0
	if project != nil {
		pid = project.ID
	}

	return d.statsSeq, pid, mr.IID
}

func (d *DetailView) fetchStats(ctx context.Context, seq uint64, projectID, iid int) {
	stats, err := d.app.GitLab.GetMRDiscussionStats(ctx, projectID, iid)
	d.g.Update(func(_ *gocui.Gui) error {
		d.applyStats(seq, stats, err)

		return nil
	})
}

func (d *DetailView) applyStats(seq uint64, stats *models.DiscussionStats, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if seq != d.statsSeq || d.mr == nil {
		return
	}
	if err != nil {
		return
	}
	d.stats = stats
	d.cached = renderOverview(d.mr, stats)
}

func renderOverview(mr *models.MergeRequest, stats *models.DiscussionStats) string {
	var sb strings.Builder
	sb.Grow(256)

	fmt.Fprintf(&sb, "!%d %s\n\n", mr.IID, mr.Title)
	fmt.Fprintf(&sb, "Author:   @%s\n", mr.Author.Username)
	fmt.Fprintf(&sb, "Created:  %s\n", mr.CreatedAt.Format(detailDateFormat))
	fmt.Fprintf(&sb, "Status:   %s %s\n", mrStateLetter(mr.State), mr.State)
	fmt.Fprintf(&sb, "Branches: %s%s%s\n", mr.SourceBranch, detailBranchesSep, mr.TargetBranch)
	fmt.Fprintf(&sb, "Conflicts: %s\n", conflictText(mr.HasConflicts))
	fmt.Fprintf(&sb, "Comments: %s\n", commentsText(mr.UserNotesCount, stats))

	return sb.String()
}

// commentsText formats the comment count, appending a resolved-thread
// breakdown when stats carry any resolvable discussions. Mirrors Python
// `_comments_text` — `N` on its own when nothing is resolvable,
// `N ✓ (X/X resolved)` in green when every resolvable thread is resolved,
// `N ⚠ (X/Y resolved)` in yellow when some remain unresolved.
func commentsText(notesCount int, stats *models.DiscussionStats) string {
	if stats == nil || stats.TotalResolvable == 0 {
		return fmt.Sprintf("%d", notesCount)
	}

	color, icon := ansiYellow, iconWarn
	if stats.Resolved == stats.TotalResolvable {
		color, icon = ansiGreen, iconOK
	}

	return fmt.Sprintf("%d %s%s (%d/%d resolved)%s",
		notesCount, color, icon, stats.Resolved, stats.TotalResolvable, ansiReset,
	)
}

func conflictText(hasConflicts bool) string {
	if hasConflicts {
		return detailHasConflicts
	}

	return detailNoConflicts
}
