package views

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/theme"
)

// pipelineStagesNow is the clock used by the stages view when formatting the
// "updated Ns ago" fragment of the overall summary line. Tests freeze it.
var pipelineStagesNow = time.Now

const (
	pipelineStagesIndent      = "   "
	pipelineStagesLoadingHint = "Loading pipeline…"
	pipelineStagesEmptyHint   = "No pipeline for this merge request."
	pipelineStagesNoJobsHint  = "Pipeline has no jobs."
	pipelineStagesJobIndent   = pipelineStagesIndent
)

// stageRow is one rendered row of the pipeline stages widget. `job` is nil
// for stage-header rows; leaves carry the domain pointer so Enter-on-job can
// resolve the selection without a second lookup.
type stageRow struct {
	text string
	job  *models.PipelineJob
}

// PipelineStagesView renders an MR's latest pipeline as a flat list —
// `stage/<name>` headers in bold with each of the stage's jobs indented
// below. Mirrors the Python PipelineStagesView but in a single vertical
// pane because gocui has no horizontal-scroll container; navigation
// pattern matches DiffTreeView exactly (j/k skip header rows, Enter
// lands on a real job).
//
// Pointer ownership: stageRow.job is `&detail.Jobs[i]`, so SelectedJob
// shares storage with the slice handed to SetDetail. Callers MUST NOT
// mutate the slice after passing it in, and the returned pointer is
// valid only until the next SetDetail call.
//
// The cursor-highlight properties (Highlight, SelBgColor, SelFgColor)
// are re-applied whenever Render receives a pane pointer it hasn't seen
// before. Necessary because the Pipeline tab unmounts + remounts the
// stages pane every time the inline job log opens and closes — a single
// "configured once" flag would leave the remounted pane without the
// green-on-black cursor row.
type PipelineStagesView struct {
	mu          sync.Mutex
	rows        []stageRow
	detail      *models.PipelineDetail
	cursor      int
	status      string
	chromeTitle string
	chromeMeta  string
	lastPane    *gocui.View
}

// NewPipelineStages constructs an empty view showing the loading hint
// until SetDetail is called.
func NewPipelineStages() *PipelineStagesView {
	return &PipelineStagesView{status: pipelineStagesLoadingHint}
}

// SetDetail replaces the stages contents. Nil detail shows the empty
// (no-pipeline) hint; a detail with no jobs shows the no-jobs hint.
//
// Cursor position survives refreshes: before replacing rows we remember the
// job under the cursor (by ID) and, after the rebuild, seek to the same job
// if it still exists. Without this the auto-refresh tick would yank the
// cursor back to the first job every 5 seconds — deeply hostile while the
// user is inspecting a late-stage job.
func (p *PipelineStagesView) SetDetail(detail *models.PipelineDetail) {
	p.mu.Lock()
	defer p.mu.Unlock()

	prevJobID := p.selectedJobIDLocked()

	if detail == nil {
		p.rows = nil
		p.detail = nil
		p.cursor = 0
		p.status = pipelineStagesEmptyHint

		return
	}
	p.detail = detail
	p.rows = buildStageRows(detail.Jobs)
	p.cursor = cursorForJobID(p.rows, prevJobID)
	switch {
	case len(detail.Jobs) == 0:
		p.status = pipelineStagesNoJobsHint
	default:
		p.status = ""
	}
}

// selectedJobIDLocked returns the ID of the job currently under the cursor,
// or 0 if the cursor isn't on a job row. Caller holds p.mu.
func (p *PipelineStagesView) selectedJobIDLocked() int {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return 0
	}
	if job := p.rows[p.cursor].job; job != nil {
		return job.ID
	}

	return 0
}

// cursorForJobID finds the row index of the job whose ID matches prev, or
// falls back to the first job row when the job is no longer present (MR
// pushed a new pipeline, job was filtered out, etc.).
func cursorForJobID(rows []stageRow, prev int) int {
	if prev == 0 {
		return firstJobRow(rows)
	}
	for i, r := range rows {
		if r.job != nil && r.job.ID == prev {
			return i
		}
	}

	return firstJobRow(rows)
}

// SetChrome updates the title / meta pair drawn on the first line of the
// pane. Called by DetailView before each render so the right-hand "updated
// Ns ago" fragment stays current. Empty strings disable the chrome.
func (p *PipelineStagesView) SetChrome(title, meta string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.chromeTitle = title
	p.chromeMeta = meta
}

// SetTransientStatus surfaces an ephemeral message (e.g. "copied 1234 chars",
// "retrying e2e-smoke…") via the existing status slot. Cleared on the next
// SetDetail so the message lifetime is naturally bounded by the refresh
// cycle.
func (p *PipelineStagesView) SetTransientStatus(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if msg == "" {
		return
	}
	p.status = theme.FgDim + msg + theme.Reset
}

// ShowLoading resets the pane to the dim-loading hint.
func (p *PipelineStagesView) ShowLoading() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.rows = nil
	p.detail = nil
	p.cursor = 0
	p.status = pipelineStagesLoadingHint
}

// ShowError replaces content with a red error message.
func (p *PipelineStagesView) ShowError(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.rows = nil
	p.detail = nil
	p.cursor = 0
	p.status = theme.FgErr + msg + theme.Reset
}

// SelectedJob returns the job under the cursor, or nil if the cursor is
// on a stage header (guarded; should not happen after SetDetail).
func (p *PipelineStagesView) SelectedJob() *models.PipelineJob {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return nil
	}

	return p.rows[p.cursor].job
}

// Cursor reports the current row index. Exposed for tests + scroll sync.
func (p *PipelineStagesView) Cursor() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.cursor
}

// RowCount returns the total row count (headers + leaves).
func (p *PipelineStagesView) RowCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.rows)
}

// MoveCursor shifts the cursor by delta rows, skipping header rows. Returns
// true when the cursor moved.
func (p *PipelineStagesView) MoveCursor(delta int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.moveCursorLocked(delta)
}

// MoveCursorToStart puts the cursor on the first job row.
func (p *PipelineStagesView) MoveCursorToStart() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.cursor = firstJobRow(p.rows)
}

// MoveCursorToEnd puts the cursor on the last job row.
func (p *PipelineStagesView) MoveCursorToEnd() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.cursor = lastJobRow(p.rows)
}

func (p *PipelineStagesView) moveCursorLocked(delta int) bool {
	if len(p.rows) == 0 || delta == 0 {
		return false
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	remaining := delta
	if remaining < 0 {
		remaining = -remaining
	}
	newCursor := p.cursor
	for remaining > 0 {
		candidate := newCursor + step
		if candidate < 0 || candidate >= len(p.rows) {
			break
		}
		newCursor = candidate
		if p.rows[newCursor].job == nil {
			continue
		}
		remaining--
	}

	if newCursor == p.cursor || p.rows[newCursor].job == nil {
		return false
	}
	p.cursor = newCursor

	return true
}

// Render paints the stages listing into pane. Configures cursor colours on
// the first render so Highlight+SelBgColor track the selected row.
func (p *PipelineStagesView) Render(pane *gocui.View) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.lastPane != pane {
		pane.Highlight = true
		pane.SelBgColor = theme.ColorAccent
		pane.SelFgColor = theme.ColorSelectionFg
		p.lastPane = pane
	}

	innerW, _ := pane.InnerSize()
	pane.Clear()

	chromeOffset := 0
	if chrome := renderChromeLine(p.chromeTitle, p.chromeMeta, innerW); chrome != "" {
		pane.WriteString(chrome + "\n\n")
		chromeOffset = 2
	}
	if p.status != "" {
		pane.WriteString(p.status + "\n")
		placeCursor(pane, chromeOffset, chromeOffset+1)

		return
	}
	for _, row := range p.rows {
		pane.WriteString(row.text + "\n")
	}

	footer := p.renderPaneSummaryLocked(innerW)
	pane.WriteString(footer)
	// Content rows start at `chromeOffset`; the footer adds 3 visible lines
	// (blank, separator, overall). The keybind strip used to live here but
	// now ships from the global FooterView (design/wireframes/layout.js).
	const footerLines = 3
	totalLines := chromeOffset + len(p.rows) + footerLines
	placeCursor(pane, p.cursor+chromeOffset, totalLines)
}

// renderPaneSummaryLocked returns the pane's trailing decoration — blank line,
// separator, overall summary. The keybind strip has moved to the global
// FooterView (mounted by tui.manageFooter). Caller holds p.mu.
func (p *PipelineStagesView) renderPaneSummaryLocked(innerW int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(renderPipelineSeparator(innerW) + "\n")
	if p.detail != nil {
		sb.WriteString(renderOverallLine(p.detail.Pipeline, p.detail.Pipeline.TriggeredBy, pipelineStagesNow()) + "\n")
	}

	return sb.String()
}

// renderPipelineSeparator emits a dim dashed line matching the wireframe's
// summary divider. Width follows the pane's inner width (minus a single
// leading space to mirror the indent of the overall line), clamped to a
// sensible minimum so a resize to near-zero doesn't produce garbage.
func renderPipelineSeparator(innerW int) string {
	width := innerW - 1
	if width < 20 {
		width = 20
	}
	if width > 200 {
		width = 200
	}

	return " " + theme.FgDim + strings.Repeat("\u2500", width) + theme.Reset
}

// renderOverallLine paints the dim "Overall <status> · [triggered by @user]
// · commit <sha7> · <age>" line per the pipeline wireframe. Missing SHA or
// triggered-by fragments are skipped; callers pass nil/zero values safely.
func renderOverallLine(p models.Pipeline, triggeredBy *models.User, now time.Time) string {
	var sb strings.Builder
	sb.WriteByte(' ')
	sb.WriteString(theme.FgDim + "Overall " + theme.Reset)

	color := pipelineStatusColor(p.Status)
	label := pipelineStatusLabel(p.Status)
	sb.WriteString(theme.Dot(color) + " " + color + label + theme.Reset)

	if triggeredBy != nil && triggeredBy.Username != "" {
		sb.WriteString(theme.FgDim + " · triggered by " + theme.Reset)
		sb.WriteString(theme.FgAccent + "@" + triggeredBy.Username + theme.Reset)
	}

	if sha := shortSHA(p.SHA); sha != "" {
		sb.WriteString(theme.FgDim + " · commit " + theme.Reset)
		sb.WriteString(theme.FgAccent + sha + theme.Reset)
	}

	if age := theme.Relative(p.CreatedAt, now); age != "" {
		sb.WriteString(theme.FgDim + " · " + age + theme.Reset)
	}

	return sb.String()
}

func shortSHA(sha string) string {
	if len(sha) < 7 {
		return sha
	}

	return sha[:7]
}

// buildStageRows groups jobs by stage in API order (first-seen wins),
// emitting a bold header once per stage followed by the stage's jobs.
func buildStageRows(jobs []models.PipelineJob) []stageRow {
	if len(jobs) == 0 {
		return nil
	}

	rows := make([]stageRow, 0, len(jobs)+4)
	printed := make(map[string]struct{}, 4)

	for i := range jobs {
		j := jobs[i]
		if _, seen := printed[j.Stage]; !seen {
			printed[j.Stage] = struct{}{}
			rows = append(rows, stageRow{
				text: fmt.Sprintf(" %s%s%s", theme.Bold, j.Stage, theme.Reset),
			})
		}
		rows = append(rows, stageRow{
			text: pipelineStagesJobIndent + renderJobLine(&jobs[i]),
			job:  &jobs[i],
		})
	}

	return rows
}

func renderJobLine(j *models.PipelineJob) string {
	icon := pipelineJobStatusIcon(j.Status)
	duration := formatJobDuration(j.Duration)
	if duration == "" {
		return fmt.Sprintf("%s %s", icon, j.Name)
	}

	return fmt.Sprintf("%s %s %s%s%s", icon, j.Name, theme.FgDim, duration, theme.Reset)
}

// formatJobDuration mirrors Python _format_duration: nil → "", <60s → "Xs",
// ≥60s → "Ym Xs". Sub-second durations round to whole seconds.
func formatJobDuration(d *float64) string {
	if d == nil {
		return ""
	}
	seconds := int(*d)
	if seconds < 0 {
		seconds = 0
	}
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	secs := seconds % 60

	return fmt.Sprintf("%dm %ds", minutes, secs)
}

// pipelineJobStatusIcon maps a PipelineStatus to a coloured ANSI glyph per
// design/project/wireframes/pipeline.js:67. Colour is the second layer of
// information atop the glyph; both must match the design palette.
func pipelineJobStatusIcon(status models.PipelineStatus) string {
	switch status {
	case models.PipelineStatusSuccess:
		return theme.FgOK + "\u2713" + theme.Reset // ✓
	case models.PipelineStatusFailed:
		return theme.FgErr + "\u2717" + theme.Reset // ✗
	case models.PipelineStatusRunning:
		return theme.FgOK + "\u25CF" + theme.Reset // ●
	case models.PipelineStatusPending,
		models.PipelineStatusWaitingForResource,
		models.PipelineStatusPreparing,
		models.PipelineStatusCreated:
		return theme.FgDim + "\u25CB" + theme.Reset // ○
	case models.PipelineStatusSkipped:
		return theme.FgWarn + "\u2298" + theme.Reset // ⊘
	case models.PipelineStatusManual:
		return theme.FgDim + "\u25B6" + theme.Reset // ▶
	case models.PipelineStatusCanceled:
		return theme.FgErr + "\u2298" + theme.Reset // ⊘ (err-coloured; `✗` would collide with failed)
	case models.PipelineStatusScheduled:
		return theme.FgDim + "\u25B6" + theme.Reset // ▶
	default:
		return theme.FgDim + strings.ToLower(string(status)) + theme.Reset
	}
}

func firstJobRow(rows []stageRow) int {
	for i, r := range rows {
		if r.job != nil {
			return i
		}
	}

	return 0
}

func lastJobRow(rows []stageRow) int {
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].job != nil {
			return i
		}
	}

	return 0
}
