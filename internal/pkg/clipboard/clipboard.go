// Package clipboard writes strings to the OS clipboard. The Clipboard
// interface is the consumer-side abstraction (DetailView depends on it) so
// tests can inject a fake that captures the written payload instead of
// touching the real clipboard.
package clipboard

import (
	"fmt"

	atotto "github.com/atotto/clipboard"
)

// Clipboard is the single-method sink the views package depends on. Kept
// narrow per the project's "small, consumer-owned interfaces" rule.
type Clipboard interface {
	WriteAll(text string) error
}

type systemClipboard struct{}

// System returns a system-backed clipboard that writes to the OS clipboard
// via atotto/clipboard. The returned value satisfies Clipboard; returning
// the interface here is deliberate — there is only one production
// implementation and no call site benefits from the concrete type.
//
//nolint:ireturn // factory seam: single production impl, tests swap in Fake.
func System() Clipboard { return systemClipboard{} }

func (systemClipboard) WriteAll(text string) error {
	if err := atotto.WriteAll(text); err != nil {
		return fmt.Errorf("clipboard: write: %w", err)
	}

	return nil
}

// Fake is a test-friendly implementation that records the most recent write.
// Concurrent writers are serialised by the caller, not here — the Pipeline
// tab only dispatches one copy at a time.
type Fake struct {
	Text string
	Err  error
}

// WriteAll captures text unless Err is set, in which case it returns Err
// without mutating Text (so tests can assert on the "user was told copy
// failed" path).
func (f *Fake) WriteAll(text string) error {
	if f.Err != nil {
		return f.Err
	}
	f.Text = text

	return nil
}
