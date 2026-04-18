package views

import (
	"fmt"
	"strings"
	"sync"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/theme"
)

const (
	pipelineStagesIndent      = "  "
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
	mu       sync.Mutex
	rows     []stageRow
	cursor   int
	status   string
	lastPane *gocui.View
}

// NewPipelineStages constructs an empty view showing the loading hint
// until SetDetail is called.
func NewPipelineStages() *PipelineStagesView {
	return &PipelineStagesView{status: pipelineStagesLoadingHint}
}

// SetDetail replaces the stages contents. Nil detail shows the empty
// (no-pipeline) hint; a detail with no jobs shows the no-jobs hint.
func (p *PipelineStagesView) SetDetail(detail *models.PipelineDetail) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if detail == nil {
		p.rows = nil
		p.cursor = 0
		p.status = pipelineStagesEmptyHint

		return
	}
	p.rows = buildStageRows(detail.Jobs)
	p.cursor = firstJobRow(p.rows)
	switch {
	case len(detail.Jobs) == 0:
		p.status = pipelineStagesNoJobsHint
	default:
		p.status = ""
	}
}

// ShowLoading resets the pane to the dim-loading hint.
func (p *PipelineStagesView) ShowLoading() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.rows = nil
	p.cursor = 0
	p.status = pipelineStagesLoadingHint
}

// ShowError replaces content with a red error message.
func (p *PipelineStagesView) ShowError(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.rows = nil
	p.cursor = 0
	p.status = ansiRed + msg + ansiReset
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
		pane.SelFgColor = gocui.ColorBlack
		p.lastPane = pane
	}

	pane.Clear()
	if p.status != "" {
		pane.WriteString(p.status + "\n")
		placeCursor(pane, 0, 1)

		return
	}
	for _, row := range p.rows {
		pane.WriteString(row.text + "\n")
	}
	placeCursor(pane, p.cursor, len(p.rows))
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
				text: fmt.Sprintf("%s%s%s", ansiBold, j.Stage, ansiReset),
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

	return fmt.Sprintf("%s %s %s%s%s", icon, j.Name, ansiDim, duration, ansiReset)
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

// pipelineJobStatusIcon maps a PipelineStatus to a coloured ANSI glyph.
// Unknown statuses fall through to a dim label so the pane does not render
// a blank column for a newly-introduced upstream state.
func pipelineJobStatusIcon(status models.PipelineStatus) string {
	switch status {
	case models.PipelineStatusSuccess:
		return ansiGreen + "\u2713" + ansiReset // ✓
	case models.PipelineStatusFailed:
		return ansiRed + "\u2717" + ansiReset // ✗
	case models.PipelineStatusRunning:
		return ansiYellow + "\u25B6" + ansiReset // ▶
	case models.PipelineStatusPending,
		models.PipelineStatusWaitingForResource,
		models.PipelineStatusPreparing:
		return ansiYellow + "\u25EF" + ansiReset // ◯
	case models.PipelineStatusCreated:
		return ansiDim + "\u25EF" + ansiReset
	case models.PipelineStatusCanceled:
		return ansiDim + "\u2717" + ansiReset
	case models.PipelineStatusSkipped:
		return ansiDim + "\u2298" + ansiReset // ⊘
	case models.PipelineStatusManual:
		return ansiCyan + "\u25B6" + ansiReset
	case models.PipelineStatusScheduled:
		return ansiCyan + "\u23F2" + ansiReset // ⏲
	default:
		return ansiDim + strings.ToLower(string(status)) + ansiReset
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
