package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCycle_WrapsInBothDirections(t *testing.T) {
	t.Parallel()

	order := []string{ViewRepos, ViewMRs, ViewDetail}

	tests := []struct {
		name    string
		current string
		delta   int
		want    string
	}{
		{name: "repos next -> mrs", current: ViewRepos, delta: +1, want: ViewMRs},
		{name: "mrs next -> detail", current: ViewMRs, delta: +1, want: ViewDetail},
		{name: "detail next wraps -> repos", current: ViewDetail, delta: +1, want: ViewRepos},
		{name: "repos prev wraps -> detail", current: ViewRepos, delta: -1, want: ViewDetail},
		{name: "mrs prev -> repos", current: ViewMRs, delta: -1, want: ViewRepos},
		{name: "detail prev -> mrs", current: ViewDetail, delta: -1, want: ViewMRs},
		{name: "unknown + next -> repos", current: "unknown", delta: +1, want: ViewRepos},
		{name: "unknown + prev -> detail", current: "unknown", delta: -1, want: ViewDetail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, cycle(order, tt.current, tt.delta))
		})
	}
}

func TestCycle_EmptyOrder(t *testing.T) {
	t.Parallel()

	require.Empty(t, cycle(nil, ViewRepos, +1))
	require.Empty(t, cycle([]string{}, ViewRepos, -1))
}
