package theme

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFgOK_IsValidTruecolorSGR(t *testing.T) {
	t.Parallel()

	require.True(t, strings.HasPrefix(FgOK, "\x1b[38;2;"), "FgOK must be a 24-bit SGR escape")
	require.True(t, strings.HasSuffix(FgOK, "m"))
}

func TestWrap_RoundTrips(t *testing.T) {
	t.Parallel()

	got := Wrap(FgOK, "ok")

	require.Equal(t, FgOK+"ok"+Reset, got)
}

func TestDot_IsColoredBullet(t *testing.T) {
	t.Parallel()

	got := Dot(FgErr)

	require.Equal(t, FgErr+"\u25CF"+Reset, got)
}

func TestPalette_UniqueColors(t *testing.T) {
	t.Parallel()

	palette := map[string]string{
		"ok":     FgOK,
		"warn":   FgWarn,
		"err":    FgErr,
		"info":   FgInfo,
		"accent": FgAccent,
		"merged": FgMerged,
		"draft":  FgDraft,
		"dim":    FgDim,
	}

	seen := make(map[string]string, len(palette))
	for name, code := range palette {
		require.NotEmpty(t, code, "%s must be non-empty", name)
		if prev, dup := seen[code]; dup {
			t.Fatalf("%s duplicates %s (%q)", name, prev, code)
		}
		seen[code] = name
	}
}
