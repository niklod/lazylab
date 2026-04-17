package tui

import "github.com/niklod/lazylab/internal/tui/keymap"

// View names are re-exported from keymap so both tui and tui/views can reference
// them without creating an import cycle.
const (
	ViewRepos             = keymap.ViewRepos
	ViewMRs               = keymap.ViewMRs
	ViewDetail            = keymap.ViewDetail
	ViewDetailDiffTree    = keymap.ViewDetailDiffTree
	ViewDetailDiffContent = keymap.ViewDetailDiffContent
)

// focusOrder is the baseline focus-cycle order (Overview tab — detail pane is
// monolithic). Tests and layout() read it directly; runtime focus cycling goes
// through focusOrderFn so the Diff tab can inject its sub-panes.
var focusOrder = []string{ViewRepos, ViewMRs, ViewDetail}

// focusOrderFn returns the currently-active focus cycle. Swapped at startup
// by SetFocusOrderProvider so focusNext / focusPrev pick up the Diff-tab
// sub-panes without threading *views.Views through every handler closure.
var focusOrderFn = func() []string { return focusOrder }

// SetFocusOrderProvider installs a dynamic focus-order lookup. Passing nil
// restores the static baseline (useful in tests that tear down a Views).
func SetFocusOrderProvider(fn func() []string) {
	if fn == nil {
		focusOrderFn = func() []string { return focusOrder }

		return
	}
	focusOrderFn = fn
}

// detailFamily reports whether name belongs to the detail pane cluster. The
// parent detail frame highlights whenever any of its members is focused —
// otherwise switching tabs would visually "lose" the frame.
func detailFamily(name string) bool {
	switch name {
	case ViewDetail, ViewDetailDiffTree, ViewDetailDiffContent:
		return true
	}

	return false
}
