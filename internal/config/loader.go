package config

import (
	"errors"
	"fmt"
	"os"

	"dario.cat/mergo"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

func Load(fs afero.Fs, path string) (*Config, error) {
	defaults := Defaults()

	raw, err := afero.ReadFile(fs, path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if saveErr := defaults.Save(fs, path); saveErr != nil {
			return nil, fmt.Errorf("seed default config: %w", saveErr)
		}
		applyEnvOverrides(defaults)
		defaults.GitLab.normalize()

		return defaults, nil
	case err != nil:
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	if err := mergo.Merge(cfg, defaults); err != nil {
		return nil, fmt.Errorf("merge defaults: %w", err)
	}
	applyEnvOverrides(cfg)
	cfg.GitLab.normalize()

	return cfg, nil
}

func applyEnvOverrides(c *Config) {
	if tok := os.Getenv(EnvGitLabToken); tok != "" {
		c.GitLab.Token = tok
	}
}
