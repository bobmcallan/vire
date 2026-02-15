package models

import "time"

// InternalUser represents a user account stored in the internal database.
// Auth and identity only — preferences are stored as UserKeyValue entries.
type InternalUser struct {
	UserID       string    `json:"user_id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"password_hash"`
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
