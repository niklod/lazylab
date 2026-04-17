package views

import (
	"strings"
	"sync"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/models"
)

const (
	diffEmptyHint   = "Select a file to view diff"
	diffBinaryHint  = "Binary file or no diff available"
	diffLoadingHint = "Loading diff…"
)

// DiffContentView renders the unified-diff body of the currently-selected
// MRDiffFile. Colouring mirrors Python render_diff_markup (mr_diff.py):
// file header lines bold, additions green, deletions red, hunk headers cyan.
//
// Scroll state lives on the gocui view (SetOrigin); handlers use viewport
// height to compute half-page deltas.
type DiffContentView struct {
	mu        sync.Mutex
	file      *models.MRDiffFile
	body      string
	bodyLines int
	status    string
}

// NewDiffContent constructs an empty DiffContentView. The caller must set
// content via SetFile, ShowLoading, or ShowError before the first Render
// (otherwise the pane shows the empty-state hint).
func NewDiffContent() *DiffContentView {
	return &DiffContentView{status: diffEmptyHint}
}

// SetFile replaces the rendered diff with the file's contents. Nil clears
// back to the empty-state hint.
func (d *DiffContentView) SetFile(file *models.MRDiffFile) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.file = file
	if file == nil {
		d.body = ""
		d.bodyLines = 0
		d.status = diffEmptyHint

		return
	}
	d.body = renderDiffMarkup(file.Diff)
	d.bodyLines = strings.Count(d.body, "\n") + 1
	d.status = ""
}

// ShowLoading displays a placeholder while the fetch is inflight.
func (d *DiffContentView) ShowLoading() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.file = nil
	d.body = ""
	d.bodyLines = 0
	d.status = diffLoadingHint
}

// ShowError replaces the diff with a red error message.
func (d *DiffContentView) ShowError(msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.file = nil
	d.body = ""
	d.bodyLines = 0
	d.status = ansiRed + msg + ansiReset
}

// CurrentFile returns the MRDiffFile currently displayed, or nil.
func (d *DiffContentView) CurrentFile() *models.MRDiffFile {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.file
}

// Render paints the diff into pane. Call from the layout callback.
func (d *DiffContentView) Render(pane *gocui.View) {
	d.mu.Lock()
	defer d.mu.Unlock()

	pane.Clear()
	pane.Wrap = false
	if d.status != "" {
		pane.WriteString(d.status + "\n")

		return
	}
	pane.WriteString(d.body)
}

// ScrollBy shifts the pane's origin by delta rows, clamped to the content.
// Half-page handlers pass ±innerH/2; single-line j/k pass ±1.
func (d *DiffContentView) ScrollBy(pane *gocui.View, delta int) {
	if pane == nil || delta == 0 {
		return
	}
	_, innerH := pane.InnerSize()
	if innerH <= 0 {
		return
	}
	_, oy := pane.Origin()

	totalLines := d.lineCount()

	target := oy + delta
	if maxOY := totalLines - innerH; target > maxOY {
		target = maxOY
	}
	if target < 0 {
		target = 0
	}
	pane.SetOrigin(0, target)
}

// ScrollToTop resets the pane origin to the first row. Called when SetFile
// replaces content so the new diff shows from the top regardless of where
// the previous file was scrolled.
func (d *DiffContentView) ScrollToTop(pane *gocui.View) {
	if pane == nil {
		return
	}
	pane.SetOrigin(0, 0)
}

func (d *DiffContentView) lineCount() int {
	if d.body == "" {
		if d.status == "" {
			return 0
		}

		return 1
	}

	return d.bodyLines
}

// renderDiffMarkup applies the Python-parity colour scheme to a raw unified
// diff. Exposed at package level so it can be table-tested independently of
// the view state.
func renderDiffMarkup(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ansiDim + diffBinaryHint + ansiReset
	}

	lines := strings.Split(raw, "\n")
	var sb strings.Builder
	sb.Grow(len(raw) + 16*len(lines))
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			sb.WriteString(ansiBold)
			sb.WriteString(line)
			sb.WriteString(ansiReset)
		case strings.HasPrefix(line, "+"):
			sb.WriteString(ansiGreen)
			sb.WriteString(line)
			sb.WriteString(ansiReset)
		case strings.HasPrefix(line, "-"):
			sb.WriteString(ansiRed)
			sb.WriteString(line)
			sb.WriteString(ansiReset)
		case strings.HasPrefix(line, "@@"):
			sb.WriteString(ansiCyan)
			sb.WriteString(line)
			sb.WriteString(ansiReset)
		default:
			sb.WriteString(line)
		}
		if i < len(lines)-1 {
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}
