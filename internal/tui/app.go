package tui

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jesseduffield/gocui"
	"golang.org/x/term"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/tui/views"
)

// ErrRequiresTTY is returned by Run when stdout is not an interactive terminal.
var ErrRequiresTTY = errors.New("lazylab run requires an interactive terminal")

// Run launches the TUI and blocks until the user quits or an error occurs.
// ctx is propagated to initial data loads (e.g. repos) so callers can cancel
// pending HTTP requests by closing it before Run returns.
func Run(ctx context.Context, app *appcontext.AppContext) error {
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

	v := views.New(g, app)
	g.SetManagerFunc(NewManager(v))

	SetFocusOrderProvider(v.FocusOrder)
	defer SetFocusOrderProvider(nil)

	if err := Bind(g, v.Bindings()...); err != nil {
		return fmt.Errorf("tui: register keybindings: %w", err)
	}

	if app.Cache != nil {
		app.Cache.SetOnRefresh(func(refreshCtx context.Context, namespace, key string) {
			g.Update(func(*gocui.Gui) error {
				v.Dispatch(refreshCtx, namespace, key)

				return nil
			})
		})
		defer app.Cache.SetOnRefresh(nil)
	}

	v.Repos.Load(ctx)

	if err := g.MainLoop(); err != nil && !errors.Is(err, gocui.ErrQuit) {
		return fmt.Errorf("tui: main loop: %w", err)
	}

	return nil
}
