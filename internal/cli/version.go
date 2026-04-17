package cli

import (
	"fmt"
	"io"

	"github.com/niklod/lazylab/internal/version"
)

func Version(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "lazylab %s\n", version.String()); err != nil {
		return fmt.Errorf("write version output: %w", err)
	}
	return nil
}
