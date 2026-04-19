// Package browser opens URLs in the user's default browser. Hides the
// pkg/browser dependency behind a thin indirection so tests can swap in a
// fake without exercising the real OS handler.
package browser

import (
	"fmt"

	pkgbrowser "github.com/pkg/browser"
)

// OpenFunc matches the signature of browser.OpenURL and is the extension
// point tests use to capture the URL without spawning a real browser.
type OpenFunc func(url string) error

// open is the active opener. Production wiring uses pkg/browser; tests
// replace this to assert the URL.
var open OpenFunc = pkgbrowser.OpenURL

// SetOpenFunc replaces the default opener. Returns a restore closure so test
// cleanups can deterministically roll back. Safe to call from multiple tests
// sequentially; do not call from concurrent tests.
func SetOpenFunc(fn OpenFunc) func() {
	prev := open
	open = fn

	return func() { open = prev }
}

// Open opens url in the user's default browser. Returns an error when the
// OS handler cannot be dispatched (missing xdg-open, Windows shell failure,
// unsupported URL scheme).
func Open(url string) error {
	if url == "" {
		return fmt.Errorf("browser: empty url")
	}
	if err := open(url); err != nil {
		return fmt.Errorf("browser: open %q: %w", url, err)
	}

	return nil
}
