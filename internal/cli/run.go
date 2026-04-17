package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/afero"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
	"github.com/niklod/lazylab/internal/tui"
)

const cacheShutdownBudget = 2 * time.Second

type runOptions struct {
	fs         afero.Fs
	configPath string
}

type RunOption func(*runOptions)

func WithFS(fs afero.Fs) RunOption {
	return func(o *runOptions) { o.fs = fs }
}

func WithConfigPath(path string) RunOption {
	return func(o *runOptions) { o.configPath = path }
}

func Run(w io.Writer, opts ...RunOption) error {
	o := &runOptions{
		fs:         afero.NewOsFs(),
		configPath: config.DefaultConfigPath(),
	}
	for _, opt := range opts {
		opt(o)
	}

	cfg, err := config.Load(o.fs, o.configPath)
	if err != nil {
		return fmt.Errorf("run: load config: %w", err)
	}

	client, err := gitlab.New(cfg.GitLab)
	if err != nil {
		return fmt.Errorf("run: build gitlab client: %w", err)
	}

	c := cache.New(cfg.Cache, o.fs)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), cacheShutdownBudget)
		defer cancel()
		if err := c.Shutdown(ctx); err != nil {
			_, _ = fmt.Fprintf(w, "lazylab: cache shutdown: %v\n", err)
		}
	}()

	app := appcontext.New(cfg, client, c)

	if err := tui.Run(context.Background(), app); err != nil {
		return fmt.Errorf("run: tui: %w", err)
	}

	return nil
}
