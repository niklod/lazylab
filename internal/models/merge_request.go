package models

import "time"

type MergeRequest struct {
	ID             int        `json:"id"`
	IID            int        `json:"iid"`
	Title          string     `json:"title"`
	Description    string     `json:"description,omitempty"`
	State          MRState    `json:"state"`
	Author         User       `json:"author"`
	Reviewers      []User     `json:"reviewers,omitempty"`
	SourceBranch   string     `json:"source_branch"`
	TargetBranch   string     `json:"target_branch"`
	WebURL         string     `json:"web_url"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	MergedAt       *time.Time `json:"merged_at,omitempty"`
	HasConflicts   bool       `json:"has_conflicts"`
	MergeStatus    string     `json:"merge_status"`
	UserNotesCount int        `json:"user_notes_count"`
	ProjectPath    string     `json:"project_path,omitempty"`
}

func (m MergeRequest) IsMerged() bool { return m.State.IsMerged() }
func (m MergeRequest) IsOpen() bool   { return m.State.IsOpen() }
func (m MergeRequest) IsClosed() bool { return m.State.IsClosed() }
