package views

import (
	"strings"

	"github.com/niklod/lazylab/internal/tui/theme"
)

// keybindMode selects the hint set the global footer paints for the
// currently-focused pane / sub-pane / modal. Per-pane wireframes live in
// design/wireframes/{layout,repos,diff,pipeline,conversation,search,modals}.js.
type keybindMode int

const (
	keybindModeRepos keybindMode = iota
	keybindModeMRs
	keybindModeDetailOverview
	keybindModeDiffTree
	keybindModeDiffContent
	keybindModePipelineStages
	keybindModePipelineLog
	keybindModeConversation
	keybindModeSearch
	keybindModeModalClose
	keybindModeModalMerge
)

// keybindHint is one `<key> <label>` pair in the strip. Keys are painted in
// the accent colour, labels in dim. The separator between hints is a dim
// middle-dot to match the design bundle.
type keybindHint struct {
	keys  string
	label string
}

// renderKeybindStrip returns a single coloured line listing the keybindings
// active for mode. No Reset is appended to the tail — callers should add
// their own newline and, if needed, wrap with a Reset.
func renderKeybindStrip(mode keybindMode) string {
	return joinKeybindHints(hintsFor(mode))
}

// joinKeybindHints renders an arbitrary slice of hints using the same
// accent-key / dim-label styling the global footer strip uses. Exposed to
// the modal widget so the inline strip stays byte-identical to the footer.
func joinKeybindHints(hints []keybindHint) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		parts = append(parts,
			theme.FgAccent+h.keys+theme.Reset+" "+theme.FgDim+h.label+theme.Reset,
		)
	}

	return strings.Join(parts, theme.FgDim+" · "+theme.Reset)
}

// helpQuit is the common tail of every focus-pane hint set. Extracted so a
// future cross-mode addition (e.g. a global search key) lands in one place.
// Ephemeral overlays (search, modal) deliberately omit it — the user's
// field of action is narrower and `?`/`q` are not the next-relevant keys.
var helpQuit = []keybindHint{{"?", "help"}, {"q", "quit"}}

func hintsFor(mode keybindMode) []keybindHint {
	switch mode {
	case keybindModeRepos:
		return append([]keybindHint{
			{"h/l", "pane"}, {"j/k", "row"},
			{"/", "search"}, {"f", "fav"}, {"Enter", "open"},
		}, helpQuit...)
	case keybindModeMRs:
		return append([]keybindHint{
			{"h/l", "pane"}, {"j/k", "row"},
			{"s", "state"}, {"o", "owner"}, {"/", "search"},
			{"Enter", "open"}, {"x", "close"}, {"M", "merge"},
		}, helpQuit...)
	case keybindModeDetailOverview:
		return append([]keybindHint{
			{"h/l", "pane"}, {"[/]", "tab"},
		}, helpQuit...)
	case keybindModeDiffTree:
		return append([]keybindHint{
			{"h/l", "pane"}, {"j/k", "file"}, {"Enter", "select"},
			{"ctrl+d/u", "scroll"}, {"[/]", "tab"},
		}, helpQuit...)
	case keybindModeDiffContent:
		return append([]keybindHint{
			{"h/l", "pane"}, {"j/k", "scroll"},
			{"ctrl+d/u", "page"}, {"[/]", "tab"},
		}, helpQuit...)
	case keybindModePipelineStages:
		return append([]keybindHint{
			{"h/l", "pane"}, {"j/k", "job"}, {"Enter", "log"},
			{"r", "retry"}, {"R", "refresh"}, {"a", "auto"},
			{"o", "browser"}, {"[/]", "tab"},
		}, helpQuit...)
	case keybindModePipelineLog:
		return append([]keybindHint{
			{"j/k", "line"}, {"ctrl+d/u", "page"}, {"g/G", "top/bot"},
			{"y", "copy"}, {"r", "retry"}, {"Esc", "close"},
		}, helpQuit...)
	case keybindModeConversation:
		return append([]keybindHint{
			{"h/l", "pane"}, {"j/k", "thread"}, {"J/K", "note"},
			{"e", "expand"}, {"r", "resolve"}, {"c", "comment"},
			{"[/]", "tab"},
		}, helpQuit...)
	case keybindModeSearch:
		return []keybindHint{{"Enter", "apply"}, {"Esc", "cancel"}}
	case keybindModeModalClose:
		return []keybindHint{{"Enter", "confirm"}, {"Esc", "cancel"}}
	case keybindModeModalMerge:
		return []keybindHint{
			{"Enter", "confirm"}, {"Esc", "cancel"},
			{"d", "delete-branch"}, {"s", "squash"},
		}
	}

	return nil
}
