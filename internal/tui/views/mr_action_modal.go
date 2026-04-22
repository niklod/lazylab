package views

import (
	"fmt"
	"strings"
	"sync"

	"github.com/jesseduffield/gocui"
	"github.com/rivo/uniseg"

	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/theme"
)

// ModalKind distinguishes the two confirmation dialogs the MR pane triggers.
type ModalKind int

const (
	ModalClose ModalKind = iota
	ModalMerge
)

// Default widths/heights used by the layout package to size the sub-pane.
// Exported so layout.go can pre-size without re-deriving from kind.
// The Close/Merge constants are floor heights — the actual pane size is
// decided by ModalHeight, which grows the pane to fit wrapped errors.
const (
	ModalCloseHeight = 8
	ModalMergeHeight = 14
	ModalWidth       = 60

	// Inner writable width: frame eats 2 cells (left + right border) and the
	// body pads 2 cells on the left. The divider already uses ModalWidth-4,
	// so we match it here.
	modalInnerWidth = ModalWidth - 4

	// Row budgets for the static body blocks. See Render for the walk.
	modalCloseBaseRows = 3 // blank / title / divider
	modalMergeBaseRows = 7 // blank / title / branches / squash / delete / blank / divider
	modalStripRows     = 1 // inline keybind strip
	modalBusySlackRows = 1 // "Running…" line, allocated pessimistically
)

// ModalHeight returns the pane height required to render the modal body.
// errLines is the wrapped-error line count (see wrapErrMsg); pass 0 for the
// no-error case. The result is clamped to the floor constants so the widget
// never shrinks below the known-good default, even with an empty body.
func ModalHeight(kind ModalKind, errLines int) int {
	base, floor := modalCloseBaseRows, ModalCloseHeight
	if kind == ModalMerge {
		base, floor = modalMergeBaseRows, ModalMergeHeight
	}

	return max(base+modalStripRows+modalBusySlackRows+errLines, floor)
}

// Titles live here so layout.go and the widget stay in sync.
const (
	modalCloseTitle = " Close this MR? "
	modalMergeTitle = " Merge this MR? "
)

// ModalSnapshot is a value-type view of the modal used for atomic reads.
// Held out here (not interleaved with MRActionModal methods) per the
// struct/method grouping rule in golang/coding-style.md.
type ModalSnapshot struct {
	Active       bool
	Kind         ModalKind
	MR           *models.MergeRequest
	DeleteBranch bool
	Squash       bool
	Busy         bool
	Locked       bool
	ErrMsg       string
	ErrLines     []string
}

// MRActionModal is the single widget used for both Close and Merge
// confirmation dialogs. Lifecycle: Open → (toggle*/confirm/cancel) → Close.
// Layout mounts a centered sub-pane while IsActive(); Render paints it.
type MRActionModal struct {
	mu           sync.Mutex
	active       bool
	kind         ModalKind
	mr           *models.MergeRequest
	deleteBranch bool
	squash       bool
	busy         bool
	// locked marks a modal that was opened purely to surface a guard
	// reason (e.g. "Cannot close: MR !N is merged"). Distinct from busy:
	// busy means "mutation in flight, Esc ignored"; locked means "no valid
	// mutation exists, Enter is a no-op, Esc still works".
	locked   bool
	errMsg   string
	errLines []string
}

func NewMRActionModal() *MRActionModal {
	return &MRActionModal{}
}

// Open activates the modal for the given kind + MR. Merge defaults mirror
// the wireframe: delete-branch on, squash off.
func (m *MRActionModal) Open(kind ModalKind, mr *models.MergeRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.applyOpenLocked(kind, mr)
}

// OpenGuarded opens the modal in a read-only state that surfaces reason as
// a pre-seeded error. Used when the selected MR cannot accept the action
// (closed/merged MR) — the user gets the same visual feedback channel as a
// mutation failure instead of a toast, and Enter is a no-op until dismissal.
func (m *MRActionModal) OpenGuarded(kind ModalKind, mr *models.MergeRequest, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.applyOpenLocked(kind, mr)
	m.locked = true
	m.errMsg = reason
	m.errLines = wrapErrMsg(reason, modalInnerWidth)
}

// applyOpenLocked centralises the shared state reset between Open and
// OpenGuarded. Caller holds m.mu.
func (m *MRActionModal) applyOpenLocked(kind ModalKind, mr *models.MergeRequest) {
	m.active = true
	m.kind = kind
	m.mr = mr
	m.busy = false
	m.locked = false
	m.errMsg = ""
	m.errLines = nil
	m.deleteBranch = kind == ModalMerge
	m.squash = false
}

// Close deactivates the modal and clears transient state. Safe to call
// from any goroutine; layout's next tick removes the sub-pane.
func (m *MRActionModal) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.active = false
	m.mr = nil
	m.busy = false
	m.locked = false
	m.errMsg = ""
	m.errLines = nil
	m.deleteBranch = false
	m.squash = false
}

func (m *MRActionModal) IsActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.active
}

func (m *MRActionModal) Kind() ModalKind {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.kind
}

func (m *MRActionModal) MR() *models.MergeRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.mr
}

func (m *MRActionModal) DeleteBranch() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.deleteBranch
}

func (m *MRActionModal) Squash() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.squash
}

func (m *MRActionModal) Busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.busy
}

// Locked returns true when the modal was opened via OpenGuarded and carries
// only a pre-seeded error — callers should treat Enter as a no-op.
func (m *MRActionModal) Locked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.locked
}

func (m *MRActionModal) ErrMsg() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.errMsg
}

// ToggleDeleteBranch flips the delete-source-branch checkbox. No-op for
// Close kind so the handler can be bound unconditionally.
func (m *MRActionModal) ToggleDeleteBranch() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.kind != ModalMerge {
		return
	}
	m.deleteBranch = !m.deleteBranch
}

// ToggleSquash flips the squash checkbox. No-op for Close kind.
func (m *MRActionModal) ToggleSquash() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.kind != ModalMerge {
		return
	}
	m.squash = !m.squash
}

// SetBusy guards double-Enter during an in-flight mutation.
func (m *MRActionModal) SetBusy(b bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.busy = b
	if b {
		m.errMsg = ""
		m.errLines = nil
	}
}

// SetErr records the last mutation failure. Kept across Busy transitions so
// the user can retry after reading the error. The wrapped representation is
// cached alongside the raw text so Render and layout read identical counts
// from a single Snapshot.
func (m *MRActionModal) SetErr(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.errMsg = msg
	m.errLines = wrapErrMsg(msg, modalInnerWidth)
	m.busy = false
}

// Snapshot returns a pure-data copy of the render-visible state. Used by
// the mutation handler to decide confirm-vs-cancel behaviour without
// re-entering the mutex for every field. ErrLines is shared (not copied) —
// the slice is only re-assigned in SetErr/Open*, never mutated in place, so
// readers can safely walk it off-lock.
func (m *MRActionModal) Snapshot() ModalSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	return ModalSnapshot{
		Active:       m.active,
		Kind:         m.kind,
		MR:           m.mr,
		DeleteBranch: m.deleteBranch,
		Squash:       m.squash,
		Busy:         m.busy,
		Locked:       m.locked,
		ErrMsg:       m.errMsg,
		ErrLines:     m.errLines,
	}
}

// Title returns the frame title for the current kind. Layout uses it to
// label the pane border.
func (m *MRActionModal) Title() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return TitleFor(m.kind)
}

// TitleFor is the pure lookup behind Title. Exposed so layout can read the
// title from a ModalSnapshot without taking the mutex a second time.
func TitleFor(kind ModalKind) string {
	if kind == ModalMerge {
		return modalMergeTitle
	}

	return modalCloseTitle
}

// Render paints the modal body into pane. The gocui view should have no
// internal padding; the widget writes its own inner padding.
// Render paints the modal body into pane. Convenience wrapper over
// RenderSnap that captures the snapshot itself — use RenderSnap directly
// when layout already has a snapshot in hand (saves one mutex round-trip
// and closes the single-frame race where layout sizes the pane from a
// stale err count that Render then exceeds).
func (m *MRActionModal) Render(pane *gocui.View) {
	if pane == nil {
		return
	}
	m.RenderSnap(pane, m.Snapshot())
}

// RenderSnap paints the body from an externally-captured snapshot. See
// Render's doc for the shared-snapshot invariant.
func (m *MRActionModal) RenderSnap(pane *gocui.View, snap ModalSnapshot) {
	if pane == nil {
		return
	}
	pane.Clear()
	if !snap.Active || snap.MR == nil {
		return
	}

	var sb strings.Builder
	sb.Grow(512) // covers close + typical merge without realloc
	sb.WriteString("\n")
	writeCenteredLine(&sb, fmt.Sprintf("%s !%d", theme.Wrap(theme.FgAccent, "MR"), snap.MR.IID)+"  "+snap.MR.Title)
	if snap.Kind == ModalMerge {
		sb.WriteString("\n")
		writeCenteredLine(&sb, snap.MR.SourceBranch+theme.Wrap(theme.FgDim, "  →  ")+snap.MR.TargetBranch)
		sb.WriteString("\n")
		writeToggleLine(&sb, "s", "Squash commits", snap.Squash)
		writeToggleLine(&sb, "d", "Delete source branch", snap.DeleteBranch)
	}

	sb.WriteString("\n")
	sb.WriteString(theme.Wrap(theme.FgDim, strings.Repeat("─", modalInnerWidth)))
	sb.WriteString("\n")

	for i, line := range snap.ErrLines {
		prefix := "   "
		if i == 0 {
			prefix = " ✕ "
		}
		sb.WriteString(theme.Wrap(theme.FgErr, prefix+line))
		sb.WriteString("\n")
	}

	if snap.Busy {
		sb.WriteString(theme.Wrap(theme.FgDim, "  Running…"))
		sb.WriteString("\n")
	}

	// Inline action buttons — mirrors design/wireframes/modals.js, giving
	// the user a pseudo-button pair at the bottom instead of a keybind
	// hint line. Always rendered so Busy/Err never leave the modal without
	// a footer cue. Source strings are cached at package init so the hot
	// path is a single WriteString.
	sb.WriteString("  ")
	sb.WriteString(modalActionButtons(snap.Kind))

	_, _ = fmt.Fprint(pane, sb.String())
}

// modalActionButtons returns the pre-rendered action-button line for kind.
// Layout mirrors `design/wireframes/modals.js:44-81,81` — primary action in
// accent, Cancel in dim. Toggle keybinds (`s` / `d`) are intentionally not
// surfaced here: the toggle rows already carry the letter next to the
// checkbox, and the global footer strip still lists them under the modal.
func modalActionButtons(kind ModalKind) string {
	if kind == ModalMerge {
		return modalButtonsMerge
	}

	return modalButtonsClose
}

var (
	modalButtonsClose = theme.Wrap(theme.FgAccent, "[ Close (Enter) ]") + "   " + theme.Wrap(theme.FgDim, "[ Cancel (Esc) ]")
	modalButtonsMerge = theme.Wrap(theme.FgAccent, "[ Merge (Enter) ]") + "   " + theme.Wrap(theme.FgDim, "[ Cancel (Esc) ]")
)

func writeCenteredLine(sb *strings.Builder, s string) {
	sb.WriteString("  ")
	sb.WriteString(s)
}

func writeToggleLine(sb *strings.Builder, key, label string, on bool) {
	mark := theme.Wrap(theme.FgDim, "[ ]")
	if on {
		mark = theme.Wrap(theme.FgAccent, "[x]")
	}
	fmt.Fprintf(sb, "  %s  %s  %s\n", mark, theme.Wrap(theme.FgAccent, key), label)
}

// wrapErrMsg breaks s into width-bounded lines. Width is measured in display
// cells via uniseg.StringWidth so CJK and emoji collapse to their true
// on-screen footprint. Whitespace-delimited tokens are kept intact when they
// fit; over-long tokens (typical: a URL the gitlab client wraps into the
// error) are hard-broken at a grapheme boundary so the pane border is never
// crossed. Control bytes (ESC/CSI, other C0 characters except \n and \t)
// are scrubbed first so an attacker-controlled upstream error cannot inject
// SGR escapes that corrupt the inline keybind strip or the dim/red theme.
func wrapErrMsg(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	s = sanitizeControlChars(s)
	if s == "" {
		return nil
	}

	var lines []string
	var cur strings.Builder
	curW := 0
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		lines = append(lines, cur.String())
		cur.Reset()
		curW = 0
	}

	for _, tok := range strings.Fields(s) {
		tw := uniseg.StringWidth(tok)
		if tw > width {
			flush()
			lines = appendHardBroken(lines, tok, width)

			continue
		}
		sep := 0
		if curW > 0 {
			sep = 1
		}
		if curW+sep+tw > width {
			flush()
			sep = 0
		}
		if sep == 1 {
			cur.WriteByte(' ')
			curW++
		}
		cur.WriteString(tok)
		curW += tw
	}
	flush()

	return lines
}

// appendHardBroken splits tok into width-bounded chunks along grapheme
// cluster boundaries and appends each chunk to lines. Called only for
// tokens that exceed width on their own — short tokens go through the
// word-wrap path instead.
func appendHardBroken(lines []string, tok string, width int) []string {
	var chunk strings.Builder
	chunkW := 0
	iter := uniseg.NewGraphemes(tok)
	for iter.Next() {
		g := iter.Str()
		gw := uniseg.StringWidth(g)
		if chunkW+gw > width && chunk.Len() > 0 {
			lines = append(lines, chunk.String())
			chunk.Reset()
			chunkW = 0
		}
		chunk.WriteString(g)
		chunkW += gw
	}
	if chunk.Len() > 0 {
		lines = append(lines, chunk.String())
	}

	return lines
}

// sanitizeControlChars drops ASCII control bytes from s so an upstream
// error payload cannot inject SGR escapes (`\x1b[...m`) that would hijack
// the modal's theme or the subsequent keybind strip. \n and \t are kept
// because the surrounding wrap loop treats them as whitespace; everything
// else in [0, 0x1f] plus DEL (0x7f) is stripped.
func sanitizeControlChars(s string) string {
	for i := 0; i < len(s); i++ {
		if isStrippableControl(s[i]) {
			return sanitizeControlCharsSlow(s)
		}
	}

	return s
}

func sanitizeControlCharsSlow(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if !isStrippableControl(s[i]) {
			b.WriteByte(s[i])
		}
	}

	return b.String()
}

func isStrippableControl(c byte) bool {
	if c == '\n' || c == '\t' {
		return false
	}

	return c < 0x20 || c == 0x7f
}
