package config

import (
	"strings"

	"github.com/niklod/lazylab/internal/models"
)

type Config struct {
	GitLab        GitLabConfig        `yaml:"gitlab"`
	Repositories  RepositoriesConfig  `yaml:"repositories"`
	MergeRequests MergeRequestsConfig `yaml:"merge_requests"`
	Cache         CacheConfig         `yaml:"cache"`
	Core          CoreConfig          `yaml:"core"`
}

type GitLabConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

func (g *GitLabConfig) normalize() {
	g.URL = strings.TrimRight(g.URL, "/")
}

type RepositoriesConfig struct {
	Favorites []string `yaml:"favorites,omitempty"`
	SortBy    string   `yaml:"sort_by"`
}

type MergeRequestsConfig struct {
	StateFilter models.MRStateFilter `yaml:"state_filter"`
	OwnerFilter models.MROwnerFilter `yaml:"owner_filter"`
}

type CacheConfig struct {
	Directory string `yaml:"directory"`
	TTL       int    `yaml:"ttl"`
}

type CoreConfig struct {
	Logfile         string `yaml:"logfile"`
	LogfileMaxBytes int    `yaml:"logfile_max_bytes"`
	LogfileCount    int    `yaml:"logfile_count"`
}
