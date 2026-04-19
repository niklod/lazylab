package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/theme"
)

const (
	pipelineChromeDash = "—"
	// updatedAgoSlot is the stable cell width of the "Ns ago" / "Nm Ss ago"
	// fragment. Per-second ticks must not reshuffle the chrome line, so the
	// age text is padded with trailing spaces to this slot. 11 fits every
	// format up to "59m 59s ago"; hour-scale intervals exceed the slot but
	// are rare and shift by at most 1–2 cells.
	updatedAgoSlot = 11
)

// pipelineRefreshState captures the subset of DetailView.pipelineRefresh that
// the stages / log widgets need in order to paint the `↻ auto 5s · updated Ns
// ago` meta fragment. Copied out under DetailView.mu so the widgets never
// reach back into the parent (lock ordering).
type pipelineRefreshState struct {
	enabled     bool          // false → "paused"
	interval    time.Duration // 5s / 30s depending on pipeline status
	lastRefresh time.Time
	// pausedReason, when non-empty, overrides "paused" with
	// "paused — <reason>" (wireframe uses this for finished jobs).
	pausedReason string
}

// renderChromeLine lays out "<title>    <meta>" inside innerW columns. When
// the pane is too narrow to hold both, it falls back to concatenating them
// with a minimum two-space gutter so neither gets clipped off entirely.
// innerW ≤ 0 → compact form for headless tests.
func renderChromeLine(title, meta string, innerW int) string {
	if title == "" && meta == "" {
		return ""
	}
	if innerW <= 0 {
		return strings.TrimSpace(title + "  " + meta)
	}

	titleW := visibleWidth(title)
	metaW := visibleWidth(meta)
	available := innerW - 1 // leading space indent
	if titleW+metaW+2 > available {
		// Pane narrow: glue the two together with two spaces. The hint is
		// still readable; the alternative is silent truncation.
		return " " + title + "  " + meta
	}

	pad := available - titleW - metaW
	if pad < 2 {
		pad = 2
	}

	return " " + title + strings.Repeat(" ", pad) + meta
}

// renderRefreshIndicator paints the `↻ auto 5s · updated Ns ago` fragment that
// sits at the right end of the Pipeline / Log pane meta line. When refresh is
// disabled (user-paused) it collapses to `↻ paused` or `↻ paused — <reason>`
// per design/project/wireframes/pipeline.js:30, :43.
func renderRefreshIndicator(state pipelineRefreshState, now time.Time) string {
	arrow := theme.FgAccent + "\u21BB" + theme.Reset

	if !state.enabled {
		if state.pausedReason != "" {
			return arrow + " " + theme.FgDim + "paused — " + state.pausedReason + theme.Reset
		}

		return arrow + " " + theme.FgDim + "paused" + theme.Reset
	}

	interval := formatRefreshInterval(state.interval)
	age := padAgeSlot(formatUpdatedAgo(state.lastRefresh, now))
	meta := theme.FgDim + "auto " + theme.Reset + interval + theme.FgDim + " · updated " + age + theme.Reset

	return arrow + " " + meta
}

// padAgeSlot right-pads age to updatedAgoSlot cells so tick-induced width
// changes ("1s ago" → "10s ago" → "1m 3s ago") do not reshuffle the chrome
// line. Content longer than the slot is returned as-is rather than truncated.
func padAgeSlot(age string) string {
	w := visibleWidth(age)
	if w >= updatedAgoSlot {
		return age
	}

	return age + strings.Repeat(" ", updatedAgoSlot-w)
}

// formatUpdatedAgo renders the "Ns ago" / "Nm Ss ago" / "Nm ago" fragment in
// the refresh indicator. Unlike theme.Relative (which floors to "just now"
// for the first minute), this needs per-second resolution so the user sees
// the 1-second UI tick actually doing something between data refreshes.
func formatUpdatedAgo(t, now time.Time) string {
	if t.IsZero() {
		return "just now"
	}
	delta := now.Sub(t)
	if delta < 0 {
		delta = 0
	}
	seconds := int(delta / time.Second)
	if seconds < 1 {
		return "just now"
	}
	if seconds < 60 {
		return fmt.Sprintf("%ds ago", seconds)
	}
	minutes := seconds / 60
	remainder := seconds % 60
	if minutes < 60 && remainder > 0 {
		return fmt.Sprintf("%dm %ds ago", minutes, remainder)
	}
	if minutes < 60 {
		return fmt.Sprintf("%dm ago", minutes)
	}
	hours := minutes / 60

	return fmt.Sprintf("%dh ago", hours)
}

func formatRefreshInterval(d time.Duration) string {
	if d <= 0 {
		return theme.FgDim + "—" + theme.Reset
	}
	seconds := int(d / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}

	return fmt.Sprintf("%dm", seconds/60)
}

// pipelineStagesMeta formats the stages pane's right-hand meta:
//
//	Pipeline · #<id> · <dur>   <refresh-indicator>
//
// dur falls back to "—" when CreatedAt/UpdatedAt are absent.
func pipelineStagesMeta(p models.Pipeline, state pipelineRefreshState, now time.Time) string {
	dur := pipelineDurationForMeta(p, now)
	head := fmt.Sprintf("Pipeline · #%d · %s", p.ID, dur)
	refresh := renderRefreshIndicator(state, now)

	return theme.FgDim + head + theme.Reset + "   " + refresh
}

// pipelineDurationForMeta mirrors pipelineDurationText but also returns a
// running wall-clock delta for non-terminal pipelines so the meta updates
// tick-by-tick. "—" covers the degenerate "no timestamps" case.
func pipelineDurationForMeta(p models.Pipeline, now time.Time) string {
	if p.CreatedAt.IsZero() {
		return pipelineChromeDash
	}
	end := p.UpdatedAt
	if !p.Status.IsTerminal() || end.IsZero() || end.Before(p.CreatedAt) {
		end = now
	}
	d := end.Sub(p.CreatedAt)
	if d <= 0 {
		return pipelineChromeDash
	}
	seconds := int(d / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}

	return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
}

// jobLogMeta formats the log pane's right-hand meta line:
//
//	stage <s> · <dur> · job #<id>   <refresh-indicator>
//
// When the job has reached a terminal state, the refresh indicator is pinned
// to `paused — job finished` to match the wireframe copy.
func jobLogMeta(job *models.PipelineJob, state pipelineRefreshState, now time.Time) string {
	if job == nil {
		return ""
	}
	dur := formatJobDuration(job.Duration)
	if dur == "" {
		dur = "—"
	}

	if job.Status.IsTerminal() {
		state.enabled = false
		state.pausedReason = "job finished"
	}

	head := fmt.Sprintf("stage %s · %s · job #%d", job.Stage, dur, job.ID)
	refresh := renderRefreshIndicator(state, now)

	return theme.FgDim + head + theme.Reset + "   " + refresh
}
