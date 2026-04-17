package cache_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/cache"
)

type KeySuite struct {
	suite.Suite
}

func (s *KeySuite) TestMakeKey_Cases() {
	tests := []struct {
		name      string
		namespace string
		args      []any
		want      string
	}{
		{name: "namespace only", namespace: "projects", args: nil, want: "projects"},
		{name: "single int arg", namespace: "project", args: []any{42}, want: "project:42"},
		{name: "multiple args", namespace: "mr", args: []any{10, 99}, want: "mr:10:99"},
		{name: "nil skipped mid", namespace: "mr_list", args: []any{10, nil, "opened"}, want: "mr_list:10:opened"},
		{name: "nil skipped all", namespace: "ping", args: []any{nil, nil}, want: "ping"},
		{name: "bool stringified", namespace: "flag", args: []any{true}, want: "flag:true"},
		{name: "string arg", namespace: "project_path", args: []any{"group/demo"}, want: "project_path:group/demo"},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got := cache.MakeKey(tt.namespace, tt.args...)
			s.Require().Equal(tt.want, got)
		})
	}
}

func TestKeySuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(KeySuite))
}
