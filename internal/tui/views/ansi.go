package views

import "github.com/niklod/lazylab/internal/tui/theme"

// ANSI SGR aliases that route through the theme palette. Historically the
// views package defined 16-color ANSI constants inline; these variables keep
// legacy call sites (diff_content, pipeline_log, pipeline_stages) compiling
// while redirecting the actual bytes emitted to the design-palette truecolor
// escapes owned by the theme package.
//
// New code should reference the theme tokens directly; these remain for
// files that have not yet been migrated.
var (
	ansiReset  = theme.Reset
	ansiBold   = theme.Bold
	ansiDim    = theme.Dim
	ansiRed    = theme.FgErr
	ansiGreen  = theme.FgOK
	ansiYellow = theme.FgWarn
	ansiCyan   = theme.FgInfo
)
