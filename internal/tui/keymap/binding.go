// Package keymap defines the shared keybinding type and pane-name constants
// used by both the tui package and its views subpackage. Lives in its own
// leaf package so views can contribute Bindings without importing tui (which
// would create a cycle, since tui imports views to wire them at startup).
package keymap

import "github.com/jesseduffield/gocui"

const (
	ViewRepos                = "repos"
	ViewMRs                  = "mrs"
	ViewDetail               = "detail"
	ViewReposSearch          = "repos_search"
	ViewMRsSearch            = "mrs_search"
	ViewDetailDiffTree       = "detail_diff_tree"
	ViewDetailDiffContent    = "detail_diff_content"
	ViewDetailPipelineStages = "detail_pipeline_stages"
	ViewDetailPipelineJobLog = "detail_pipeline_job_log"
	ViewDetailConversation   = "detail_conversation"
	ViewMRActionsModal       = "mr_actions_modal"
	ViewFooter               = "footer"
)

// detailFamily is the authoritative list of pane names that belong to the
// detail cluster (Overview + Diff tree/content + Conversation + Pipeline
// stages/log). Kept unexported so callers cannot mutate the slice; use
// IsDetailFamily for membership and DetailFamily for iteration over a
// defensive copy.
var detailFamily = []string{
	ViewDetail,
	ViewDetailDiffTree, ViewDetailDiffContent,
	ViewDetailPipelineStages, ViewDetailPipelineJobLog,
	ViewDetailConversation,
}

// DetailFamily returns a fresh copy of the detail-family pane names. Binding
// registration iterates this to emit per-view shortcuts (`[`/`]`, `x`/`M`).
func DetailFamily() []string {
	out := make([]string, len(detailFamily))
	copy(out, detailFamily)

	return out
}

// IsDetailFamily reports whether name belongs to the detail pane cluster.
// Used by layout's frame-focus rule and by the MR-action modal's origin
// check.
func IsDetailFamily(name string) bool {
	for _, v := range detailFamily {
		if v == name {
			return true
		}
	}

	return false
}

// HandlerFunc matches gocui's keybinding handler signature.
type HandlerFunc func(*gocui.Gui, *gocui.View) error

// Binding is a single keymap entry. View "" means global.
type Binding struct {
	View    string
	Key     any
	Mod     gocui.Modifier
	Handler HandlerFunc
}
