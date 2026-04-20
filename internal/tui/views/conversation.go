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

// conversationNow is the clock used when formatting relative note timestamps.
// Tests freeze it.
var conversationNow = time.Now

const (
	conversationLoadingHint    = "Loading conversation…"
	conversationEmptyHint      = "No discussions on this merge request."
	conversationResolvedHeader = "Resolved threads"
	conversationGeneralHeader  = "General comments"

	convIndent         = " "      // left gutter before glyph
	convSpineMiddle    = "\u2502" // │
	convSpineEnd       = "\u2575" // ╵
	convSpineSingle    = "\u2575" // ╵ — used for threads with no replies
	convBullet         = "\u25CB" // ○ unresolved head
	convBulletResolved = "\u25CF" // ● resolved head

	convDividerWidth = 61 // ─ repeat count for the resolved / general divider
)

// sanitizeInline strips CSI SGR escape sequences and other C0/C1 control
// characters from a string so GitLab-supplied content (note bodies, author
// usernames, file paths) can't inject arbitrary terminal control sequences —
// SGR colour bleeds, cursor moves, scroll-region changes, OSC — into the
// LazyLab TUI. A CR (`\r`) inside a single visual row would also overprint
// the spine gutter on the same line, so it's collapsed too. Tabs and
// newlines survive because they are meaningful to callers that split first.
// For inline contexts (usernames, paths) callers should pass already-
// single-line input; this helper does NOT normalize newlines.
func sanitizeInline(s string) string {
	if s == "" {
		return s
	}
	// Strip CSI SGR (ESC [ … m) first — the existing stripSGR helper covers
	// the escape sequence boundaries, including params and terminator, that a
	// naïve rune-by-rune drop of 0x1b would leave behind.
	s = stripSGR(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\x1b' || r == '\r' || r == '\x7f' {
			continue
		}
		if r < 0x20 && r != '\t' && r != '\n' {
			continue
		}
		if r >= 0x80 && r < 0xa0 {
			continue
		}
		b.WriteRune(r)
	}

	return b.String()
}

// rowKind discriminates rows inside the conversation pane so keyboard motion
// can skip over non-selectable rows (dividers, section headers, indented
// note bodies) and land on threads and general-comment headers.
type rowKind int

const (
	rowKindUnresolvedHeader rowKind = iota
	rowKindUnresolvedNote
	rowKindUnresolvedBlank
	rowKindResolvedSectionHeader
	rowKindResolvedCollapsed
	rowKindResolvedHeader
	rowKindResolvedNote
	rowKindResolvedBlank
	rowKindDivider
	rowKindGeneralSectionHeader
	rowKindGeneralHeader
	rowKindGeneralBody
	rowKindGeneralBlank
	rowKindEmpty
)

// conversationRow is one pre-rendered line in the conversation pane. `text`
// already carries all SGR coloring; `kind` drives cursor logic.
//
// threadIdx points into the unresolved/resolved slice the row came from;
// generalIdx into the general-comments slice. noteIdx is 0-based within
// the thread's VisibleNotes (0 = head). `selectable` marks rows j/k may
// land on. `selectionAnchor` identifies the base row of the current logical
// selection (header rows act as anchors for their notes) so the highlight
// moves in lockstep with the logical cursor.
type conversationRow struct {
	kind            rowKind
	text            string
	threadIdx       int
	noteIdx         int
	generalIdx      int
	selectable      bool
	selectionAnchor int
}

// ConversationView renders an MR's discussions. See docs/adr/019-go-
// conversation-tab.md for the thread-card layout and two-level cursor
// rationale.
type ConversationView struct {
	mu sync.Mutex

	unresolved []*models.Discussion
	resolved   []*models.Discussion
	general    []*models.Discussion

	rows []conversationRow

	threadCursor     int
	noteCursor       int
	selectedAnchor   int
	expandedResolved map[string]bool // key: discussion.ID; absent ⇒ collapsed
	rebuildWidth     int             // pane width rows were last built for (0 = unset → no wrap)
	loading          bool
	errMsg           string

	chromeTitle string
	chromeMeta  string
	lastPane    *gocui.View
}

// NewConversation constructs a view in the "loading" state. SetDiscussions
// replaces the placeholder with real data.
func NewConversation() *ConversationView {
	return &ConversationView{
		loading:          true,
		expandedResolved: map[string]bool{},
	}
}

// SetDiscussions ingests a fresh list of discussions. Buckets them into
// unresolved / resolved / general, rebuilds the row cache, resets cursors.
// Accepts nil to render the empty-state hint.
func (c *ConversationView) SetDiscussions(discs []*models.Discussion) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.loading = false
	c.errMsg = ""
	c.unresolved = c.unresolved[:0]
	c.resolved = c.resolved[:0]
	c.general = c.general[:0]
	c.expandedResolved = map[string]bool{}

	for _, d := range discs {
		if d == nil || len(d.Notes) == 0 {
			continue
		}
		if len(d.VisibleNotes()) == 0 {
			continue
		}
		switch {
		case !d.IsResolvable():
			c.general = append(c.general, d)
		case d.IsResolved():
			c.resolved = append(c.resolved, d)
		default:
			c.unresolved = append(c.unresolved, d)
		}
	}

	c.rebuildRowsLocked()
	c.threadCursor = 0
	c.noteCursor = 0
	c.selectedAnchor = firstSelectableRow(c.rows)
}

// ShowLoading flips the view into the dim "Loading…" state.
func (c *ConversationView) ShowLoading() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.loading = true
	c.errMsg = ""
	c.unresolved = nil
	c.resolved = nil
	c.general = nil
	c.rows = nil
	c.threadCursor = 0
	c.noteCursor = 0
	c.selectedAnchor = 0
	c.expandedResolved = map[string]bool{}
}

// ShowError replaces the view content with a red error message.
func (c *ConversationView) ShowError(msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.loading = false
	c.errMsg = msg
	c.rows = nil
	c.threadCursor = 0
	c.noteCursor = 0
	c.selectedAnchor = 0
}

// SetChrome stores the title/meta pair painted on the pane's first line.
func (c *ConversationView) SetChrome(title, meta string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.chromeTitle = title
	c.chromeMeta = meta
}

// ToggleExpandResolvedUnderCursor flips the expand state of the single
// resolved thread the cursor is currently parked on. No-op when the cursor
// sits on an unresolved thread, general comment, or non-thread row. Bound
// to `e`; the "expand every resolved thread" mass-toggle lives in
// ToggleExpandAllResolved (bound to `E`).
func (c *ConversationView) ToggleExpandResolvedUnderCursor() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	anchor := c.rowAtLocked(c.selectedAnchor)
	if anchor.kind != rowKindResolvedHeader && anchor.kind != rowKindResolvedCollapsed {
		return false
	}
	if anchor.threadIdx < 0 || anchor.threadIdx >= len(c.resolved) {
		return false
	}
	id := c.resolved[anchor.threadIdx].ID
	if c.expandedResolved[id] {
		delete(c.expandedResolved, id)
	} else {
		c.expandedResolved[id] = true
	}
	c.rebuildPreservingCursorLocked()

	return true
}

// ToggleExpandAllResolved flips every resolved thread between collapsed and
// expanded as a single group. Bound to `E`. If at least one resolved thread
// is currently collapsed, all expand; otherwise all collapse.
func (c *ConversationView) ToggleExpandAllResolved() {
	c.mu.Lock()
	defer c.mu.Unlock()

	anyCollapsed := false
	for _, d := range c.resolved {
		if !c.expandedResolved[d.ID] {
			anyCollapsed = true

			break
		}
	}
	if anyCollapsed {
		for _, d := range c.resolved {
			c.expandedResolved[d.ID] = true
		}
	} else {
		c.expandedResolved = map[string]bool{}
	}
	c.rebuildPreservingCursorLocked()
}

// rebuildPreservingCursorLocked rebuilds the row cache and relocates the
// cursor onto the same logical anchor (thread or general-comment) it was
// pointing at before the rebuild. Caller holds c.mu.
func (c *ConversationView) rebuildPreservingCursorLocked() {
	prev, hadPrev := anchorIdentityOf(c.rowAtLocked(c.selectedAnchor))

	c.rebuildRowsLocked()

	if hadPrev {
		if idx := findAnchorByIdentity(c.rows, prev); idx >= 0 {
			c.selectedAnchor = idx
			c.noteCursor = 0
			c.updateThreadCursorFromAnchorLocked()

			return
		}
	}
	c.clampCursorsLocked()
}

func (c *ConversationView) rowAtLocked(idx int) conversationRow {
	if idx < 0 || idx >= len(c.rows) {
		return conversationRow{}
	}

	return c.rows[idx]
}

// ExpandAllResolved reports whether every resolved thread is currently in
// the expanded state. Exposed for tests.
func (c *ConversationView) ExpandAllResolved() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.resolved) == 0 {
		return false
	}
	for _, d := range c.resolved {
		if !c.expandedResolved[d.ID] {
			return false
		}
	}

	return true
}

// IsResolvedThreadExpanded reports whether the resolved thread at index i
// (matching the resolved-bucket order) is currently expanded. Exposed for
// tests.
func (c *ConversationView) IsResolvedThreadExpanded(i int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if i < 0 || i >= len(c.resolved) {
		return false
	}

	return c.expandedResolved[c.resolved[i].ID]
}

// ThreadCount returns the aggregate thread count (unresolved + resolved),
// excluding general comments. Used by the chrome meta line.
func (c *ConversationView) ThreadCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.unresolved) + len(c.resolved)
}

// UnresolvedCount returns the number of unresolved review threads.
func (c *ConversationView) UnresolvedCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.unresolved)
}

// MoveThreadCursor advances the logical cursor by delta threads. j/k → ±1.
// When the cursor is on a general-comment header, the step still moves by
// one header row (general headers are selectable alongside threads).
func (c *ConversationView) MoveThreadCursor(delta int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.rows) == 0 || delta == 0 {
		return false
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	anchors := selectableAnchors(c.rows)
	if len(anchors) == 0 {
		return false
	}
	idx := currentAnchorIndex(anchors, c.selectedAnchor)
	remaining := delta
	if remaining < 0 {
		remaining = -remaining
	}
	for remaining > 0 {
		next := idx + step
		if next < 0 || next >= len(anchors) {
			break
		}
		idx = next
		remaining--
	}
	if anchors[idx] == c.selectedAnchor && c.noteCursor == 0 {
		return false
	}
	c.selectedAnchor = anchors[idx]
	c.noteCursor = 0
	c.updateThreadCursorFromAnchorLocked()

	return true
}

// MoveNoteCursor shifts the note-level cursor within the current thread.
// No-op for general comments (they render as a single block). Cursor range
// is 0..N where N = visible note count: 0 addresses the header row, 1..N
// addresses the N note rows (see selectedRenderRowLocked for the mapping).
// Clamp of (N-1) would make the last note unreachable.
func (c *ConversationView) MoveNoteCursor(delta int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if delta == 0 {
		return false
	}
	notes := c.visibleNotesForAnchorLocked(c.selectedAnchor)
	if notes == 0 {
		return false
	}
	target := c.noteCursor + delta
	if target < 0 {
		target = 0
	}
	if target > notes {
		target = notes
	}
	if target == c.noteCursor {
		return false
	}
	c.noteCursor = target

	return true
}

// MoveToStart places the cursor on the first selectable row.
func (c *ConversationView) MoveToStart() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.selectedAnchor = firstSelectableRow(c.rows)
	c.noteCursor = 0
	c.updateThreadCursorFromAnchorLocked()
}

// MoveToEnd places the cursor on the last selectable row.
func (c *ConversationView) MoveToEnd() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.selectedAnchor = lastSelectableRow(c.rows)
	c.noteCursor = 0
	c.updateThreadCursorFromAnchorLocked()
}

// RowCount returns total rendered row count. Exposed for tests + scroll math.
func (c *ConversationView) RowCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.rows)
}

// SnapshotRows exposes a copy of the row cache for tests. Returns a fresh
// slice so callers cannot mutate internal state.
func (c *ConversationView) SnapshotRows() []conversationRow {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]conversationRow, len(c.rows))
	copy(out, c.rows)

	return out
}

// Cursor returns the (threadCursor, noteCursor) pair. Exposed for tests.
func (c *ConversationView) Cursor() (thread, note int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.threadCursor, c.noteCursor
}

// Render paints the pane. Layout:
//
//	<chrome>
//	<blank>
//	<unresolved threads>
//	<resolved section>
//	<divider>
//	<general section>
//	<blank>
//	<keybind strip>
func (c *ConversationView) Render(pane *gocui.View) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.lastPane != pane {
		pane.Highlight = true
		pane.SelBgColor = theme.ColorAccent
		pane.SelFgColor = theme.ColorSelectionFg
		c.lastPane = pane
	}

	innerW, _ := pane.InnerSize()
	pane.Clear()

	chromeOffset := 0
	if chrome := renderChromeLine(c.chromeTitle, c.chromeMeta, innerW); chrome != "" {
		pane.WriteString(chrome + "\n\n")
		chromeOffset = 2
	}

	if status := c.statusLineLocked(); status != "" {
		pane.WriteString(status + "\n")
		placeCursor(pane, chromeOffset, chromeOffset+1)

		return
	}

	if innerW != c.rebuildWidth {
		c.rebuildWidth = innerW
		c.rebuildPreservingCursorLocked()
	}

	selectedRow := c.selectedRenderRowLocked()
	physicalOffset := 0
	rowPhysicalLine := make([]int, len(c.rows))
	for i, row := range c.rows {
		rowPhysicalLine[i] = physicalOffset
		pane.WriteString(row.text + "\n")
		// Each row.text may itself contain newlines (note blocks render
		// "@author age\n    body"); a single row can occupy multiple
		// physical pane lines. Count the embedded newlines plus the
		// trailing one we just added so placeCursor lands on the real
		// line, not the logical row index.
		physicalOffset += strings.Count(row.text, "\n") + 1
	}
	cursorLine := chromeOffset
	if selectedRow >= 0 && selectedRow < len(rowPhysicalLine) {
		cursorLine = chromeOffset + rowPhysicalLine[selectedRow]
	}
	totalLines := chromeOffset + physicalOffset
	placeCursor(pane, cursorLine, totalLines)
}

// statusLineLocked returns the status hint for empty/loading/error states,
// or "" when the pane should render discussions. Caller holds c.mu.
func (c *ConversationView) statusLineLocked() string {
	switch {
	case c.errMsg != "":
		return theme.FgErr + c.errMsg + theme.Reset
	case c.loading:
		return theme.FgDim + conversationLoadingHint + theme.Reset
	case len(c.rows) == 0:
		return theme.FgDim + conversationEmptyHint + theme.Reset
	}

	return ""
}

// selectedRenderRowLocked returns the concrete row index the gocui cursor
// should sit on (anchor + noteCursor offset). Caller holds c.mu.
func (c *ConversationView) selectedRenderRowLocked() int {
	anchor := c.selectedAnchor
	if anchor < 0 || anchor >= len(c.rows) {
		return 0
	}
	if c.noteCursor == 0 {
		return anchor
	}
	count := 0
	for i := anchor + 1; i < len(c.rows); i++ {
		row := c.rows[i]
		if row.selectionAnchor != anchor {
			break
		}
		if row.kind != rowKindUnresolvedNote && row.kind != rowKindResolvedNote {
			continue
		}
		count++
		if count == c.noteCursor {
			return i
		}
	}

	return anchor
}

// visibleNotesForAnchorLocked counts the visible notes belonging to the
// thread anchored at rowIdx. Returns 0 when the anchor isn't a thread
// header. Caller holds c.mu.
func (c *ConversationView) visibleNotesForAnchorLocked(rowIdx int) int {
	if rowIdx < 0 || rowIdx >= len(c.rows) {
		return 0
	}
	anchor := c.rows[rowIdx]
	switch anchor.kind {
	case rowKindUnresolvedHeader:
		if anchor.threadIdx < len(c.unresolved) {
			return c.unresolved[anchor.threadIdx].VisibleNoteCount()
		}
	case rowKindResolvedHeader:
		if anchor.threadIdx < len(c.resolved) {
			return c.resolved[anchor.threadIdx].VisibleNoteCount()
		}
	}

	return 0
}

// rebuildRowsLocked recomputes c.rows from the current buckets + expand
// flag using the pane width stored in c.rebuildWidth. Caller holds c.mu.
func (c *ConversationView) rebuildRowsLocked() {
	rows := make([]conversationRow, 0, 32)
	now := conversationNow()
	width := c.rebuildWidth

	for i, d := range c.unresolved {
		rows = appendThreadRows(rows, d, i, false, now, width)
	}

	if len(c.resolved) > 0 {
		if len(rows) > 0 {
			rows = append(rows, conversationRow{kind: rowKindResolvedBlank, text: ""})
		}
		rows = append(rows, conversationRow{
			kind: rowKindResolvedSectionHeader,
			text: convIndent + theme.FgDim + conversationResolvedHeader + theme.Reset,
		})
		for i, d := range c.resolved {
			if c.expandedResolved[d.ID] {
				rows = appendThreadRows(rows, d, i, true, now, width)
			} else {
				rows = append(rows, collapsedResolvedRow(d, i))
			}
		}
	}

	if len(c.general) > 0 {
		if len(rows) > 0 {
			rows = append(rows, conversationRow{kind: rowKindDivider, text: dividerLine()})
		}
		rows = append(rows, conversationRow{
			kind: rowKindGeneralSectionHeader,
			text: convIndent + theme.Bold + conversationGeneralHeader + theme.Reset,
		})
		for i, d := range c.general {
			rows = appendGeneralRows(rows, d, i, now, width)
		}
	}

	fillSelectionAnchors(rows)
	c.rows = rows
}

// clampCursorsLocked brings cursors back onto a selectable row after a
// rebuild. Caller holds c.mu.
func (c *ConversationView) clampCursorsLocked() {
	if len(c.rows) == 0 {
		c.selectedAnchor = 0
		c.threadCursor = 0
		c.noteCursor = 0

		return
	}
	if c.selectedAnchor < 0 || c.selectedAnchor >= len(c.rows) || !c.rows[c.selectedAnchor].selectable {
		c.selectedAnchor = firstSelectableRow(c.rows)
	}
	notes := c.visibleNotesForAnchorLocked(c.selectedAnchor)
	if notes == 0 {
		c.noteCursor = 0
	} else if c.noteCursor > notes {
		c.noteCursor = notes
	}
	c.updateThreadCursorFromAnchorLocked()
}

func (c *ConversationView) updateThreadCursorFromAnchorLocked() {
	anchors := selectableAnchors(c.rows)
	c.threadCursor = currentAnchorIndex(anchors, c.selectedAnchor)
}

// appendThreadRows emits a thread header + its visible notes. `isResolved`
// picks the dim colour palette. innerW is the pane width the row text is
// wrapped to (0 disables wrapping — useful in unit tests that do not attach
// a real pane).
func appendThreadRows(rows []conversationRow, d *models.Discussion, idx int, isResolved bool, now time.Time, innerW int) []conversationRow {
	visible := d.VisibleNotes()
	if len(visible) == 0 {
		return rows
	}
	headerColor, spineColor, bullet := theme.FgErr, theme.FgErr, convBullet
	if isResolved {
		headerColor, spineColor, bullet = theme.FgDim, theme.FgDim, convBulletResolved
	}
	kindHeader, kindNote := rowKindUnresolvedHeader, rowKindUnresolvedNote
	if isResolved {
		kindHeader, kindNote = rowKindResolvedHeader, rowKindResolvedNote
	}

	rows = append(rows, conversationRow{
		kind:       kindHeader,
		threadIdx:  idx,
		selectable: true,
		text:       renderThreadHeader(d, bullet, headerColor, isResolved),
	})

	for i, note := range visible {
		isLast := i == len(visible)-1
		spine := chooseSpine(len(visible), isLast)
		rows = append(rows, conversationRow{
			kind:       kindNote,
			threadIdx:  idx,
			noteIdx:    i,
			selectable: false,
			text:       renderNoteBlock(spine, spineColor, note, now, innerW),
		})
	}
	rows = append(rows, conversationRow{kind: rowKindUnresolvedBlank, text: ""})

	return rows
}

func appendGeneralRows(rows []conversationRow, d *models.Discussion, idx int, now time.Time, innerW int) []conversationRow {
	head := d.Head()
	if head == nil {
		return rows
	}
	rows = append(rows, conversationRow{
		kind:       rowKindGeneralHeader,
		generalIdx: idx,
		selectable: true,
		text:       renderGeneralHeader(head, now),
	})
	rows = append(rows, conversationRow{
		kind:       rowKindGeneralBody,
		generalIdx: idx,
		selectable: false,
		text:       renderGeneralBody(head.Body, innerW),
	})
	rows = append(rows, conversationRow{kind: rowKindGeneralBlank, text: ""})

	return rows
}

func collapsedResolvedRow(d *models.Discussion, idx int) conversationRow {
	return conversationRow{
		kind:       rowKindResolvedCollapsed,
		threadIdx:  idx,
		selectable: true,
		text:       renderResolvedCollapsed(d),
	}
}

// renderThreadHeader produces `  ○ Thread N · <location> · unresolved` (or
// the resolved equivalent).
func renderThreadHeader(d *models.Discussion, bullet, color string, isResolved bool) string {
	var sb strings.Builder
	sb.WriteString(convIndent)
	sb.WriteString(color + bullet + theme.Reset + " ")
	sb.WriteString(theme.Bold + fmt.Sprintf("Thread · %s", shortDiscussionID(d.ID)) + theme.Reset)

	if loc := d.LocationHint(); loc != "" {
		sb.WriteString(theme.FgDim + " · " + theme.Reset)
		sb.WriteString(theme.FgDim + sanitizeInline(loc) + theme.Reset)
	}
	if isResolved {
		if r := d.Resolver(); r != nil && r.Username != "" {
			sb.WriteString(theme.FgDim + " · resolved by " + theme.Reset)
			sb.WriteString(theme.FgAccent + "@" + sanitizeInline(r.Username) + theme.Reset)
		} else {
			sb.WriteString(theme.FgDim + " · resolved" + theme.Reset)
		}
	} else {
		sb.WriteString(theme.FgDim + " · unresolved" + theme.Reset)
	}

	return sb.String()
}

// renderNoteBlock renders one note attached to a thread spine. Output is a
// two-line-ish block: "<spine>   @author  <age>" then the body lines,
// each prefixed by the spine glyph. `spine` is passed in by the caller so
// this function stays ignorant of position within the thread.
func renderNoteBlock(spine, spineColor string, note models.Note, now time.Time, innerW int) string {
	spineCell := spineColor + spine + theme.Reset
	var sb strings.Builder
	sb.WriteString(convIndent)
	sb.WriteString(spineCell)
	sb.WriteString("   ")
	sb.WriteString(theme.FgAccent + "@" + sanitizeInline(note.Author.Username) + theme.Reset)
	if age := theme.Relative(note.CreatedAt, now); age != "" {
		sb.WriteString("  " + theme.FgDim + age + theme.Reset)
	}
	if note.Resolved {
		sb.WriteString("  " + theme.FgOK + "\u2713 resolved" + theme.Reset)
	}
	sb.WriteByte('\n')
	bodyIndent := convIndent + spineCell + "     "
	sb.WriteString(wrapBodyWithIndent(sanitizeInline(note.Body), bodyIndent, innerW))

	return sb.String()
}

func renderGeneralHeader(note *models.Note, now time.Time) string {
	var sb strings.Builder
	sb.WriteString(convIndent)
	sb.WriteString("  ")
	sb.WriteString(theme.FgAccent + "@" + sanitizeInline(note.Author.Username) + theme.Reset)
	if age := theme.Relative(note.CreatedAt, now); age != "" {
		sb.WriteString(theme.FgDim + " · " + age + theme.Reset)
	}

	return sb.String()
}

func renderGeneralBody(body string, innerW int) string {
	return wrapBodyWithIndent(sanitizeInline(body), convIndent+"    ", innerW)
}

// wrapBodyWithIndent renders body text so each physical line starts with the
// same indent. Long logical lines are word-wrapped to fit `innerW` cells;
// innerW ≤ 0 disables wrapping and each logical `\n`-separated segment
// becomes a single row. Preserves blank lines in the source body.
func wrapBodyWithIndent(body, indent string, innerW int) string {
	indentW := visibleWidth(indent)
	avail := innerW - indentW
	var sb strings.Builder
	for i, logical := range strings.Split(body, "\n") {
		if i > 0 {
			sb.WriteByte('\n')
		}
		chunks := softWrapLine(logical, avail)
		if len(chunks) == 0 {
			sb.WriteString(indent)

			continue
		}
		for j, chunk := range chunks {
			if j > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(indent)
			sb.WriteString(chunk)
		}
	}

	return sb.String()
}

// softWrapLine word-wraps a single logical line to `width` cells. Returns
// [s] unchanged when width ≤ 0 or the line already fits. Prefers word
// boundaries (space); hard-splits an overflowing single word.
func softWrapLine(s string, width int) []string {
	if s == "" {
		return nil
	}
	if width <= 0 || visibleWidth(s) <= width {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{s}
	}
	var out []string
	var cur strings.Builder
	curW := 0
	for _, w := range words {
		wW := visibleWidth(w)
		switch {
		case cur.Len() == 0:
			cur.WriteString(w)
			curW = wW
		case curW+1+wW > width:
			out = append(out, cur.String())
			cur.Reset()
			cur.WriteString(w)
			curW = wW
		default:
			cur.WriteByte(' ')
			cur.WriteString(w)
			curW += 1 + wW
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	var final []string
	for _, line := range out {
		if visibleWidth(line) <= width {
			final = append(final, line)

			continue
		}
		final = append(final, hardSplit(line, width)...)
	}

	return final
}

// hardSplit breaks a string that has no usable space boundary into chunks
// of at most `width` cells. Rune-aware so Cyrillic / CJK don't get cut
// mid-codepoint.
func hardSplit(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var out []string
	var cur strings.Builder
	curW := 0
	for _, r := range s {
		rw := visibleWidth(string(r))
		if curW+rw > width && cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
			curW = 0
		}
		cur.WriteRune(r)
		curW += rw
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}

	return out
}

func renderResolvedCollapsed(d *models.Discussion) string {
	var sb strings.Builder
	sb.WriteString(convIndent)
	sb.WriteString(theme.FgDim + convBulletResolved + " " + theme.Reset)
	sb.WriteString(theme.FgDim + fmt.Sprintf("Thread · %s", shortDiscussionID(d.ID)) + theme.Reset)
	if loc := d.LocationHint(); loc != "" {
		sb.WriteString(theme.FgDim + " · " + sanitizeInline(loc) + theme.Reset)
	}
	if r := d.Resolver(); r != nil && r.Username != "" {
		sb.WriteString(theme.FgDim + " · resolved by @" + sanitizeInline(r.Username) + theme.Reset)
	} else {
		sb.WriteString(theme.FgDim + " · resolved" + theme.Reset)
	}
	if replies := len(d.Replies()); replies > 0 {
		sb.WriteString(theme.FgDim + fmt.Sprintf(" · %d replies", replies) + theme.Reset)
	}

	return sb.String()
}

// chooseSpine picks `│` / `╵` / `╷` based on the note's position inside the
// thread so the vertical line terminates cleanly.
func chooseSpine(total int, isLast bool) string {
	if total == 1 {
		return convSpineSingle
	}
	if isLast {
		return convSpineEnd
	}

	return convSpineMiddle
}

func shortDiscussionID(id string) string {
	if len(id) <= 8 {
		return id
	}

	return id[:8]
}

func dividerLine() string {
	return convIndent + theme.FgDim + strings.Repeat("\u2500", convDividerWidth) + theme.Reset
}

func fillSelectionAnchors(rows []conversationRow) {
	currentAnchor := 0
	for i := range rows {
		if rows[i].selectable {
			currentAnchor = i
		}
		rows[i].selectionAnchor = currentAnchor
	}
}

func firstSelectableRow(rows []conversationRow) int {
	for i, r := range rows {
		if r.selectable {
			return i
		}
	}

	return 0
}

func lastSelectableRow(rows []conversationRow) int {
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].selectable {
			return i
		}
	}

	return 0
}

func selectableAnchors(rows []conversationRow) []int {
	var out []int
	for i, r := range rows {
		if r.selectable {
			out = append(out, i)
		}
	}

	return out
}

func currentAnchorIndex(anchors []int, target int) int {
	for i, a := range anchors {
		if a == target {
			return i
		}
	}

	return 0
}

// anchorIdentity names a selectable row by its domain meaning instead of its
// row-array position. A resolved thread keeps the same identity across
// collapse/expand (kind flips between collapsed and header, but bucket + idx
// remain); the cursor can therefore follow the user's intended selection
// through a rebuild rather than snapping to the first row.
type anchorIdentity struct {
	bucket string
	idx    int
}

const (
	anchorBucketUnresolved = "unresolved"
	anchorBucketResolved   = "resolved"
	anchorBucketGeneral    = "general"
)

func anchorIdentityOf(row conversationRow) (anchorIdentity, bool) {
	switch row.kind {
	case rowKindUnresolvedHeader:
		return anchorIdentity{anchorBucketUnresolved, row.threadIdx}, true
	case rowKindResolvedHeader, rowKindResolvedCollapsed:
		return anchorIdentity{anchorBucketResolved, row.threadIdx}, true
	case rowKindGeneralHeader:
		return anchorIdentity{anchorBucketGeneral, row.generalIdx}, true
	}

	return anchorIdentity{}, false
}

func findAnchorByIdentity(rows []conversationRow, id anchorIdentity) int {
	for i, row := range rows {
		if !row.selectable {
			continue
		}
		got, ok := anchorIdentityOf(row)
		if !ok {
			continue
		}
		if got == id {
			return i
		}
	}

	return -1
}
