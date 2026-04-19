package views

import (
	"strings"

	"github.com/niklod/lazylab/internal/tui/theme"
)

// keybindMode selects which set of hints the Pipeline tab renders at the
// bottom of the pane. The wireframe (design/project/wireframes/pipeline.js:27,
// :58) specifies two variants — one for the stages list, one for the inline
// log viewer.
type keybindMode int

const (
	keybindModePipelineStages keybindMode = iota
	keybindModePipelineLog
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
	var hints []keybindHint
	switch mode {
	case keybindModePipelineStages:
		hints = []keybindHint{
			{"j/k", "job"},
			{"Enter", "open log"},
			{"r", "retry"},
			{"o", "open"},
			{"R", "refresh"},
			{"a", "toggle auto-refresh"},
		}
	case keybindModePipelineLog:
		hints = []keybindHint{
			{"j/k", "line"},
			{"ctrl+d/u", "page"},
			{"y", "copy"},
			{"r", "retry"},
			{"Esc", "close"},
		}
	}

	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		parts = append(parts,
			theme.FgAccent+h.keys+theme.Reset+" "+theme.FgDim+h.label+theme.Reset,
		)
	}

	return strings.Join(parts, theme.FgDim+" · "+theme.Reset)
}
