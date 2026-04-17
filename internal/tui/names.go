package tui

import "github.com/niklod/lazylab/internal/tui/keymap"

// View names are re-exported from keymap so both tui and tui/views can reference
// them without creating an import cycle.
const (
	ViewRepos  = keymap.ViewRepos
	ViewMRs    = keymap.ViewMRs
	ViewDetail = keymap.ViewDetail
)

// focusOrder is the canonical focus-cycle order. Treat as immutable — tests and
// layout() depend on its identity and length.
var focusOrder = []string{ViewRepos, ViewMRs, ViewDetail}
