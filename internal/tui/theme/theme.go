// Package theme centralises the LazyLab color palette so every view consumes
// the same semantic tokens. The palette is sourced from
// design/project/wireframes/_helpers.js — RGB values must stay in sync with
// the design bundle.
//
// Two flavours of colour are exposed: ANSI SGR strings (for inline text
// coloring inside view buffers; gocui parses 24-bit escapes when the Gui is
// built with OutputTrue) and gocui.Attribute values (for tcell-side frame
// and selection colouring). tcell downgrades RGB to 256/16-color on
// terminals that don't support truecolor, so no manual fallback is needed.
package theme

import (
	"fmt"

	"github.com/jesseduffield/gocui"
)

// ANSI SGR control sequences used across the UI.
const (
	Reset = "\x1b[0m"
	Bold  = "\x1b[1m"
	Dim   = "\x1b[2m"
)

// rgbFg returns a 24-bit ANSI SGR foreground escape for (r,g,b).
func rgbFg(r, g, b uint8) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

// Semantic foreground colors — inline text variants. Names mirror the
// design palette roles so call sites read like intent, not pigment.
var (
	FgOK     = rgbFg(74, 168, 90)   // #4aa85a — passed, approvals met, no conflicts, added
	FgWarn   = rgbFg(217, 164, 65)  // #d9a441 — running/pending, unresolved
	FgErr    = rgbFg(204, 80, 64)   // #cc5040 — failed, conflicts, removed
	FgInfo   = rgbFg(90, 140, 200)  // #5a8cc8 — hunk headers, manual/scheduled
	FgAccent = rgbFg(217, 119, 87)  // #d97757 — mentions, active tab, keybinds
	FgMerged = rgbFg(138, 92, 200)  // #8a5cc8 — merged state
	FgDraft  = rgbFg(138, 133, 123) // #8a857b — draft/WIP, canceled, skipped
)

// Attribute mirrors of the accent color for tcell-side use (panel frames,
// selection background/foreground). Additional attributes can be added here
// as the UI grows.
var (
	ColorAccent = gocui.NewRGBColor(217, 119, 87)
)

// Wrap returns text wrapped in color...Reset. Cheap helper that beats
// concatenating at every call site.
func Wrap(color, text string) string {
	return color + text + Reset
}

// Dot returns a coloured filled-circle glyph (●) used by status rows. The
// Reset at the end keeps colour from bleeding into the rest of the line.
func Dot(color string) string {
	return color + "\u25CF" + Reset
}
