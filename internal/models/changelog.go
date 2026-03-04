package models

import "time"

// ChangelogEntry represents a single changelog entry for a service.
type ChangelogEntry struct {
	ID             string    `json:"id"`
	Service        string    `json:"service"`                   // e.g. "vire-server", "vire-portal"
	ServiceVersion string    `json:"service_version"`           // e.g. "0.3.153"
	ServiceBuild   string    `json:"service_build,omitempty"`   // e.g. "2026-03-04-14-30-00"
	Content        string    `json:"content"`                   // markdown body
	CreatedByID    string    `json:"created_by_id,omitempty"`   // user/service ID
	CreatedByName  string    `json:"created_by_name,omitempty"` // display name
	UpdatedByID    string    `json:"updated_by_id,omitempty"`
	UpdatedByName  string    `json:"updated_by_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
