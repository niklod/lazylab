package tui

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jesseduffield/gocui"
	"golang.org/x/term"

	"github.com/niklod/lazylab/internal/appcontext"
)

// ErrRequiresTTY is returned by Run when stdout is not an interactive terminal.
var ErrRequiresTTY = errors.New("lazylab run requires an interactive terminal")

// Run launches the TUI and blocks until the user quits or an error occurs.
// The ctx argument is reserved for later phases (it will wire parent cancellation
// to gocui.ErrQuit via g.Update once data-loading goroutines exist). G2 task 1
// spawns no goroutines, so ctx cancellation has nothing to tear down today.
// Likewise app is held for future handlers that need GitLab/Cache access.
func Run(_ context.Context, app *appcontext.AppContext) error {
	if app == nil {
		return fmt.Errorf("tui: app context is nil")
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return ErrRequiresTTY
	}

	g, err := gocui.NewGui(gocui.NewGuiOpts{OutputMode: gocui.OutputTrue})
	if err != nil {
		return fmt.Errorf("tui: init gui: %w", err)
	}
	defer g.Close()

	g.SetManagerFunc(layout)

	if err := Bind(g); err != nil {
		return fmt.Errorf("tui: register keybindings: %w", err)
	}

	if err := g.MainLoop(); err != nil && !errors.Is(err, gocui.ErrQuit) {
		return fmt.Errorf("tui: main loop: %w", err)
	}

	return nil
}
