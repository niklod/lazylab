package views

import (
	"fmt"
	"strings"
	"sync"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/models"
)

const (
	diffTreeIndent     = "  "
	diffTreeEmptyHint  = "No files changed."
	diffTreeStatusMod  = "M"
	diffTreeStatusAdd  = "A"
	diffTreeStatusDel  = "D"
	diffTreeStatusRen  = "R"
	diffTreeLoadingStr = "Loading files…"
)

// diffTreeRow is one rendered row in the flat file-tree listing. file is nil
// for directory header rows; leaves carry the domain model so j/k cursor
// selection can resolve the chosen file without a second lookup.
type diffTreeRow struct {
	text string
	file *models.MRDiffFile
}

// DiffTreeView is the left pane of the Diff tab — a flat, always-expanded
// tree of changed files grouped by directory. Mirrors Python DiffFileTree
// but renders as plain text: directory headers printed once, leaves
// indented under their parents with an A/M/D/R status letter.
//
// Cursor highlighting follows ReposView/MRsView: Highlight + SelBgColor on
// first render + placeCursor after writing the buffer. j/k skip over
// directory rows (file == nil) so Enter always lands on a real file.
//
// The Highlight properties are re-applied whenever Render receives a pane
// pointer it hasn't seen before. Diff sub-panes are unmounted on every
// tab cycle away from Diff and remounted on return; a single "configured
// once" flag would leave the remounted pane without a visible cursor row.
type DiffTreeView struct {
	mu       sync.Mutex
	rows     []diffTreeRow
	cursor   int
	status   string
	lastPane *gocui.View
}

// NewDiffTree constructs an empty DiffTreeView. Populate via SetFiles.
func NewDiffTree() *DiffTreeView {
	return &DiffTreeView{status: diffTreeLoadingStr}
}

// SetFiles replaces the tree contents. Cursor snaps to the first leaf row
// (or 0 when the tree is empty). Nil input shows the "Loading files…"
// hint; an empty (non-nil) slice shows the "No files changed." hint.
//
// Pointer ownership: the flattened rows store `&files[i]` so SelectedFile
// returns a stable handle. Callers MUST NOT mutate `files` after handing
// it in, and the returned pointer is valid only until the next SetFiles
// call (including implicit clears via commitMR).
func (d *DiffTreeView) SetFiles(files []models.MRDiffFile) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.rows = buildDiffTreeRows(files)
	d.cursor = firstLeafRow(d.rows)
	switch {
	case files == nil:
		d.status = diffTreeLoadingStr
	case len(files) == 0:
		d.status = diffTreeEmptyHint
	default:
		d.status = ""
	}
}

// ShowError replaces the tree with a red error message.
func (d *DiffTreeView) ShowError(msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.rows = nil
	d.cursor = 0
	d.status = ansiRed + msg + ansiReset
}

// SelectedFile returns the MRDiffFile under the cursor, or nil if the
// cursor is on a directory row (shouldn't happen after SetFiles but
// guarded anyway). The returned pointer shares storage with the slice
// passed to SetFiles — read-only, invalidated by the next SetFiles call.
func (d *DiffTreeView) SelectedFile() *models.MRDiffFile {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.cursor < 0 || d.cursor >= len(d.rows) {
		return nil
	}

	return d.rows[d.cursor].file
}

// Cursor returns the 0-indexed cursor row. Exposed for tests + scroll sync.
func (d *DiffTreeView) Cursor() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.cursor
}

// RowCount returns the total row count (directories + leaves).
func (d *DiffTreeView) RowCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.rows)
}

// MoveCursor shifts the cursor by delta rows, skipping over directory rows
// in the chosen direction. Returns true when the cursor actually moved —
// callers can use this to decide whether to emit a file-selection callback.
func (d *DiffTreeView) MoveCursor(delta int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.moveCursorLocked(delta)
}

// MoveCursorToStart puts the cursor on the first leaf row. No-op when the
// tree is empty.
func (d *DiffTreeView) MoveCursorToStart() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.cursor = firstLeafRow(d.rows)
}

// MoveCursorToEnd puts the cursor on the last leaf row.
func (d *DiffTreeView) MoveCursorToEnd() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.cursor = lastLeafRow(d.rows)
}

func (d *DiffTreeView) moveCursorLocked(delta int) bool {
	if len(d.rows) == 0 || delta == 0 {
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
	newCursor := d.cursor
	for remaining > 0 {
		candidate := newCursor + step
		if candidate < 0 || candidate >= len(d.rows) {
			break
		}
		newCursor = candidate
		if d.rows[newCursor].file == nil {
			continue
		}
		remaining--
	}

	// Rollback if we walked off into a directory row at the edge of the
	// tree — e.g. `k` on the first leaf above a header: the loop lands on
	// the header, then candidate-1 falls out of range, so we'd commit a
	// directory row. Cursor must always land on a leaf.
	if newCursor == d.cursor || d.rows[newCursor].file == nil {
		return false
	}
	d.cursor = newCursor

	return true
}

// Render paints the tree into pane. Configures cursor colours on first call.
func (d *DiffTreeView) Render(pane *gocui.View) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.lastPane != pane {
		pane.Highlight = true
		pane.SelBgColor = gocui.ColorGreen
		pane.SelFgColor = gocui.ColorBlack
		d.lastPane = pane
	}

	pane.Clear()
	if d.status != "" {
		pane.WriteString(d.status + "\n")
		placeCursor(pane, 0, 1)

		return
	}
	for _, row := range d.rows {
		pane.WriteString(row.text + "\n")
	}
	placeCursor(pane, d.cursor, len(d.rows))
}

// buildDiffTreeRows flattens the files into a list of rows. Directories are
// printed once (lazily, when their first file appears). File order follows
// the server — no alphabetical resort so parity holds with Python.
func buildDiffTreeRows(files []models.MRDiffFile) []diffTreeRow {
	if len(files) == 0 {
		return nil
	}

	rows := make([]diffTreeRow, 0, len(files)*2)
	printed := make(map[string]struct{}, len(files))

	for i := range files {
		f := files[i]
		dir, filename := splitDirFile(f.NewPath)

		if dir != "" {
			parts := strings.Split(dir, "/")
			cumulative := ""
			for depth, seg := range parts {
				if cumulative == "" {
					cumulative = seg
				} else {
					cumulative += "/" + seg
				}
				if _, seen := printed[cumulative]; seen {
					continue
				}
				printed[cumulative] = struct{}{}
				rows = append(rows, diffTreeRow{
					text: fmt.Sprintf("%s%s%s/%s",
						strings.Repeat(diffTreeIndent, depth),
						ansiBold, seg, ansiReset,
					),
				})
			}
		}

		indent := 0
		if dir != "" {
			indent = strings.Count(dir, "/") + 1
		}
		rows = append(rows, diffTreeRow{
			text: fmt.Sprintf("%s%s %s",
				strings.Repeat(diffTreeIndent, indent),
				fileStatusLabel(&f),
				filename,
			),
			file: &files[i],
		})
	}

	return rows
}

func splitDirFile(path string) (dir, name string) {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "", path
	}

	return path[:i], path[i+1:]
}

// fileStatusLabel maps MRDiffFile flags to a coloured A/M/D/R glyph. Mirrors
// Python _file_status_label.
func fileStatusLabel(f *models.MRDiffFile) string {
	switch {
	case f.NewFile:
		return ansiGreen + diffTreeStatusAdd + ansiReset
	case f.DeletedFile:
		return ansiRed + diffTreeStatusDel + ansiReset
	case f.RenamedFile:
		return ansiYellow + diffTreeStatusRen + ansiReset
	default:
		return ansiCyan + diffTreeStatusMod + ansiReset
	}
}

func firstLeafRow(rows []diffTreeRow) int {
	for i, r := range rows {
		if r.file != nil {
			return i
		}
	}

	return 0
}

func lastLeafRow(rows []diffTreeRow) int {
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].file != nil {
			return i
		}
	}

	return 0
}
