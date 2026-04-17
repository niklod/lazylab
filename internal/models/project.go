package models

import "time"

type Project struct {
	ID                int       `json:"id"`
	Name              string    `json:"name"`
	PathWithNamespace string    `json:"path_with_namespace"`
	DefaultBranch     string    `json:"default_branch,omitempty"`
	WebURL            string    `json:"web_url"`
	LastActivityAt    time.Time `json:"last_activity_at"`
	Archived          bool      `json:"archived"`
}
