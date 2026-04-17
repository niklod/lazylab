package tui

import (
	"reflect"
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/require"
)

func TestBindings_RequiredKeysRegistered(t *testing.T) {
	t.Parallel()

	index := make(map[string]map[any]bool, len(bindings))
	for _, b := range bindings {
		if index[b.view] == nil {
			index[b.view] = map[any]bool{}
		}
		index[b.view][b.key] = true
	}

	tests := []struct {
		name    string
		view    string
		key     any
		present bool
	}{
		{name: "global q", view: "", key: 'q', present: true},
		{name: "global Ctrl+C", view: "", key: gocui.KeyCtrlC, present: true},
		{name: "global h", view: "", key: 'h', present: true},
		{name: "global l", view: "", key: 'l', present: true},
		{name: "repos j", view: ViewRepos, key: 'j', present: true},
		{name: "repos k", view: ViewRepos, key: 'k', present: true},
		{name: "repos g", view: ViewRepos, key: 'g', present: true},
		{name: "repos G", view: ViewRepos, key: 'G', present: true},
		{name: "repos /", view: ViewRepos, key: '/', present: true},
		{name: "mrs j", view: ViewMRs, key: 'j', present: true},
		{name: "mrs k", view: ViewMRs, key: 'k', present: true},
		{name: "mrs g", view: ViewMRs, key: 'g', present: true},
		{name: "mrs G", view: ViewMRs, key: 'G', present: true},
		{name: "mrs /", view: ViewMRs, key: '/', present: true},
		{name: "detail j", view: ViewDetail, key: 'j', present: true},
		{name: "detail k", view: ViewDetail, key: 'k', present: true},
		{name: "detail g", view: ViewDetail, key: 'g', present: true},
		{name: "detail G", view: ViewDetail, key: 'G', present: true},
		{name: "detail /", view: ViewDetail, key: '/', present: true},
		{name: "detail [", view: ViewDetail, key: '[', present: true},
		{name: "detail ]", view: ViewDetail, key: ']', present: true},
		{name: "repos has no [", view: ViewRepos, key: '[', present: false},
		{name: "repos has no ]", view: ViewRepos, key: ']', present: false},
		{name: "mrs has no [", view: ViewMRs, key: '[', present: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.present, index[tt.view][tt.key])
		})
	}
}

func TestBindings_EveryHandlerSet(t *testing.T) {
	t.Parallel()

	for _, b := range bindings {
		require.NotNil(t, b.handler, "binding %q/%v has nil handler", b.view, b.key)
	}
}

func TestBindings_FocusHandlersNotSwapped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		view      string
		key       any
		wantFnPtr uintptr
	}{
		{name: "l -> focusNext", view: "", key: 'l', wantFnPtr: reflect.ValueOf(focusNext).Pointer()},
		{name: "h -> focusPrev", view: "", key: 'h', wantFnPtr: reflect.ValueOf(focusPrev).Pointer()},
		{name: "q -> quit", view: "", key: 'q', wantFnPtr: reflect.ValueOf(quit).Pointer()},
		{name: "Ctrl+C -> quit", view: "", key: gocui.KeyCtrlC, wantFnPtr: reflect.ValueOf(quit).Pointer()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got handlerFunc
			for _, b := range bindings {
				if b.view == tt.view && b.key == tt.key {
					got = b.handler

					break
				}
			}
			require.NotNil(t, got, "binding %q/%v not found", tt.view, tt.key)
			require.Equal(t, tt.wantFnPtr, reflect.ValueOf(got).Pointer())
		})
	}
}
