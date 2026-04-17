package tui

import (
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/require"
)

func TestPaneRects_StandardTerminal(t *testing.T) {
	t.Parallel()

	r := paneRects(120, 40)

	require.Equal(t, rect{0, 0, 48, 20}, r.repos)
	require.Equal(t, rect{0, 20, 48, 40}, r.mrs)
	require.Equal(t, rect{48, 0, 120, 40}, r.detail)
}

func TestPaneRects_SmallTerminal(t *testing.T) {
	t.Parallel()

	r := paneRects(80, 24)

	require.Equal(t, rect{0, 0, 32, 12}, r.repos)
	require.Equal(t, rect{0, 12, 32, 24}, r.mrs)
	require.Equal(t, rect{32, 0, 80, 24}, r.detail)
}

func TestPaneRects_LeftColumnsShareWidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxX, maxY int
	}{
		{name: "120x40", maxX: 120, maxY: 40},
		{name: "80x24", maxX: 80, maxY: 24},
		{name: "200x60", maxX: 200, maxY: 60},
		{name: "odd cols", maxX: 97, maxY: 41},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := paneRects(tt.maxX, tt.maxY)

			require.Equal(t, r.repos.x1, r.mrs.x1, "left column width must match between repos and mrs")
			require.Equal(t, r.repos.x1, r.detail.x0, "detail must start where left column ends")
			require.Equal(t, tt.maxX, r.detail.x1, "detail must reach the right edge")
			require.Equal(t, tt.maxY, r.mrs.y1, "mrs must reach the bottom edge")
			require.Equal(t, tt.maxY, r.detail.y1, "detail must reach the bottom edge")
			require.Equal(t, r.repos.y1, r.mrs.y0, "repos bottom must meet mrs top")
		})
	}
}

// TestLayout_TinyTerminalIsNoOp exercises the maxX < 2 / maxY < 2 guard.
// Build tag behaviour is different here: we construct a tiny gocui headless
// Gui, call layout(), and assert no view got created.
//
//nolint:paralleltest // gocui stores the tcell simulation screen in a package global.
func TestLayout_TinyTerminalIsNoOp(t *testing.T) {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 1, Height: 1})
	require.NoError(t, err)
	t.Cleanup(g.Close)

	require.NoError(t, layout(g))

	_, err = g.View(ViewRepos)
	require.Error(t, err, "no views should be created at sub-minimum size")
}

//nolint:paralleltest // gocui headless screen is shared; parallel access races.
func TestHighlightFocused_LightsDetailFrameForDetailFamily(t *testing.T) {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	require.NoError(t, err)
	t.Cleanup(g.Close)
	t.Cleanup(func() { SetFocusOrderProvider(nil) })

	require.NoError(t, layout(g))

	SetFocusOrderProvider(func() []string {
		return []string{ViewRepos, ViewMRs, ViewDetailDiffTree, ViewDetailDiffContent}
	})
	_, err = g.SetView(ViewDetailDiffTree, 50, 1, 80, 30, 0)
	if err != nil && err.Error() != "unknown view" {
		require.NoError(t, err)
	}
	_, err = g.SetCurrentView(ViewDetailDiffTree)
	require.NoError(t, err)

	highlightFocused(g)

	detail, err := g.View(ViewDetail)
	require.NoError(t, err)
	require.Equal(t, gocui.ColorGreen, detail.FrameColor,
		"detail frame must stay green while a sub-pane is focused")

	repos, err := g.View(ViewRepos)
	require.NoError(t, err)
	require.Equal(t, gocui.ColorDefault, repos.FrameColor,
		"non-focused panes stay default colour")
}

//nolint:paralleltest // focusOrderFn is a package global.
func TestSetFocusOrderProvider_NilRestoresBaseline(t *testing.T) {
	t.Cleanup(func() { SetFocusOrderProvider(nil) })

	SetFocusOrderProvider(func() []string { return []string{"x", "y"} })
	require.Equal(t, []string{"x", "y"}, focusOrderFn())

	SetFocusOrderProvider(nil)
	require.Equal(t, focusOrder, focusOrderFn())
}
