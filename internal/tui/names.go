package tui

const (
	ViewRepos  = "repos"
	ViewMRs    = "mrs"
	ViewDetail = "detail"
)

// focusOrder is the canonical focus-cycle order. Treat as immutable — tests and
// layout() depend on its identity and length.
var focusOrder = []string{ViewRepos, ViewMRs, ViewDetail}
