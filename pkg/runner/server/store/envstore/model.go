package envstore

import "time"

type EnvVar struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Sensitive   bool   `json:"sensitive"`
}

type EnvironmentRecord struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Vars        []EnvVar  `json:"vars,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}
