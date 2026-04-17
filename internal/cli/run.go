package cli

import (
	"fmt"
	"io"

	"github.com/spf13/afero"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
)

const runReadyMessage = "lazylab: config loaded; TUI coming in Phase G2"

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

	// TODO(G2): hand AppContext to the TUI instead of discarding it.
	_ = appcontext.New(cfg, client)

	if _, err := fmt.Fprintln(w, runReadyMessage); err != nil {
		return fmt.Errorf("run: write output: %w", err)
	}
	return nil
}
