package views

import (
	"strconv"
	"strings"

	"github.com/rivo/uniseg"

	"github.com/niklod/lazylab/internal/tui/theme"
)

// dim and accent are file-local shortcuts for the two most common colour
// wrappers in the list-pane render paths. They cut visual noise where
// empty-state writers concatenate the same wrapper 4–5 times per line.
func dim(s string) string    { return theme.Wrap(theme.FgDim, s) }
func accent(s string) string { return theme.Wrap(theme.FgAccent, s) }

// Column padding constants. Two spaces between left content and right tail
// keep the eye scanning the right column without it looking glued on.
const (
	rowGutter = 2
	ellipsis  = "…"
)

// formatRepoRow renders one Repositories pane row:
//
//	<icon> <path-with-namespace>           <ago>
//
// `icon` is rendered verbatim (already coloured by caller) and its cell width
// is supplied in `iconWidth` so the function never has to strip SGR from a
// coloured string. `path` left, `ago` right, separated so `ago` is right-
// aligned at paneWidth and dim. Long paths truncate with an ellipsis.
// paneWidth ≤ 0 falls back to an un-aligned form for headless test panes.
func formatRepoRow(icon string, iconWidth int, path, ago string, paneWidth int) string {
	leadingWidth := iconWidth + 1 // icon + single space
	agoWidth := uniseg.StringWidth(ago)
	rightSegment := dim(ago)

	if paneWidth <= 0 {
		return icon + " " + path + "  " + rightSegment
	}

	pathBudget := paneWidth - leadingWidth - rowGutter - agoWidth
	if pathBudget < 1 {
		// Pane too narrow for the right column — drop ago and let the path
		// take everything that's left.
		pathBudget = paneWidth - leadingWidth
		if pathBudget < 1 {
			return icon
		}
		path = truncate(path, pathBudget)

		return icon + " " + path
	}

	path = truncate(path, pathBudget)
	pathPadding := paneWidth - leadingWidth - uniseg.StringWidth(path) - agoWidth

	return icon + " " + path + strings.Repeat(" ", pathPadding) + rightSegment
}

// formatMRRow renders one Merge Requests pane row:
//
//	<icon> !<IID> <title>            @<author>
//
// `icon` is already coloured by the caller; the design palette uses single-
// cell glyphs (●/◐/✓/✕) for every MR state, so the icon's cell width is
// hard-coded to 1 here. `@author` is dim and right-aligned. paneWidth ≤ 0
// falls back to the un-aligned form for headless test panes.
func formatMRRow(icon string, iid int, title, author string, paneWidth int) string {
	const iconWidth = 1
	idPart := "!" + strconv.Itoa(iid) + " "
	authorPart := "@" + author

	prefixWidth := iconWidth + 1 + len(idPart) // idPart is ASCII
	authorWidth := uniseg.StringWidth(authorPart)
	authorWrapped := dim(authorPart)

	if paneWidth <= 0 {
		return icon + " " + idPart + title + "  " + authorWrapped
	}

	titleBudget := paneWidth - prefixWidth - rowGutter - authorWidth
	if titleBudget < 1 {
		titleBudget = paneWidth - prefixWidth
		if titleBudget < 1 {
			return icon + " " + idPart
		}
		title = truncate(title, titleBudget)

		return icon + " " + idPart + title
	}

	title = truncate(title, titleBudget)
	pad := paneWidth - prefixWidth - uniseg.StringWidth(title) - authorWidth

	return icon + " " + idPart + title + strings.Repeat(" ", pad) + authorWrapped
}

// truncate returns s shortened so its visible width is at most maxWidth,
// appending an ellipsis when truncation occurs. Pure-ASCII inputs (the common
// case for repo paths and MR titles) take a byte-length fast path; multibyte
// or grapheme-cluster strings go through the uniseg walker.
func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if isASCII(s) {
		if len(s) <= maxWidth {
			return s
		}
		if maxWidth == 1 {
			return ellipsis
		}

		return s[:maxWidth-1] + ellipsis
	}
	if uniseg.StringWidth(s) <= maxWidth {
		return s
	}
	if maxWidth == 1 {
		return ellipsis
	}

	limit := maxWidth - 1
	var b strings.Builder
	b.Grow(len(s))
	width := 0
	gr := uniseg.NewGraphemes(s)
	for gr.Next() {
		w := gr.Width()
		if w == 0 {
			b.WriteString(gr.Str())

			continue
		}
		if width+w > limit {
			break
		}
		b.WriteString(gr.Str())
		width += w
	}
	b.WriteString(ellipsis)

	return b.String()
}

// isASCII reports whether s contains only 7-bit bytes — true for repo paths
// and MR titles in the overwhelming majority of cases, which lets truncate
// skip the grapheme-cluster walk.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}

	return true
}

// visibleWidth returns the on-screen cell width of s, ignoring zero-width
// graphemes and SGR escape sequences. Primarily used by tests; hot-path
// callers pass widths explicitly to avoid escape-stripping on every render.
func visibleWidth(s string) int {
	return uniseg.StringWidth(stripSGR(s))
}

// stripSGR removes CSI SGR escape sequences (`ESC [ ... m`) so width math
// counts only printable cells. Other CSI sequences are left alone — none of
// the colour helpers here emit them, but if a future callsite does, this
// helper won't silently swallow cursor/erase escapes.
func stripSGR(s string) string {
	if !strings.ContainsRune(s, 0x1b) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' && (s[j] == ';' || (s[j] >= '0' && s[j] <= '9')) {
				j++
			}
			if j < len(s) && s[j] == 'm' {
				i = j + 1

				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}

	return b.String()
}
