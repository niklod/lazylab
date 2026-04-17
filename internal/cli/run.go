package cli

import (
	"fmt"
	"io"
)

const runStubMessage = "lazylab run: TUI not yet implemented (Phase G2+)"

func Run(w io.Writer) error {
	if _, err := fmt.Fprintln(w, runStubMessage); err != nil {
		return fmt.Errorf("write run stub: %w", err)
	}
	return nil
}
