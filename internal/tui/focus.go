package tui

import (
	"fmt"

	"github.com/jesseduffield/gocui"
)

// cycle returns the name of the view delta steps from current in order, wrapping around.
// If current is not in order, it returns the first element for delta >= 0 and the last for delta < 0.
func cycle(order []string, current string, delta int) string {
	n := len(order)
	if n == 0 {
		return ""
	}

	idx := -1
	for i, name := range order {
		if name == current {
			idx = i

			break
		}
	}
	if idx < 0 {
		if delta >= 0 {
			return order[0]
		}

		return order[n-1]
	}

	next := ((idx+delta)%n + n) % n

	return order[next]
}

func focusNext(g *gocui.Gui, _ *gocui.View) error {
	return setFocus(g, +1)
}

func focusPrev(g *gocui.Gui, _ *gocui.View) error {
	return setFocus(g, -1)
}

func setFocus(g *gocui.Gui, delta int) error {
	next := cycle(focusOrder, currentViewName(g), delta)
	if _, err := g.SetCurrentView(next); err != nil {
		return fmt.Errorf("focus %q: %w", next, err)
	}

	return nil
}

// currentViewName returns the name of the focused view, or "" if none is focused.
func currentViewName(g *gocui.Gui) string {
	if v := g.CurrentView(); v != nil {
		return v.Name()
	}

	return ""
}
