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
	jobLogLoadingHint = "Loading job trace…"
	jobLogEmptyHint   = "Press Enter on a job to view its trace."
	jobLogEmptyTrace  = "(trace is empty)"
)

// JobLogView renders a CI job's trace inline inside the Pipeline tab.
//
// Traces are attacker-influenced: anyone who can push to a branch controls
// what the CI runner prints. Raw passthrough to a gocui pane built with
// OutputTrue would let a malicious job emit CSI cursor-movement, screen-
// clear, or OSC hyperlink sequences that corrupt the surrounding TUI
// chrome and mislead the user. `sanitizeTraceBody` therefore keeps SGR
// (colour / bold / reset) — the only escape family a log actually needs —
// and strips everything else before the body reaches the pane.
type JobLogView struct {
	mu          sync.Mutex
	job         *models.PipelineJob
	body        string
	bodyLines   int
	status      string
	chromeTitle string
	chromeMeta  string
	footer      string
}

// NewJobLog constructs a view that shows the empty-state hint until
// SetJob / ShowLoading / ShowError is called.
func NewJobLog() *JobLogView {
	return &JobLogView{status: jobLogEmptyHint}
}

// SetJob stores the sanitized trace. The job header is surfaced via the pane
// chrome (SetChrome) rather than prepended to the body — this keeps the
// scrollable content free of a line that would otherwise be dragged around
// by every j/k. The footer ("end of log · N lines · press y to copy · Esc to
// close") is assembled here so its line count reflects the final body after
// sanitize+trim.
func (l *JobLogView) SetJob(job *models.PipelineJob, trace string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if job == nil {
		l.job = nil
		l.body = ""
		l.bodyLines = 0
		l.footer = ""
		l.status = jobLogEmptyHint

		return
	}

	l.job = job
	sanitized := sanitizeTraceBody(trace)
	trimmed := strings.TrimRight(sanitized, "\n\t ")
	if trimmed == "" {
		trimmed = theme.FgDim + jobLogEmptyTrace + theme.Reset
	}

	l.body = trimmed
	l.bodyLines = strings.Count(trimmed, "\n") + 1
	l.footer = renderJobLogFooter(l.bodyLines)
	l.status = ""
}

// ShowLoading displays a placeholder while the trace is being fetched.
func (l *JobLogView) ShowLoading() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.job = nil
	l.body = ""
	l.bodyLines = 0
	l.footer = ""
	l.status = jobLogLoadingHint
}

// ShowError replaces the log with a red error message.
func (l *JobLogView) ShowError(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.job = nil
	l.body = ""
	l.bodyLines = 0
	l.footer = ""
	l.status = theme.FgErr + msg + theme.Reset
}

// SetChrome updates the title / meta pair drawn on the first line of the
// pane. Meta is right-aligned per design/project/wireframes/pipeline.js:43.
func (l *JobLogView) SetChrome(title, meta string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.chromeTitle = title
	l.chromeMeta = meta
}

// CopyBody returns the sanitized trace stripped of SGR escapes so the text
// lands in the user's clipboard as plain readable log output. The job header
// is omitted (pane chrome already shows it); callers can prepend their own
// label if the clipboard target needs context.
func (l *JobLogView) CopyBody() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	body := l.body
	if body == "" {
		return ""
	}
	header := strings.Index(body, "\n\n")
	if header > 0 {
		body = body[header+2:]
	}

	return stripSGR(body)
}

// CurrentJob returns the job currently displayed, or nil.
func (l *JobLogView) CurrentJob() *models.PipelineJob {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.job
}

// Render paints the trace into pane. Locks before touching state.
func (l *JobLogView) Render(pane *gocui.View) {
	l.mu.Lock()
	defer l.mu.Unlock()

	innerW, _ := pane.InnerSize()
	pane.Clear()
	pane.Wrap = false
	if chrome := renderChromeLine(l.chromeTitle, l.chromeMeta, innerW); chrome != "" {
		pane.WriteString(chrome + "\n\n")
	}
	if l.status != "" {
		pane.WriteString(l.status + "\n")

		return
	}
	pane.WriteString(l.body)
	if l.footer != "" {
		pane.WriteString("\n\n" + l.footer)
	}
}

// ScrollBy shifts the pane origin by delta rows, clamped to content extent.
// Half-page handlers pass ±innerH/2; single-line j/k pass ±1.
func (l *JobLogView) ScrollBy(pane *gocui.View, delta int) {
	if pane == nil || delta == 0 {
		return
	}
	_, innerH := pane.InnerSize()
	if innerH <= 0 {
		return
	}
	_, oy := pane.Origin()

	totalLines := l.lineCount()

	target := oy + delta
	if maxOY := totalLines - innerH; target > maxOY {
		target = maxOY
	}
	if target < 0 {
		target = 0
	}
	pane.SetOrigin(0, target)
}

// ScrollToTop resets the pane origin. Called when SetJob replaces the
// currently-shown trace so the new log shows from the first line.
func (l *JobLogView) ScrollToTop(pane *gocui.View) {
	if pane == nil {
		return
	}
	pane.SetOrigin(0, 0)
}

// ScrollToBottom jumps the pane origin to the last visible page of the
// trace. Clamps to zero when the content fits inside the viewport so G
// on a short log is a safe no-op rather than scrolling past the end.
func (l *JobLogView) ScrollToBottom(pane *gocui.View) {
	if pane == nil {
		return
	}
	_, innerH := pane.InnerSize()
	if innerH <= 0 {
		return
	}
	l.mu.Lock()
	totalLines := l.lineCount()
	l.mu.Unlock()

	oy := totalLines - innerH
	if oy < 0 {
		oy = 0
	}
	pane.SetOrigin(0, oy)
}

func (l *JobLogView) lineCount() int {
	if l.body == "" {
		if l.status == "" {
			return 0
		}

		return 1
	}

	return l.bodyLines
}

// sanitizeTraceBody filters untrusted terminal escape sequences and control
// bytes from a CI job trace before it reaches the pane.
//
// Allowed: printable UTF-8, newline, tab, and SGR escapes (`ESC[...m`) —
// the colour/bold/reset family GitLab uses. Stripped: all other CSI
// sequences (cursor move, screen clear, etc.), OSC sequences (including
// hyperlinks and title-set), bare ESC, NUL, BS. CR and CRLF collapse to
// a single LF so ScrollBy's line count matches what the terminal paints.
func sanitizeTraceBody(raw string) string {
	if raw == "" {
		return ""
	}

	var sb strings.Builder
	sb.Grow(len(raw))

	for i := 0; i < len(raw); {
		c := raw[i]
		switch c {
		case 0x1B: // ESC — start of a terminal control sequence
			if n := consumeEscape(raw, i); n > 0 {
				// consumeEscape returns 0 when the sequence is a safe
				// SGR to pass through verbatim; in that case fall into
				// the default write. Non-zero = advance past the stripped
				// bytes.
				i = n

				continue
			}
			// Safe SGR: copy up to and including the terminating `m`.
			for j := i; j < len(raw); j++ {
				if raw[j] == 'm' {
					sb.WriteString(raw[i : j+1])
					i = j + 1

					break
				}
				if j == len(raw)-1 {
					i = len(raw) // unterminated — drop the rest
				}
			}
		case '\r':
			// Collapse CR and CRLF to LF. Bare CR would let a malicious
			// job overwrite the previous visible line.
			if i+1 < len(raw) && raw[i+1] == '\n' {
				sb.WriteByte('\n')
				i += 2

				continue
			}
			sb.WriteByte('\n')
			i++
		case 0x00, 0x08:
			// NUL and BS can overdraw content; drop outright.
			i++
		default:
			sb.WriteByte(c)
			i++
		}
	}

	return sb.String()
}

// consumeEscape inspects an ESC sequence starting at raw[i] and decides
// whether to keep it verbatim (return 0, caller copies) or drop the whole
// sequence (return new index past the sequence).
//
// Only CSI `\x1b[...m` (SGR) is kept; every other CSI class, every OSC,
// and a bare ESC are treated as dangerous and stripped.
func consumeEscape(raw string, i int) int {
	if i+1 >= len(raw) {
		return i + 1 // lone trailing ESC
	}
	next := raw[i+1]

	// CSI: ESC [ <params> <final>
	if next == '[' {
		j := i + 2
		for j < len(raw) {
			b := raw[j]
			// Final byte range per ECMA-48 is 0x40..0x7e.
			if b >= 0x40 && b <= 0x7E {
				if b == 'm' {
					// SGR — safe, signal the caller to copy verbatim.
					return 0
				}

				return j + 1
			}
			j++
		}

		return len(raw) // unterminated CSI
	}

	// OSC: ESC ] <payload> (BEL | ESC \). Drop wholesale.
	if next == ']' {
		j := i + 2
		for j < len(raw) {
			if raw[j] == 0x07 { // BEL
				return j + 1
			}
			if raw[j] == 0x1B && j+1 < len(raw) && raw[j+1] == '\\' { // ST
				return j + 2
			}
			j++
		}

		return len(raw) // unterminated OSC
	}

	// Any other ESC <x> two-byte sequence: drop the pair.
	return i + 2
}

// renderJobLogFooter builds the dim "(end of log · N lines · press y to
// copy · Esc to close)" line shown beneath a trace per
// design/project/wireframes/pipeline.js:57.
func renderJobLogFooter(lineCount int) string {
	noun := "lines"
	if lineCount == 1 {
		noun = "line"
	}

	return fmt.Sprintf("%s(end of log · %d %s · press y to copy · Esc to close)%s",
		theme.FgDim, lineCount, noun, theme.Reset,
	)
}
