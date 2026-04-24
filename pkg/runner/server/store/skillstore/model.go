package skillstore

import "time"

type SkillRecord struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Triggers    []string  `json:"triggers,omitempty"`
	Content     string    `json:"content,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}
