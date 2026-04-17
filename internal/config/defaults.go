package config

import "github.com/niklod/lazylab/internal/models"

func Defaults() *Config {
	return &Config{
		GitLab: GitLabConfig{
			URL: "https://gitlab.com",
		},
		Repositories: RepositoriesConfig{
			SortBy: "last_activity",
		},
		MergeRequests: MergeRequestsConfig{
			StateFilter: models.MRStateFilterOpened,
			OwnerFilter: models.MROwnerFilterAll,
		},
		Cache: CacheConfig{
			Directory: defaultCacheDir(),
			TTL:       600,
		},
		Core: CoreConfig{
			Logfile:         defaultLogfilePath(),
			LogfileMaxBytes: 5_000_000,
			LogfileCount:    5,
		},
	}
}
