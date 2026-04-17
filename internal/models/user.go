package models

type User struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	WebURL    string `json:"web_url"`
	AvatarURL string `json:"avatar_url,omitempty"`
}
