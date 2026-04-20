package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/tui/keymap"
	"github.com/niklod/lazylab/internal/tui/theme"
)

// FooterState captures every input the footer needs to paint its two lines.
// Snapshotted each tick by the layout package so Render is pure — no I/O,
// no mutex traversal, no cross-view lookups.
type FooterState struct {
	FocusedView  string
	RepoPath     string
	MRIID        int
	MRIndex      int
	MRTotal      int
	LastSync     time.Time
	Now          time.Time
	SearchActive bool
	ModalActive  bool
	ModalKind    ModalKind
}

// FooterView renders the global two-line bottom strip described in
// design/wireframes/layout.js:60-62 — a dim breadcrumb + last-sync meta
// line above a context-sensitive keybind hint line. Driven each tick by
// layout.manageFooter; no internal state to mutate.
type FooterView struct{}

func NewFooter() *FooterView { return &FooterView{} }

// Render paints the meta + keybind lines into pane. Safe to call from the
// layout goroutine (the only goroutine) without locking — FooterView is
// stateless and all inputs arrive via st.
func (f *FooterView) Render(pane *gocui.View, st FooterState) {
	if pane == nil {
		return
	}
	pane.Clear()

	pane.WriteString(metaLine(st) + "\n")
	pane.WriteString(renderKeybindStrip(keybindModeFor(st)))
}

// metaLine composes line 1: `lazylab · <repo> › !<iid> · <idx>/<total> · last sync <ago>`.
// Each segment is optional — missing fields drop out cleanly so early-boot
// states (no repo, no MR, never synced) render a minimal `lazylab` token.
func metaLine(st FooterState) string {
	var sb strings.Builder
	sb.WriteString(theme.FgDim)
	sb.WriteString("lazylab")

	if st.RepoPath != "" {
		sb.WriteString(" · ")
		writeAccent(&sb, st.RepoPath)
	}

	if st.MRIID > 0 {
		sb.WriteString(" › ")
		writeAccent(&sb, fmt.Sprintf("!%d", st.MRIID))

		if st.MRIndex > 0 && st.MRTotal > 0 {
			fmt.Fprintf(&sb, " · %d/%d", st.MRIndex, st.MRTotal)
		}
	}

	if !st.LastSync.IsZero() {
		now := st.Now
		if now.IsZero() {
			now = time.Now()
		}
		sb.WriteString(" · last sync ")
		sb.WriteString(theme.Relative(st.LastSync, now))
	}

	sb.WriteString(theme.Reset)

	return sb.String()
}

// writeAccent emits s in the accent colour and returns to the dim tone
// the meta line uses for its scaffolding. Saves five identical Reset-Accent
// Reset-Dim dances in metaLine.
func writeAccent(sb *strings.Builder, s string) {
	sb.WriteString(theme.Reset + theme.FgAccent)
	sb.WriteString(s)
	sb.WriteString(theme.Reset + theme.FgDim)
}

// keybindModeFor picks the hint set that matches the current focus +
// modal/search overlay. Higher-priority overlays (search input, modal) win
// over pane focus — consistent with the wireframe's narrative of "the
// strip reflects what the user can press next".
func keybindModeFor(st FooterState) keybindMode {
	if st.SearchActive {
		return keybindModeSearch
	}
	if st.ModalActive {
		if st.ModalKind == ModalMerge {
			return keybindModeModalMerge
		}

		return keybindModeModalClose
	}

	switch st.FocusedView {
	case keymap.ViewRepos:
		return keybindModeRepos
	case keymap.ViewMRs:
		return keybindModeMRs
	case keymap.ViewDetailDiffTree:
		return keybindModeDiffTree
	case keymap.ViewDetailDiffContent:
		return keybindModeDiffContent
	case keymap.ViewDetailPipelineStages:
		return keybindModePipelineStages
	case keymap.ViewDetailPipelineJobLog:
		return keybindModePipelineLog
	case keymap.ViewDetailConversation:
		return keybindModeConversation
	}

	return keybindModeDetailOverview
}
