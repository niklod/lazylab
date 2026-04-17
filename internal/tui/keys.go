package tui

import (
	"fmt"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/tui/keymap"
)

// globalBindings are always registered regardless of which views exist.
// Per-view bindings live on the views themselves and are passed through Bind's
// variadic parameter so each view owns its own keymap.
var globalBindings = []keymap.Binding{
	{View: "", Key: 'q', Mod: gocui.ModNone, Handler: quit},
	{View: "", Key: gocui.KeyCtrlC, Mod: gocui.ModNone, Handler: quit},
	{View: "", Key: 'h', Mod: gocui.ModNone, Handler: focusPrev},
	{View: "", Key: 'l', Mod: gocui.ModNone, Handler: focusNext},
}

func quit(*gocui.Gui, *gocui.View) error {
	return gocui.ErrQuit
}

// Bind registers globalBindings plus any extra per-view bindings against g.
func Bind(g *gocui.Gui, extra ...keymap.Binding) error {
	all := make([]keymap.Binding, 0, len(globalBindings)+len(extra))
	all = append(all, globalBindings...)
	all = append(all, extra...)

	for _, b := range all {
		if err := g.SetKeybinding(b.View, b.Key, b.Mod, b.Handler); err != nil {
			return fmt.Errorf("bind %v on %q: %w", b.Key, b.View, err)
		}
	}

	return nil
}
