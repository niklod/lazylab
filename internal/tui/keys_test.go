package tui

import (
	"reflect"
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/require"

	"github.com/niklod/lazylab/internal/tui/keymap"
)

func TestGlobalBindings_RequiredKeysRegistered(t *testing.T) {
	t.Parallel()

	index := make(map[any]bool, len(globalBindings))
	for _, b := range globalBindings {
		require.Empty(t, b.View, "only global bindings live in globalBindings")
		index[b.Key] = true
	}

	for _, key := range []any{'q', gocui.KeyCtrlC, 'h', 'l'} {
		require.True(t, index[key], "global binding %v must be registered", key)
	}
}

func TestGlobalBindings_EveryHandlerSet(t *testing.T) {
	t.Parallel()

	for _, b := range globalBindings {
		require.NotNil(t, b.Handler, "binding %q/%v has nil handler", b.View, b.Key)
	}
}

func TestGlobalBindings_FocusHandlersNotSwapped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		key       any
		wantFnPtr uintptr
	}{
		{name: "l -> focusNext", key: 'l', wantFnPtr: reflect.ValueOf(focusNext).Pointer()},
		{name: "h -> focusPrev", key: 'h', wantFnPtr: reflect.ValueOf(focusPrev).Pointer()},
		{name: "q -> quit", key: 'q', wantFnPtr: reflect.ValueOf(quit).Pointer()},
		{name: "Ctrl+C -> quit", key: gocui.KeyCtrlC, wantFnPtr: reflect.ValueOf(quit).Pointer()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got keymap.HandlerFunc
			for _, b := range globalBindings {
				if b.Key == tt.key {
					got = b.Handler

					break
				}
			}
			require.NotNil(t, got, "binding %v not found", tt.key)
			require.Equal(t, tt.wantFnPtr, reflect.ValueOf(got).Pointer())
		})
	}
}
