package config

import (
	"os"
	"path/filepath"
)

const (
	appDirName     = "lazylab"
	configFileName = "config.yaml"
	logFileName    = "lazylab.log"
	cacheDirName   = ".cache"

	EnvConfigPath  = "LAZYLAB_CONFIG"
	EnvGitLabToken = "LAZYLAB_GITLAB_TOKEN"
)

func DefaultConfigPath() string {
	if p := os.Getenv(EnvConfigPath); p != "" {
		return p
	}

	return filepath.Join(defaultAppDir(), configFileName)
}

func defaultAppDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, appDirName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", appDirName)
	}

	return filepath.Join(home, ".config", appDirName)
}

func defaultCacheDir() string {
	return filepath.Join(defaultAppDir(), cacheDirName)
}

func defaultLogfilePath() string {
	return filepath.Join(defaultAppDir(), logFileName)
}
