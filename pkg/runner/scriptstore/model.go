package scriptstore

import "time"

type Script struct {
	Name        string    `json:"name" yaml:"name"`
	Language    string    `json:"language" yaml:"language"`
	Description string    `json:"description" yaml:"description"`
	Tags        []string  `json:"tags" yaml:"tags"`
	Content     string    `json:"content" yaml:"content"`
	Version     int64     `json:"version" yaml:"version"`
	Checksum    string    `json:"checksum" yaml:"checksum"`
	CreatedAt   time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" yaml:"updated_at"`
}
