// Package keymap defines the shared keybinding type and pane-name constants
// used by both the tui package and its views subpackage. Lives in its own
// leaf package so views can contribute Bindings without importing tui (which
// would create a cycle, since tui imports views to wire them at startup).
package keymap

import "github.com/jesseduffield/gocui"

const (
	ViewRepos       = "repos"
	ViewMRs         = "mrs"
	ViewDetail      = "detail"
	ViewReposSearch = "repos_search"
)

// HandlerFunc matches gocui's keybinding handler signature.
type HandlerFunc func(*gocui.Gui, *gocui.View) error

// Binding is a single keymap entry. View "" means global.
type Binding struct {
	View    string
	Key     any
	Mod     gocui.Modifier
	Handler HandlerFunc
}
