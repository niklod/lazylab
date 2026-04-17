package models

type DiscussionStats struct {
	TotalResolvable int `json:"total_resolvable"`
	Resolved        int `json:"resolved"`
}
