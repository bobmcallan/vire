package models

import "time"

// OAuthClient represents a registered OAuth 2.1 client (DCR).
type OAuthClient struct {
	ClientID         string    `json:"client_id"`
	ClientSecretHash string    `json:"-"`
	ClientName       string    `json:"client_name"`
	RedirectURIs     []string  `json:"redirect_uris"`
	CreatedAt        time.Time `json:"created_at"`
}

// OAuthCode represents an authorization code issued during the OAuth flow.
type OAuthCode struct {
	Code                string    `json:"code"`
	ClientID            string    `json:"client_id"`
	UserID              string    `json:"user_id"`
	RedirectURI         string    `json:"redirect_uri"`
	CodeChallenge       string    `json:"code_challenge"`
	CodeChallengeMethod string    `json:"code_challenge_method"` // always "S256"
	Scope               string    `json:"scope"`
	ExpiresAt           time.Time `json:"expires_at"`
	Used                bool      `json:"used"`
	CreatedAt           time.Time `json:"created_at"`
}

// OAuthRefreshToken represents a refresh token stored with a hashed value.
type OAuthRefreshToken struct {
	TokenHash  string    `json:"-"`
	ClientID   string    `json:"client_id"`
	UserID     string    `json:"user_id"`
	Scope      string    `json:"scope"`
	ExpiresAt  time.Time `json:"expires_at"`
	Revoked    bool      `json:"revoked"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"` // Updated on each token use (sliding expiry)
}
