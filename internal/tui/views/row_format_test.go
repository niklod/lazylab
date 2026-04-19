package views

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/niklod/lazylab/internal/tui/theme"
)

func TestFormatRepoRow_PadsSoAgoEndsAtPaneWidth(t *testing.T) {
	t.Parallel()

	got := formatRepoRow("  ", 2, "grp/alpha", "2m ago", 30)

	require.Equal(t, 30, visibleWidth(stripANSI(got)),
		"row visible width must equal paneWidth so ago right-aligns")
	require.Contains(t, got, "grp/alpha")
	require.True(t, strings.HasSuffix(stripANSI(got), "2m ago"),
		"ago is the last visible token")
	require.Contains(t, got, theme.FgDim+"2m ago"+theme.Reset, "ago wrapped in FgDim")
}

func TestFormatRepoRow_LongPath_TruncatesWithEllipsisAndKeepsAgoAligned(t *testing.T) {
	t.Parallel()

	got := formatRepoRow("★", 1, "very/long/namespace/that-overflows", "1h ago", 25)

	plain := stripANSI(got)
	require.Equal(t, 25, visibleWidth(plain), "row width still equals paneWidth after truncation")
	require.Contains(t, plain, "…", "long path is truncated with an ellipsis")
	require.True(t, strings.HasSuffix(plain, "1h ago"), "ago survives the truncation")
}

func TestFormatRepoRow_HeadlessPaneFallback_NoPanic(t *testing.T) {
	t.Parallel()

	got := formatRepoRow("  ", 2, "grp/x", "1d ago", 0)

	require.Contains(t, got, "grp/x")
	require.Contains(t, got, "1d ago")
}

func TestFormatRepoRow_VeryNarrowPane_DropsAgo(t *testing.T) {
	t.Parallel()

	got := formatRepoRow("★", 1, "grp/alpha", "5m ago", 8)

	require.NotContains(t, got, "5m ago", "ago dropped when pane has no room")
	require.Contains(t, got, "★")
}

func TestFormatMRRow_PadsSoAuthorEndsAtPaneWidth(t *testing.T) {
	t.Parallel()

	got := formatMRRow("●", 482, "Add retry button", "mira", 40)

	plain := stripANSI(got)
	require.Equal(t, 40, visibleWidth(plain))
	require.True(t, strings.HasSuffix(plain, "@mira"), "@author is the last visible token")
	require.Contains(t, got, theme.FgDim+"@mira"+theme.Reset, "author wrapped in FgDim")
	require.Contains(t, plain, "!482 Add retry button")
}

func TestFormatMRRow_LongTitle_TruncatesAndKeepsAuthorAligned(t *testing.T) {
	t.Parallel()

	got := formatMRRow("◐", 7, "Replace the entire pipeline machinery with something faster", "jay", 35)

	plain := stripANSI(got)
	require.Equal(t, 35, visibleWidth(plain))
	require.Contains(t, plain, "…", "long title truncated with ellipsis")
	require.True(t, strings.HasSuffix(plain, "@jay"))
}

func TestFormatMRRow_HeadlessPaneFallback_NoPanic(t *testing.T) {
	t.Parallel()

	got := formatMRRow("✓", 1, "x", "a", 0)

	require.Contains(t, got, "!1")
	require.Contains(t, got, "@a")
}

func TestTruncate_GraphemeClusters_NotSplit(t *testing.T) {
	t.Parallel()

	family := "\U0001F468\u200D\U0001F469\u200D\U0001F467" // 👨‍👩‍👧
	s := "x" + family + "yz"

	got := truncate(s, 4)

	require.NotContains(t, got, "\u200D"+ellipsis, "ellipsis must not glue to ZWJ joiner")
	require.True(t, strings.HasSuffix(got, ellipsis))
}

func TestVisibleWidth_AsciiAndCJK(t *testing.T) {
	t.Parallel()

	require.Equal(t, 5, visibleWidth("hello"))
	require.Equal(t, 4, visibleWidth("日本"), "CJK chars are 2 cells each")
	require.Equal(t, 1, visibleWidth("★"))
}

// stripANSI removes SGR escape sequences so visible-width / column assertions
// can run against the rendered glyph stream. Keep this test-local — production
// code never strips its own colour escapes.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1

				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}

	return b.String()
}
