package tui

import (
	"fmt"

	"github.com/jesseduffield/gocui"
)

type handlerFunc func(*gocui.Gui, *gocui.View) error

type binding struct {
	view    string
	key     any
	mod     gocui.Modifier
	handler handlerFunc
}

// bindings is the full keymap registered by Bind. Per-view entries for
// j/k/g/G// and [/] use noop handlers until the widgets that own those
// behaviours land in later Phase G2/G4 tasks; the DoD only requires the
// dispatch path be wired.
var bindings = []binding{
	{view: "", key: 'q', mod: gocui.ModNone, handler: quit},
	{view: "", key: gocui.KeyCtrlC, mod: gocui.ModNone, handler: quit},
	{view: "", key: 'h', mod: gocui.ModNone, handler: focusPrev},
	{view: "", key: 'l', mod: gocui.ModNone, handler: focusNext},

	{view: ViewRepos, key: 'j', mod: gocui.ModNone, handler: noop},
	{view: ViewRepos, key: 'k', mod: gocui.ModNone, handler: noop},
	{view: ViewRepos, key: 'g', mod: gocui.ModNone, handler: noop},
	{view: ViewRepos, key: 'G', mod: gocui.ModNone, handler: noop},
	{view: ViewRepos, key: '/', mod: gocui.ModNone, handler: noop},

	{view: ViewMRs, key: 'j', mod: gocui.ModNone, handler: noop},
	{view: ViewMRs, key: 'k', mod: gocui.ModNone, handler: noop},
	{view: ViewMRs, key: 'g', mod: gocui.ModNone, handler: noop},
	{view: ViewMRs, key: 'G', mod: gocui.ModNone, handler: noop},
	{view: ViewMRs, key: '/', mod: gocui.ModNone, handler: noop},

	{view: ViewDetail, key: 'j', mod: gocui.ModNone, handler: noop},
	{view: ViewDetail, key: 'k', mod: gocui.ModNone, handler: noop},
	{view: ViewDetail, key: 'g', mod: gocui.ModNone, handler: noop},
	{view: ViewDetail, key: 'G', mod: gocui.ModNone, handler: noop},
	{view: ViewDetail, key: '/', mod: gocui.ModNone, handler: noop},
	{view: ViewDetail, key: '[', mod: gocui.ModNone, handler: noop},
	{view: ViewDetail, key: ']', mod: gocui.ModNone, handler: noop},
}

func quit(*gocui.Gui, *gocui.View) error {
	return gocui.ErrQuit
}

func noop(*gocui.Gui, *gocui.View) error { return nil }

// Bind registers every keybinding in bindings against g.
func Bind(g *gocui.Gui) error {
	for _, b := range bindings {
		if err := g.SetKeybinding(b.view, b.key, b.mod, b.handler); err != nil {
			return fmt.Errorf("bind %v on %q: %w", b.key, b.view, err)
		}
	}

	return nil
}
