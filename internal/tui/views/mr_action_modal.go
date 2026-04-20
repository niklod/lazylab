package views

import (
	"fmt"
	"strings"
	"sync"

	"github.com/jesseduffield/gocui"

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
const (
	ModalCloseHeight = 8
	ModalMergeHeight = 14
	ModalWidth       = 60
)

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
	ErrMsg       string
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
	errMsg       string
}

func NewMRActionModal() *MRActionModal {
	return &MRActionModal{}
}

// Open activates the modal for the given kind + MR. Merge defaults mirror
// the wireframe: delete-branch on, squash off.
func (m *MRActionModal) Open(kind ModalKind, mr *models.MergeRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.active = true
	m.kind = kind
	m.mr = mr
	m.busy = false
	m.errMsg = ""
	if kind == ModalMerge {
		m.deleteBranch = true
		m.squash = false

		return
	}
	m.deleteBranch = false
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
	m.errMsg = ""
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
	}
}

// SetErr records the last mutation failure. Kept across Busy transitions so
// the user can retry after reading the error.
func (m *MRActionModal) SetErr(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.errMsg = msg
	m.busy = false
}

// Snapshot returns a pure-data copy of the render-visible state. Used by
// the mutation handler to decide confirm-vs-cancel behaviour without
// re-entering the mutex for every field.
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
		ErrMsg:       m.errMsg,
	}
}

// Title returns the frame title for the current kind. Layout uses it to
// label the pane border.
func (m *MRActionModal) Title() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.kind == ModalMerge {
		return modalMergeTitle
	}

	return modalCloseTitle
}

// Render paints the modal body into pane. The gocui view should have no
// internal padding; the widget writes its own inner padding.
func (m *MRActionModal) Render(pane *gocui.View) {
	if pane == nil {
		return
	}
	snap := m.Snapshot()
	pane.Clear()
	if !snap.Active || snap.MR == nil {
		return
	}

	var sb strings.Builder
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
	sb.WriteString(theme.Wrap(theme.FgDim, strings.Repeat("─", ModalWidth-4)))
	sb.WriteString("\n")

	if snap.ErrMsg != "" {
		sb.WriteString(theme.Wrap(theme.FgErr, " ✕ "+snap.ErrMsg))
		sb.WriteString("\n")
	}

	// Busy indicator stays local to the modal because the global footer
	// only reflects available keys, not in-flight mutation state. The
	// static keybind hints (Enter/Esc/d/s) live in the global footer — no
	// duplicate strip here.
	if snap.Busy {
		sb.WriteString(theme.Wrap(theme.FgDim, "  Running…"))
	}

	_, _ = fmt.Fprint(pane, sb.String())
}

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
