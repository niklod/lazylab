package config

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

const (
	configFileMode = 0o600
	configDirMode  = 0o700
)

func (c *Config) Save(fs afero.Fs, path string) error {
	dir := filepath.Dir(path)
	if err := fs.MkdirAll(dir, configDirMode); err != nil {
		return fmt.Errorf("mkdir config dir %s: %w", dir, err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := afero.WriteFile(fs, path, data, configFileMode); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}

	return nil
}
