package models

import (
	"fmt"
	"time"
)

// Role constants for user access control.
const (
	RoleAdmin   = "admin"
	RoleUser    = "user"
	RoleService = "service"
)

// ValidateRole checks that a role string is one of the allowed values.
func ValidateRole(role string) error {
	switch role {
	case RoleAdmin, RoleUser, RoleService:
		return nil
	default:
		return fmt.Errorf("invalid role %q: must be %q, %q, or %q", role, RoleAdmin, RoleUser, RoleService)
	}
}

// InternalUser represents a user account stored in the internal database.
// Auth and identity only — preferences are stored as UserKeyValue entries.
type InternalUser struct {
	UserID       string    `json:"user_id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"password_hash"`
	Provider     string    `json:"provider"` // "email", "google", "github", "dev"
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	ModifiedAt   time.Time `json:"modified_at"`
}

// UserKeyValue represents a per-user configuration key-value pair.
type UserKeyValue struct {
	UserID   string    `json:"user_id"`
	Key      string    `json:"key"`
	Value    string    `json:"value"`
	Version  int       `json:"version"`
	DateTime time.Time `json:"datetime"`
}

// UserRecord is a generic document record for all user domain data.
// Replaces typed domain stores (portfolio, strategy, plan, watchlist, report, search).
type UserRecord struct {
	UserID   string    `json:"user_id"`
	Subject  string    `json:"subject"`
	Key      string    `json:"key"`
	Value    string    `json:"value"`
	Version  int       `json:"version"`
	DateTime time.Time `json:"datetime"`
}
