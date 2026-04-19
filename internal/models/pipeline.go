package models

import "time"

type Pipeline struct {
	ID          int            `json:"id"`
	Status      PipelineStatus `json:"status"`
	Ref         string         `json:"ref"`
	SHA         string         `json:"sha"`
	WebURL      string         `json:"web_url"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	TriggeredBy *User          `json:"triggered_by,omitempty"`
}

type PipelineJob struct {
	ID           int            `json:"id"`
	Name         string         `json:"name"`
	Stage        string         `json:"stage"`
	Status       PipelineStatus `json:"status"`
	WebURL       string         `json:"web_url"`
	Duration     *float64       `json:"duration,omitempty"`
	AllowFailure bool           `json:"allow_failure"`
}

type PipelineDetail struct {
	Pipeline Pipeline      `json:"pipeline"`
	Jobs     []PipelineJob `json:"jobs,omitempty"`
}
